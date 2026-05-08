package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
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
