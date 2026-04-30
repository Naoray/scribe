package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestSkill creates a minimal skill dir with SKILL.md.
func writeTestSkill(t *testing.T, parentDir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(parentDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupAdoptHome creates a temp HOME with ~/.claude/skills/<name> and returns HOME.
func setupAdoptHome(t *testing.T, skillName, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeTestSkill(t, claudeSkills, skillName, content)

	// Create scribe dir so state + config load don't error.
	if err := os.MkdirAll(filepath.Join(home, ".scribe"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestAdopt_DryRunWritesNothing verifies --dry-run never mutates state.json.
func TestAdopt_DryRunWritesNothing(t *testing.T) {
	home := setupAdoptHome(t, "dry-skill", "# dry-skill\ncontent")

	stateFile := filepath.Join(home, ".scribe", "state.json")

	// Capture stdout.
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newAdoptCommand()
	cmd.SetArgs([]string{"--dry-run", "--json"})
	err := cmd.Execute()

	w.Close()
	os.Stdout = oldOut

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	// state.json must not exist (no state was written).
	if _, statErr := os.Stat(stateFile); statErr == nil {
		data, _ := os.ReadFile(stateFile)
		t.Errorf("state.json should not exist after --dry-run, but found: %s", string(data))
	}

	// Output must be valid JSON with dry_run: true.
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if data, ok := plan["data"].(map[string]any); ok {
		plan = data
	}
	if plan["dry_run"] != true {
		t.Errorf("expected dry_run=true in JSON output, got: %v", plan["dry_run"])
	}
	if _, ok := plan["adopt"]; !ok {
		t.Error("expected 'adopt' key in JSON output")
	}
}

// TestAdopt_YesForcesAuto verifies --yes adopts candidates without prompting.
// Source is in a custom path (not ~/.claude/skills/) so tool.Install creates
// fresh symlinks without needing to remove a directory first.
func TestAdopt_YesForcesAuto(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Place skill in a custom dir (not a tool-facing dir) so Install creates new symlinks.
	customDir := filepath.Join(home, "my-skills")
	writeTestSkill(t, customDir, "auto-skill", "# auto-skill\ncontent")

	// Write a config pointing at customDir as an extra adoption path.
	// Disable all builtin tools so Install only runs against tools detected in temp home.
	// Gemini is detected by binary presence; explicitly disable it to avoid CLI calls.
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgContent := "adoption:\n  mode: auto\n  paths:\n    - " + customDir + "\ntools:\n  - name: gemini\n    enabled: false\n  - name: cursor\n    enabled: false\n  - name: codex\n    enabled: false\n"
	if err := os.WriteFile(filepath.Join(scribeDir, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Redirect stdout to suppress formatter output.
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newAdoptCommand()
	cmd.SetArgs([]string{"--yes", "--json"})
	err := cmd.Execute()

	w.Close()
	os.Stdout = oldOut

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, buf.String())
	}

	// state.json should now exist and contain auto-skill with origin=local.
	stateFile := filepath.Join(home, ".scribe", "state.json")
	data, readErr := os.ReadFile(stateFile)
	if readErr != nil {
		t.Fatalf("state.json not written: %v", readErr)
	}
	var stateMap map[string]any
	if err := json.Unmarshal(data, &stateMap); err != nil {
		t.Fatalf("state.json is not valid JSON: %v", err)
	}
	installed, ok := stateMap["installed"].(map[string]any)
	if !ok {
		t.Fatalf("state.json missing 'installed' map: %v", stateMap)
	}
	skillEntry, ok := installed["auto-skill"].(map[string]any)
	if !ok {
		t.Fatalf("auto-skill not found in installed: %v", installed)
	}
	if skillEntry["origin"] != "local" {
		t.Errorf("expected origin=local, got: %v", skillEntry["origin"])
	}
}

// TestAdopt_JSONStructure verifies the dry-run JSON shape for a known fixture.
func TestAdopt_JSONStructure(t *testing.T) {
	setupAdoptHome(t, "shape-skill", "# shape-skill\ncontent")

	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newAdoptCommand()
	cmd.SetArgs([]string{"--dry-run", "--json", "--verbose"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	os.Stdout = oldOut

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var env struct {
		Data dryRunPlan `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	plan := env.Data
	if !plan.DryRun {
		t.Error("expected dry_run=true")
	}
	if len(plan.Adopt) != 1 {
		t.Fatalf("expected 1 adopt entry, got %d", len(plan.Adopt))
	}
	if plan.Adopt[0].Name != "shape-skill" {
		t.Errorf("expected shape-skill, got %q", plan.Adopt[0].Name)
	}
	if plan.Adopt[0].LocalPath == "" {
		t.Error("expected LocalPath populated with --verbose")
	}
}

// TestAdopt_UnknownNameExitCode verifies that --yes with an unknown name returns an error.
func TestAdopt_UnknownNameExitCode(t *testing.T) {
	setupAdoptHome(t, "real-skill", "# real-skill\ncontent")

	cmd := newAdoptCommand()
	cmd.SetArgs([]string{"--yes", "nonexistent"})
	err := cmd.RunE(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected non-nil error for unknown candidate name")
	}
}

// TestAdopt_NonTTYNoYesErrors verifies non-interactive mode without --yes returns an error.
// Go test binaries have non-TTY stdin/stdout by default, so the non-TTY guard fires
// when no --yes, --json, or --dry-run flags are passed.
func TestAdopt_NonTTYNoYesErrors(t *testing.T) {
	setupAdoptHome(t, "some-skill", "# some-skill\ncontent")

	cmd := newAdoptCommand()
	cmd.SetArgs([]string{}) // no --yes, no --json, no --dry-run

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "non-interactive") && !strings.Contains(msg, "--yes") {
		t.Errorf("expected error mentioning non-interactive or --yes, got: %q", msg)
	}
}
