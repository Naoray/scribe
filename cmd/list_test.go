package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestPrintLocalTable_WithSkills(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version:    "v0.12.9.0",
				Source:     "github:garrytan/gstack@v0.12.9.0",
				Targets:    []string{"claude", "cursor"},
				Registries: []string{"ArtistfyHQ/team-skills"},
			},
			"deploy": {
				Version:    "main",
				CommitSHA:  "e4f8a2d1234567",
				Source:     "github:ArtistfyHQ/team-skills@main",
				Targets:    []string{"claude"},
				Registries: []string{"ArtistfyHQ/team-skills"},
			},
		},
	}

	var buf bytes.Buffer
	err := printLocalTable(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()

	// Header present.
	if !strings.Contains(out, "SKILL") || !strings.Contains(out, "VERSION") {
		t.Errorf("missing table header, got:\n%s", out)
	}

	// Skills are sorted alphabetically: deploy before gstack.
	deployIdx := strings.Index(out, "deploy")
	gstackIdx := strings.Index(out, "gstack")
	if deployIdx == -1 || gstackIdx == -1 {
		t.Fatalf("missing skill names, got:\n%s", out)
	}
	if deployIdx > gstackIdx {
		t.Errorf("skills not sorted alphabetically: deploy at %d, gstack at %d", deployIdx, gstackIdx)
	}

	// Version display: branch ref shows sha.
	if !strings.Contains(out, "main@e4f8a2d") {
		t.Errorf("expected branch@sha version display, got:\n%s", out)
	}

	// Source column strips @ref.
	if !strings.Contains(out, "github:garrytan/gstack") {
		t.Errorf("expected stripped source, got:\n%s", out)
	}

	// Targets column.
	if !strings.Contains(out, "claude, cursor") {
		t.Errorf("expected targets 'claude, cursor', got:\n%s", out)
	}
}

func TestPrintLocalTable_EmptyState(t *testing.T) {
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := printLocalTable(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No skills installed.") {
		t.Errorf("expected empty state message, got:\n%s", out)
	}
	if !strings.Contains(out, "scribe connect") {
		t.Errorf("expected connect hint, got:\n%s", out)
	}
}

func TestPrintLocalJSON_WithSkills(t *testing.T) {
	now := time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC)
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version:     "v0.12.9.0",
				Source:      "github:garrytan/gstack@v0.12.9.0",
				InstalledAt: now,
				Targets:     []string{"claude", "cursor"},
				Registries:  []string{"ArtistfyHQ/team-skills"},
			},
		},
	}

	var buf bytes.Buffer
	err := printLocalJSON(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected name field in JSON, got:\n%s", out)
	}
	if !strings.Contains(out, `"gstack"`) {
		t.Errorf("expected gstack in JSON, got:\n%s", out)
	}
	if !strings.Contains(out, `"version"`) {
		t.Errorf("expected version field in JSON, got:\n%s", out)
	}
}

func TestPrintLocalJSON_EmptyState(t *testing.T) {
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := printLocalJSON(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected empty JSON array, got: %s", out)
	}
}

func TestRunList_LocalFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version: "v0.12.9.0",
				Source:  "github:garrytan/gstack@v0.12.9.0",
				Targets: []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	listLocal = true
	listJSON = false
	defer func() { listLocal = false }()

	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)

	err := runList(listCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "gstack") {
		t.Errorf("expected skill in output, got:\n%s", out)
	}
}

func TestRunList_NoRegistries_FallsBackToLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"deploy": {
				Version: "v1.0.0",
				Source:  "github:ArtistfyHQ/team-skills@v1.0.0",
				Targets: []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	listLocal = false
	listJSON = false
	registryFlag = ""

	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)

	err := runList(listCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy") {
		t.Errorf("expected local skill in fallback output, got:\n%s", out)
	}
	if !strings.Contains(out, "scribe connect") {
		t.Errorf("expected connect hint in fallback output, got:\n%s", out)
	}
}
