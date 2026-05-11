package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/registry"
)

func TestKitShowTextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "go-tui", `name: go-tui
description: Go CLI + TUI (Cobra + Bubble Tea / Charm) conventions.
skills:
  - init-go-cli
  - init-go-cli-tui
  - init-charm
source:
  registry: acme/kits
`)

	cmd := newKitShowCommand()
	cmd.SetArgs([]string{"go-tui"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit show: %v", err)
	}

	want := `Kit: go-tui
Description: Go CLI + TUI (Cobra + Bubble Tea / Charm) conventions.
Skills (3): init-go-cli, init-go-cli-tui, init-charm
Source: acme/kits
`
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestKitShowTextOutputUsesLocalSourceFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "local", `name: local
skills: []
`)

	cmd := newKitShowCommand()
	cmd.SetArgs([]string{"local"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit show: %v", err)
	}
	if !strings.Contains(out.String(), "Source: (local)\n") {
		t.Fatalf("output missing local source:\n%s", out.String())
	}
}

func TestKitShowJSONEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKitFixture(t, home, "go-tui", `name: go-tui
description: Go CLI + TUI conventions.
skills:
  - init-go-cli
source:
  registry: acme/kits
`)

	env := executeEnvelopeCommand(t, []string{"kit", "show", "go-tui", "--json"})

	var data struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Skills      []string `json:"skills"`
		Source      *struct {
			Registry string `json:"registry"`
		} `json:"source"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Name != "go-tui" {
		t.Fatalf("name = %q, want go-tui", data.Name)
	}
	if strings.Join(data.Skills, ",") != "init-go-cli" {
		t.Fatalf("skills = %#v", data.Skills)
	}
	if data.Source == nil || data.Source.Registry != "acme/kits" {
		t.Fatalf("source = %#v, want acme/kits", data.Source)
	}
}

func TestKitShowNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newKitShowCommand()
	cmd.SetArgs([]string{"missing"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected not found error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitNotFound {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitNotFound, err)
	}
}

func TestKitShowRemoteJSONClassifiesRefs(t *testing.T) {
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
	})
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{
					{Repo: "acme/skills", Enabled: true},
					{Repo: "other/skills", Enabled: true},
				}}, nil
			},
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name, Path: "kits/" + name + ".yaml"}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{
			Name:        entry.Name,
			Description: "Remote baseline",
			Skills:      []string{"tdd", "other/skills:debugging", "missing/skills:qa", "init-*"},
			Source:      &kit.Source{Registry: registryRepo},
		}, nil
	}

	cmd := newKitShowCommand()
	cmd.SetArgs([]string{"acme/skills:baseline", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute kit show remote: %v", err)
	}

	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	var data struct {
		Name     string `json:"name"`
		Registry string `json:"registry"`
		Refs     []struct {
			Raw       string `json:"raw"`
			Origin    string `json:"origin"`
			Registry  string `json:"registry"`
			Connected bool   `json:"connected"`
			Glob      bool   `json:"glob"`
			Reason    string `json:"reason"`
		} `json:"refs"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Name != "baseline" || data.Registry != "acme/skills" {
		t.Fatalf("data = %#v", data)
	}
	if len(data.Refs) != 4 {
		t.Fatalf("refs count = %d, want 4", len(data.Refs))
	}
	if data.Refs[0].Origin != "same_registry" || !data.Refs[0].Connected {
		t.Fatalf("same-registry ref = %#v", data.Refs[0])
	}
	if data.Refs[1].Origin != "cross_registry" || data.Refs[1].Registry != "other/skills" || !data.Refs[1].Connected {
		t.Fatalf("connected cross-registry ref = %#v", data.Refs[1])
	}
	if data.Refs[2].Reason != "registry_not_connected" || data.Refs[2].Connected {
		t.Fatalf("missing cross-registry ref = %#v", data.Refs[2])
	}
	if !data.Refs[3].Glob {
		t.Fatalf("glob ref = %#v", data.Refs[3])
	}
}

func TestKitShowRemoteRegistryNotConnected(t *testing.T) {
	oldFactory := commandFactory
	t.Cleanup(func() { commandFactory = oldFactory })
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{}, nil
			},
		}
	}

	cmd := newKitShowCommand()
	cmd.SetArgs([]string{"acme/skills:baseline", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitNotFound {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitNotFound, err)
	}
	if !strings.Contains(err.Error(), `registry "acme/skills" is not connected`) {
		t.Fatalf("error = %v", err)
	}
}
