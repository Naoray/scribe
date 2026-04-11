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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// adoptMockTool satisfies tools.Tool for adoption tests; no-op on disk.
type adoptMockTool struct {
	name string
}

func (m *adoptMockTool) Name() string                            { return m.name }
func (m *adoptMockTool) Detect() bool                           { return true }
func (m *adoptMockTool) Install(_, _ string) ([]string, error)  { return nil, nil }
func (m *adoptMockTool) Uninstall(_ string) error               { return nil }

var _ tools.Tool = (*adoptMockTool)(nil)

// writeSkillForAdoption creates <parent>/<name>/SKILL.md with content.
func writeSkillForAdoption(t *testing.T, parent, name, content string) {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// freshState returns a State with no installed skills.
func freshState() *state.State {
	return &state.State{
		SchemaVersion: 4,
		Installed:     make(map[string]state.InstalledSkill),
	}
}

const minimalSkillMD = `---
name: my-skill
description: test
---
body
`

// ---------------------------------------------------------------------------
// TestPromptAdoption_NoCandidates_NoOutput
// ---------------------------------------------------------------------------

// TestPromptAdoption_NoCandidates_NoOutput verifies that when no unmanaged
// skills exist the prompt does not fire and nothing is written to out.
func TestPromptAdoption_NoCandidates_NoOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Empty ~/.claude/skills/ — creates the dir but no skill subdirs.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	st := freshState()
	toolSet := []tools.Tool{&adoptMockTool{name: "mock"}}

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

// ---------------------------------------------------------------------------
// TestPromptAdoption_UserAccepts_Adopts
// ---------------------------------------------------------------------------

// TestPromptAdoption_UserAccepts_Adopts verifies that "y" input causes the
// candidate to be adopted and recorded in state with OriginLocal.
func TestPromptAdoption_UserAccepts_Adopts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkillForAdoption(t, claudeSkills, "my-skill", minimalSkillMD)

	cfg := &config.Config{}
	st := freshState()
	toolSet := []tools.Tool{&adoptMockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "unmanaged skill") {
		t.Errorf("expected prompt text mentioning 'unmanaged skill' in output, got: %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "adopt") {
		t.Errorf("expected 'adopt' mention in output, got: %q", output)
	}

	installed, ok := st.Installed["my-skill"]
	if !ok {
		t.Fatal("expected my-skill in state.Installed after user accepted")
	}
	if installed.Origin != state.OriginLocal {
		t.Errorf("origin = %q, want %q", installed.Origin, state.OriginLocal)
	}
}

// ---------------------------------------------------------------------------
// TestPromptAdoption_UserDeclines_SkipsAdoption
// ---------------------------------------------------------------------------

// TestPromptAdoption_UserDeclines_SkipsAdoption verifies that "n" input shows
// the prompt but leaves state unchanged and prints a skip message.
func TestPromptAdoption_UserDeclines_SkipsAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkillForAdoption(t, claudeSkills, "my-skill", minimalSkillMD)

	cfg := &config.Config{}
	st := freshState()
	toolSet := []tools.Tool{&adoptMockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("n\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "unmanaged skill") {
		t.Errorf("expected prompt text in output, got: %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "skip") {
		t.Errorf("expected skip message in output, got: %q", output)
	}
	if _, ok := st.Installed["my-skill"]; ok {
		t.Error("my-skill must NOT be in state.Installed after user declined")
	}
}

// ---------------------------------------------------------------------------
// TestPromptAdoption_IgnoresConfigMode
// ---------------------------------------------------------------------------

// TestPromptAdoption_IgnoresConfigMode verifies spec §8: firstrun always
// prompts regardless of cfg.Adoption.Mode (even "off").
func TestPromptAdoption_IgnoresConfigMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkillForAdoption(t, claudeSkills, "my-skill", minimalSkillMD)

	cfg := &config.Config{
		Adoption: config.AdoptionConfig{Mode: "off"},
	}
	st := freshState()
	toolSet := []tools.Tool{&adoptMockTool{name: "mock"}}

	var out bytes.Buffer
	if err := firstrun.PromptAdoption(cfg, st, toolSet, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Prompt must have fired even though mode="off".
	if !strings.Contains(out.String(), "unmanaged skill") {
		t.Errorf("expected prompt even with mode=off, got: %q", out.String())
	}

	if _, ok := st.Installed["my-skill"]; !ok {
		t.Error("my-skill should be adopted even when cfg.Adoption.Mode == 'off'")
	}
}
