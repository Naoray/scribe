package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/app"
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

func TestKitListRemoteJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "local-kit", `name: local-kit
skills:
  - local-skill
`)

	oldFactory := commandFactory
	oldList := listRemoteKitsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		listRemoteKitsFn = oldList
	})
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	listRemoteKitsFn = func(_ context.Context, _ registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		return []registry.ManifestKit{{
			Registry:    repo,
			Name:        "remote-kit",
			Path:        "kits/remote-kit.yaml",
			Description: "Remote kit",
			Author:      "acme",
		}}, nil
	}

	cmd := newKitListCommand()
	cmd.SetArgs([]string{"--remote", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit list --remote: %v", err)
	}

	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	var data struct {
		Kits []struct {
			Name     string `json:"name"`
			Registry string `json:"registry,omitempty"`
			Path     string `json:"path,omitempty"`
			Remote   bool   `json:"remote,omitempty"`
		} `json:"kits"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Kits) != 2 {
		t.Fatalf("kits count = %d, want 2: %#v", len(data.Kits), data.Kits)
	}
	if data.Kits[1].Name != "remote-kit" || data.Kits[1].Registry != "acme/skills" || data.Kits[1].Path != "kits/remote-kit.yaml" || !data.Kits[1].Remote {
		t.Fatalf("remote kit = %#v", data.Kits[1])
	}
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
