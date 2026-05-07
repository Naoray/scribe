package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

const partialFixtureEnv = "SCRIBE_TEST_PARTIAL_PROVIDER"

type partialFixtureProvider struct{}

func (partialFixtureProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	entries := []manifest.Entry{
		{Name: "good-skill", Source: "github:acme/team@v1.0.0"},
		{Name: "bad-skill", Source: "github:acme/team@v1.0.0"},
	}
	return &provider.DiscoverResult{Entries: entries, IsTeam: true}, nil
}

func (partialFixtureProvider) Fetch(_ context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	if entry.Name == "bad-skill" {
		return nil, errors.New("fixture fetch failed")
	}
	return []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# Good Skill\n")}}, nil
}

func init() {
	if os.Getenv(partialFixtureEnv) != "1" {
		return
	}
	commandFactory = func() *app.Factory {
		f := app.NewFactory()
		f.Client = func() (*gh.Client, error) {
			return nil, nil
		}
		f.Provider = func() (provider.Provider, error) {
			return partialFixtureProvider{}, nil
		}
		return f
	}
}

func TestSyncPartialSingleEnvelopeAndStateSave(t *testing.T) {
	home := setupPartialFixtureHome(t, true)
	writeOutdatedPartialState(t, home)
	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"sync", "--json", "--registry", "acme/team"})

	assertPartialSubprocessEnvelope(t, stdout, stderr, code)
	assertStateHasInstalledSkill(t, home, "good-skill")
}

func TestAdoptPartialSingleEnvelope(t *testing.T) {
	home := setupAdoptPartialHome(t)
	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"adopt", "--no-interaction", "--json"})

	assertPartialSubprocessEnvelope(t, stdout, stderr, code)
}

func TestConnectInstallAllPartialSingleEnvelopeAndStateSave(t *testing.T) {
	home := setupPartialFixtureHome(t, false)
	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"connect", "acme/team", "--install-all", "--json"})

	assertPartialSubprocessEnvelope(t, stdout, stderr, code)
	assertStateHasInstalledSkill(t, home, "good-skill")
}

func runScribeHelperWithHome(t *testing.T, home string, args []string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestScribeHelperProcess", "--"}, args...)...)
	cmd.Dir = home
	cmd.Env = withoutEnv(os.Environ(), "HOME", "PWD")
	cmd.Env = append(cmd.Env, "GO_WANT_SCRIBE_HELPER_PROCESS=1", "HOME="+home, "PWD="+home, partialFixtureEnv+"=1")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper run: %v", err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

func setupPartialFixtureHome(t *testing.T, connected bool) string {
	t.Helper()
	home := t.TempDir()
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatalf("mkdir .scribe: %v", err)
	}
	registries := ""
	if connected {
		registries = `registries:
  - repo: acme/team
    enabled: true
    type: team
`
	}
	cfg := registries + `adoption:
  mode: off
tools:
  - name: claude
    enabled: false
  - name: codex
    enabled: false
  - name: cursor
    enabled: false
  - name: gemini
    enabled: false
  - name: fixture
    type: custom
    enabled: true
    install: "mkdir -p \"$HOME/.fixture-skills\" && ln -sfn \"{{canonical_dir}}\" \"$HOME/.fixture-skills/{{skill_name}}\""
    uninstall: "rm -f \"$HOME/.fixture-skills/{{skill_name}}\""
    path: "$HOME/.fixture-skills/{{skill_name}}"
scribe_agent:
  enabled: false
`
	if err := os.WriteFile(filepath.Join(scribeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return home
}

func setupAdoptPartialHome(t *testing.T) string {
	t.Helper()
	home := setupPartialFixtureHome(t, false)
	sourceDir := filepath.Join(home, "adopt-src")
	writeTestSkill(t, sourceDir, "good-adopt", "# good-adopt\n")
	writeTestSkill(t, sourceDir, "bad-adopt", "# bad-adopt\n")
	cfg := `adoption:
  mode: auto
  paths:
    - adopt-src
tools:
  - name: claude
    enabled: false
  - name: codex
    enabled: false
  - name: cursor
    enabled: false
  - name: gemini
    enabled: false
  - name: fixture
    type: custom
    enabled: true
    install: "test \"{{skill_name}}\" != bad-adopt && mkdir -p \"$HOME/.fixture-skills\" && ln -sfn \"{{canonical_dir}}\" \"$HOME/.fixture-skills/{{skill_name}}\""
    uninstall: "rm -f \"$HOME/.fixture-skills/{{skill_name}}\""
    path: "$HOME/.fixture-skills/{{skill_name}}"
scribe_agent:
  enabled: false
`
	if err := os.WriteFile(filepath.Join(home, ".scribe", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write adopt config: %v", err)
	}
	return home
}

func writeOutdatedPartialState(t *testing.T, home string) {
	t.Helper()
	st := state.State{
		SchemaVersion:      5,
		Installed:          map[string]state.InstalledSkill{},
		RemovedByUser:      []state.RemovedSkill{},
		Migrations:         map[string]bool{"store-v2-flat-skills": true},
		RegistryFailures:   map[string]state.RegistryFailure{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	for _, name := range []string{"good-skill", "bad-skill"} {
		st.Installed[name] = state.InstalledSkill{
			Revision: 1,
			Sources: []state.SkillSource{{
				Registry: "acme/team",
				Ref:      "v0.9.0",
			}},
			Tools: []string{"fixture"},
		}
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".scribe", "state.json"), raw, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func assertPartialSubprocessEnvelope(t *testing.T, stdout, stderr string, code int) {
	t.Helper()
	if code != 10 {
		t.Fatalf("exit = %d, want 10\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	trimmed := strings.TrimSpace(stdout)
	if strings.Count(trimmed, "\n") != 0 {
		t.Fatalf("stdout contains more than one JSON object:\n%s\nstderr=%s", stdout, stderr)
	}
	var env struct {
		Status string `json:"status"`
		Data   struct {
			Summary struct {
				Failed int `json:"failed"`
			} `json:"summary"`
			Adoption struct {
				Failed int `json:"failed"`
			} `json:"adoption"`
		} `json:"data"`
		Error *json.RawMessage `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if env.Status != "partial_success" {
		t.Fatalf("status = %q, want partial_success\nstdout=%s", env.Status, stdout)
	}
	if env.Error != nil {
		t.Fatalf("partial envelope included root error object: %s\nstdout=%s", string(*env.Error), stdout)
	}
	if env.Data.Summary.Failed == 0 && env.Data.Adoption.Failed == 0 {
		t.Fatalf("partial envelope did not report a failed item\nstdout=%s", stdout)
	}
}

func assertStateHasInstalledSkill(t *testing.T, home, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".scribe", "state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state.State
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("state is not JSON: %v\n%s", err, string(raw))
	}
	if _, ok := st.Installed[name]; !ok {
		t.Fatalf("state missing installed %q: %s", name, string(raw))
	}
}
