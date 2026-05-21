package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/registry"
)

func TestKitListTextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "go-tui", `name: go-tui
description: Go CLI + TUI conventions.
skills:
  - init-go-cli
  - init-go-cli-tui
`)
	writeKitFixture(t, home, "laravel", `name: laravel
description: Laravel conventions.
skills:
  - init-laravel
`)

	cmd := newKitListCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit list: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "go-tui  Go CLI + TUI conventions.  (2 skills)") {
		t.Fatalf("output missing go-tui row:\n%s", got)
	}
	if !strings.Contains(got, "laravel  Laravel conventions.  (1 skills)") {
		t.Fatalf("output missing laravel row:\n%s", got)
	}
}

func TestKitListJSONEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "go-tui", `name: go-tui
description: Go CLI + TUI conventions.
skills:
  - init-go-cli
  - init-go-cli-tui
`)

	env := executeEnvelopeCommand(t, []string{"kit", "list", "--json"})

	var data struct {
		Kits []struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			SkillsCount int      `json:"skills_count"`
			Skills      []string `json:"skills"`
		} `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Kits) != 1 {
		t.Fatalf("kits count = %d, want 1", len(data.Kits))
	}
	if data.Kits[0].Name != "go-tui" {
		t.Fatalf("name = %q, want go-tui", data.Kits[0].Name)
	}
	if data.Kits[0].SkillsCount != 2 {
		t.Fatalf("skills_count = %d, want 2", data.Kits[0].SkillsCount)
	}
	if strings.Join(data.Kits[0].Skills, ",") != "init-go-cli,init-go-cli-tui" {
		t.Fatalf("skills = %#v", data.Kits[0].Skills)
	}
}

func TestKitListFieldsFiltersJSONKitEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "go-tui", `name: go-tui
description: Go CLI + TUI conventions.
skills:
  - init-go-cli
`)

	env := executeEnvelopeCommand(t, []string{"kit", "list", "--json", "--fields", "name,skills_count"})

	var data struct {
		Kits []map[string]any `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Kits) != 1 {
		t.Fatalf("kits count = %d, want 1", len(data.Kits))
	}
	if _, ok := data.Kits[0]["description"]; ok {
		t.Fatalf("description field present after projection: %#v", data.Kits[0])
	}
	if data.Kits[0]["name"] != "go-tui" {
		t.Fatalf("name = %v, want go-tui", data.Kits[0]["name"])
	}
	if data.Kits[0]["skills_count"] != float64(1) {
		t.Fatalf("skills_count = %v, want 1", data.Kits[0]["skills_count"])
	}
}

func TestKitListDefaultMergesLocalAndRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "local-kit", `name: local-kit
skills:
  - local-skill
`)

	stubKitFactory(t, []string{"acme/skills"}, map[string][]registry.ManifestKit{
		"acme/skills": {{
			Registry:    "acme/skills",
			Name:        "remote-kit",
			Path:        "kits/remote-kit.yaml",
			Description: "Remote kit",
			Author:      "acme",
		}},
	})

	data := executeKitListJSON(t, "--json")
	if len(data) != 2 {
		t.Fatalf("kits count = %d, want 2: %#v", len(data), data)
	}
	names := []string{data[0].Name, data[1].Name}
	sort.Strings(names)
	if names[0] != "local-kit" || names[1] != "remote-kit" {
		t.Fatalf("names = %#v, want [local-kit remote-kit]", names)
	}
}

func TestKitListRemoteOnlyHidesLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "local-kit", `name: local-kit
skills:
  - local-skill
`)

	stubKitFactory(t, []string{"acme/skills"}, map[string][]registry.ManifestKit{
		"acme/skills": {{Registry: "acme/skills", Name: "remote-kit", Path: "kits/remote-kit.yaml"}},
	})

	data := executeKitListJSON(t, "--remote", "--json")
	if len(data) != 1 || data[0].Name != "remote-kit" || !data[0].Remote {
		t.Fatalf("data = %#v, want one remote-kit row", data)
	}
}

func TestKitListLocalSkipsNetwork(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "local-kit", `name: local-kit
skills:
  - local-skill
`)

	stubKitFactory(t, []string{"acme/skills"}, map[string][]registry.ManifestKit{
		"acme/skills": nil,
	})
	called := false
	oldList := listRemoteKitsFn
	listRemoteKitsFn = func(ctx context.Context, f registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { listRemoteKitsFn = oldList })

	data := executeKitListJSON(t, "--local", "--json")
	if called {
		t.Fatal("--local must not call remote listing")
	}
	if len(data) != 1 || data[0].Name != "local-kit" {
		t.Fatalf("data = %#v, want one local-kit row", data)
	}
}

func TestKitListWarnsOnRegistryListFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stubKitFactory(t, []string{"broken/skills", "acme/skills"}, map[string][]registry.ManifestKit{
		"broken/skills": nil,
		"acme/skills":   {{Registry: "acme/skills", Name: "good-kit", Path: "kits/good-kit.yaml"}},
	})
	oldList := listRemoteKitsFn
	listRemoteKitsFn = func(ctx context.Context, f registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		if repo == "broken/skills" {
			return nil, fmt.Errorf("permission denied")
		}
		return []registry.ManifestKit{{Registry: repo, Name: "good-kit", Path: "kits/good-kit.yaml"}}, nil
	}
	t.Cleanup(func() { listRemoteKitsFn = oldList })

	cmd := newKitListCommand()
	cmd.SetArgs([]string{"--json"})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit list: %v", err)
	}
	if !strings.Contains(errBuf.String(), "warning: skip remote kits from broken/skills") {
		t.Fatalf("missing skip warning, stderr = %q", errBuf.String())
	}
	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var data struct {
		Kits []kitJSONRow `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Kits) != 1 || data.Kits[0].Name != "good-kit" {
		t.Fatalf("data = %#v, want one good-kit row", data.Kits)
	}
}

func TestKitListSilentlySkipsRegistryWithoutManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stubKitFactory(t, []string{"missing/skills", "acme/skills"}, map[string][]registry.ManifestKit{
		"missing/skills": nil,
		"acme/skills":    {{Registry: "acme/skills", Name: "good-kit", Path: "kits/good-kit.yaml"}},
	})
	oldList := listRemoteKitsFn
	listRemoteKitsFn = func(ctx context.Context, f registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		if repo == "missing/skills" {
			return nil, clierrors.Wrap(fmt.Errorf("no manifest"), "REGISTRY_NOT_FOUND", clierrors.ExitNotFound)
		}
		return []registry.ManifestKit{{Registry: repo, Name: "good-kit", Path: "kits/good-kit.yaml"}}, nil
	}
	t.Cleanup(func() { listRemoteKitsFn = oldList })

	cmd := newKitListCommand()
	cmd.SetArgs([]string{"--json"})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit list: %v", err)
	}
	if strings.Contains(errBuf.String(), "warning: skip remote kits from missing/skills") {
		t.Fatalf("unexpected skip warning, stderr = %q", errBuf.String())
	}
	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var data struct {
		Kits []kitJSONRow `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Kits) != 1 || data.Kits[0].Name != "good-kit" {
		t.Fatalf("data = %#v, want one good-kit row", data.Kits)
	}
}

func TestKitListRegistryFiltersBoth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "acme-kit", `name: acme-kit
source:
  registry: acme/skills
skills:
  - x
`)
	writeKitFixture(t, home, "other-kit", `name: other-kit
source:
  registry: other/skills
skills:
  - y
`)

	stubKitFactory(t, []string{"acme/skills", "other/skills"}, map[string][]registry.ManifestKit{
		"acme/skills":  {{Registry: "acme/skills", Name: "remote-acme", Path: "kits/remote-acme.yaml"}},
		"other/skills": {{Registry: "other/skills", Name: "remote-other", Path: "kits/remote-other.yaml"}},
	})

	data := executeKitListJSON(t, "--registry", "acme/skills", "--json")
	got := map[string]bool{}
	for _, row := range data {
		got[row.Name] = true
	}
	if !got["acme-kit"] || !got["remote-acme"] || got["other-kit"] || got["remote-other"] {
		t.Fatalf("filter mismatch: %#v", got)
	}
}

type kitJSONRow struct {
	Name             string `json:"name"`
	Registry         string `json:"registry,omitempty"`
	Path             string `json:"path,omitempty"`
	Remote           bool   `json:"remote,omitempty"`
	InstalledLocally bool   `json:"installed_locally,omitempty"`
}

func stubKitFactory(t *testing.T, repos []string, kitsByRepo map[string][]registry.ManifestKit) {
	t.Helper()
	oldFactory := commandFactory
	oldList := listRemoteKitsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		listRemoteKitsFn = oldList
	})
	regs := make([]config.RegistryConfig, 0, len(repos))
	for _, r := range repos {
		regs = append(regs, config.RegistryConfig{Repo: r, Enabled: true})
	}
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: regs}, nil
			},
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	listRemoteKitsFn = func(_ context.Context, _ registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		return kitsByRepo[repo], nil
	}
}

func executeKitListJSON(t *testing.T, args ...string) []kitJSONRow {
	t.Helper()
	cmd := newKitListCommand()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit list %v: %v", args, err)
	}
	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	var payload struct {
		Kits []kitJSONRow `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	return payload.Kits
}

func writeKitFixture(t *testing.T, home, name, content string) {
	t.Helper()
	path := filepath.Join(home, ".scribe", "kits", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir kits dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write kit fixture: %v", err)
	}
}
