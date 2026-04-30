package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	clischema "github.com/Naoray/scribe/internal/cli/schema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestReadOnlyCommandsEmitEnvelopeAndValidateOutputSchema(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		setup func(t *testing.T, home string)
	}{
		{name: "list", args: []string{"list", "--json"}},
		{name: "status", args: []string{"status", "--json"}},
		{name: "doctor", args: []string{"doctor", "--json"}},
		{name: "guide", args: []string{"guide", "--json"}, setup: func(t *testing.T, home string) {
			t.Setenv("PATH", home)
		}},
		{name: "explain", args: []string{"explain", "--json", "test-skill"}, setup: func(t *testing.T, home string) {
			writeEnvelopeTestSkill(t, home)
		}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.setup != nil {
				tt.setup(t, home)
			}

			env := executeEnvelopeCommand(t, tt.args)
			if env.Status != "ok" {
				t.Fatalf("status = %q, want ok\nenvelope=%#v", env.Status, env)
			}
			if env.FormatVersion != "1" {
				t.Fatalf("format_version = %q, want 1", env.FormatVersion)
			}
			if env.Data == nil {
				t.Fatalf("data is nil")
			}

			rawSchema, ok := clischema.Get("scribe " + tt.name)
			if !ok {
				t.Fatalf("missing output schema for %s", tt.name)
			}
			schema, err := jsonschema.CompileString(tt.name+".schema.json", rawSchema)
			if err != nil {
				t.Fatalf("compile schema: %v\n%s", err, rawSchema)
			}
			var data any
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("unmarshal data: %v", err)
			}
			if err := schema.Validate(data); err != nil {
				t.Fatalf("schema validation: %v\ndata=%s\nschema=%s", err, string(env.Data), rawSchema)
			}
		})
	}
}

func TestListEnvelopeDataMatchesLegacyGolden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	env := executeEnvelopeCommand(t, []string{"list", "--json"})
	got := normalizeListLegacyData(t, env.Data, home)

	golden, err := os.ReadFile(filepath.Join("..", "testdata", "golden", "list.legacy.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want any
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("list data changed\nwant=%s\ngot=%s", wantJSON, gotJSON)
	}
}

func TestListFieldsProjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env := executeEnvelopeCommand(t, []string{"list", "--json", "--fields", "skills"})
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if _, ok := data["skills"]; !ok {
		t.Fatalf("projected data missing skills: %#v", data)
	}
	if _, ok := data["packages"]; ok {
		t.Fatalf("projected data kept packages: %#v", data)
	}

	stdout, stderr, code := runScribeHelper(t, []string{"list", "--json", "--fields", "nonexistent"}, false)
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var errEnv struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	jsonStart := strings.LastIndex(stdout, "{\"status\"")
	if jsonStart < 0 {
		t.Fatalf("stdout missing JSON envelope: %s", stdout)
	}
	if err := json.Unmarshal([]byte(stdout[jsonStart:]), &errEnv); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if errEnv.Error.Code != "USAGE_UNKNOWN_FIELD" {
		t.Fatalf("error code = %q, want USAGE_UNKNOWN_FIELD", errEnv.Error.Code)
	}
}

type testEnvelope struct {
	Status        string          `json:"status"`
	FormatVersion string          `json:"format_version"`
	Data          json.RawMessage `json:"data"`
	Meta          struct {
		DurationMS  int64 `json:"duration_ms"`
		BootstrapMS int64 `json:"bootstrap_ms"`
	} `json:"meta"`
}

func TestReadOnlyTypedErrorsRoundTripThroughEnvelope(t *testing.T) {
	stdout, stderr, code := runScribeHelper(t, []string{"explain", "nope", "--json"}, false)
	if code != 3 {
		t.Fatalf("exit = %d, want 3\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Status string `json:"status"`
		Error  struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	jsonStart := strings.LastIndex(stdout, "{\"status\"")
	if jsonStart < 0 {
		t.Fatalf("stdout missing JSON envelope: %s", stdout)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout[jsonStart:])), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if env.Status != "error" {
		t.Fatalf("status = %q, want error\nenvelope=%#v", env.Status, env)
	}
	if env.Error.Code != "SKILL_NOT_FOUND" {
		t.Fatalf("error code = %q, want SKILL_NOT_FOUND\nenvelope=%#v", env.Error.Code, env)
	}
}

func executeEnvelopeCommand(t *testing.T, args []string) testEnvelope {
	t.Helper()
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v): %v\nstdout=%s\nstderr=%s", args, err, stdout.String(), stderr.String())
	}
	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	return env
}

func writeEnvelopeTestSkill(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".scribe", "skills", "test-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := "---\nname: test-skill\ndescription: A test skill\n---\n\n# Test Skill\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func normalizeListLegacyData(t *testing.T, raw json.RawMessage, home string) any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal list data: %v", err)
	}
	skills, _ := data["skills"].([]any)
	for _, item := range skills {
		skill, _ := item.(map[string]any)
		if path, _ := skill["path"].(string); path == filepath.Join(home, ".scribe", "skills", "scribe") {
			skill["path"] = "$HOME/.scribe/skills/scribe"
		}
	}
	return data
}
