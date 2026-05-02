package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/kit"
)

func TestKitCreateWritesKitYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newKitCreateCommand()
	cmd.SetArgs([]string{
		"laravel-stack",
		"--description", "Laravel project defaults",
		"--skills", "eloquent,queues",
		"--skills", "livewire",
		"--registry", "my-org/scribe-registry",
	})

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	kitPath := filepath.Join(home, ".scribe", "kits", "laravel-stack.yaml")
	got, err := kit.Load(kitPath)
	if err != nil {
		t.Fatalf("load kit: %v", err)
	}

	if got.Name != "laravel-stack" {
		t.Fatalf("name = %q, want laravel-stack", got.Name)
	}
	if got.Description != "Laravel project defaults" {
		t.Fatalf("description = %q", got.Description)
	}
	if got.Source == nil || got.Source.Registry != "my-org/scribe-registry" {
		t.Fatalf("source registry = %#v", got.Source)
	}
	wantSkills := []string{"eloquent", "queues", "livewire"}
	if len(got.Skills) != len(wantSkills) {
		t.Fatalf("skills = %#v, want %#v", got.Skills, wantSkills)
	}
	for i := range wantSkills {
		if got.Skills[i] != wantSkills[i] {
			t.Fatalf("skills = %#v, want %#v", got.Skills, wantSkills)
		}
	}

	wantOutput := "Created kit laravel-stack at " + kitPath + " with 3 skills\n"
	if out.String() != wantOutput {
		t.Fatalf("output = %q, want %q", out.String(), wantOutput)
	}
}

func TestKitCreateJSONOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newKitCreateCommand()
	cmd.SetArgs([]string{"core", "--skills", "one,two", "--json"})

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		SkillsCount int    `json:"skills_count"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v\n%s", err, out.String())
	}

	if payload.Name != "core" {
		t.Fatalf("name = %q, want core", payload.Name)
	}
	if payload.Path != filepath.Join(home, ".scribe", "kits", "core.yaml") {
		t.Fatalf("path = %q", payload.Path)
	}
	if payload.SkillsCount != 2 {
		t.Fatalf("skills_count = %d, want 2", payload.SkillsCount)
	}
}

func TestKitCreateExistingFileRequiresForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	kitPath := filepath.Join(home, ".scribe", "kits", "core.yaml")
	if err := os.MkdirAll(filepath.Dir(kitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kitPath, []byte("name: core\nskills: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newKitCreateCommand()
	cmd.SetArgs([]string{"core", "--skills", "one"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitConflict {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitConflict, err)
	}

	data, err := os.ReadFile(kitPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "name: core\nskills: []\n" {
		t.Fatalf("existing file was overwritten: %q", string(data))
	}
}
