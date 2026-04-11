package firstrun_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func TestBuiltinRegistries(t *testing.T) {
	registries := firstrun.BuiltinRegistries()
	if len(registries) == 0 {
		t.Fatal("expected at least one built-in registry")
	}

	for _, r := range registries {
		if r.Repo == "" {
			t.Error("builtin registry has empty repo")
		}
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
		if !r.Enabled {
			t.Errorf("%s: expected Enabled=true", r.Repo)
		}
	}
}

func TestIsFirstRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if !firstrun.IsFirstRun() {
		t.Error("expected first run when no config exists")
	}
}

func TestApplyBuiltins(t *testing.T) {
	cfg := &config.Config{}
	firstrun.ApplyBuiltins(cfg)

	if len(cfg.Registries) == 0 {
		t.Fatal("expected registries to be populated")
	}

	for _, r := range cfg.Registries {
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
		if !r.Enabled {
			t.Errorf("%s: expected enabled", r.Repo)
		}
	}
}

func TestApplyBuiltinsIdempotent(t *testing.T) {
	cfg := &config.Config{}
	firstrun.ApplyBuiltins(cfg)
	count := len(cfg.Registries)

	// Apply again -- should not duplicate.
	firstrun.ApplyBuiltins(cfg)
	if len(cfg.Registries) != count {
		t.Errorf("expected %d registries after second apply, got %d", count, len(cfg.Registries))
	}
}

// ---------------------------------------------------------------------------
// PromptAdoption tests
// ---------------------------------------------------------------------------

const skillFrontmatter = `---
name: my-skill
description: test
---
body
`

// mockTool is a minimal Tool that records installs without touching the real FS.
type mockTool struct {
	name string
}

func (m mockTool) Name() string                                           { return m.name }
func (m mockTool) Detect() bool                                           { return true }
func (m mockTool) Install(_, _ string) ([]string, error)                  { return nil, nil }
func (m mockTool) Uninstall(_ string) error                               { return nil }

// seedSkill writes a SKILL.md under <claudeSkillsDir>/<name>/.
func seedSkill(t *testing.T, home, name, content string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func emptyState() *state.State {
	return &state.State{
		SchemaVersion: 4,
		Installed:     make(map[string]state.InstalledSkill),
	}
}

func TestPromptAdoption_NoCandidates_NoOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Empty ~/.claude/skills/ — no candidates.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	st := emptyState()
	toolSet := []tools.Tool{mockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader(""), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Len() != 0 {
		t.Errorf("expected no output, got: %q", out.String())
	}
	if len(st.Installed) != 0 {
		t.Errorf("expected no installed skills, got %d", len(st.Installed))
	}
}

func TestPromptAdoption_UserAccepts_Adopts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, home, "my-skill", skillFrontmatter)

	cfg := &config.Config{}
	st := emptyState()
	toolSet := []tools.Tool{mockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "unmanaged skill") {
		t.Errorf("expected prompt text in output, got: %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "adopt") {
		t.Errorf("expected 'adopt' mention in output, got: %q", output)
	}

	installed, ok := st.Installed["my-skill"]
	if !ok {
		t.Fatal("expected my-skill in state.Installed after acceptance")
	}
	if installed.Origin != state.OriginLocal {
		t.Errorf("origin = %q, want %q", installed.Origin, state.OriginLocal)
	}
}

func TestPromptAdoption_UserDeclines_SkipsAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, home, "my-skill", skillFrontmatter)

	cfg := &config.Config{}
	st := emptyState()
	toolSet := []tools.Tool{mockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("n\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "unmanaged skill") {
		t.Errorf("expected prompt text in output, got: %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "skip") {
		t.Errorf("expected 'skip' mention in output, got: %q", output)
	}

	if _, ok := st.Installed["my-skill"]; ok {
		t.Error("my-skill should NOT be in state.Installed after decline")
	}
}

func TestPromptAdoption_IgnoresConfigMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, home, "my-skill", skillFrontmatter)

	// Set adoption mode to "off" — PromptAdoption must ignore this.
	cfg := &config.Config{
		Adoption: config.AdoptionConfig{Mode: "off"},
	}
	st := emptyState()
	toolSet := []tools.Tool{mockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := st.Installed["my-skill"]; !ok {
		t.Error("my-skill should be adopted even when cfg.Adoption.Mode == 'off'")
	}
}
