package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// seedSkillEnv provisions a tmp HOME with an installed skill `commit` linked
// into the builtin tools and returns the test Command ready for Execute.
func seedSkillEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Config: pin the exact set of tools this test cares about. Gemini is
	// explicitly disabled so a dev machine with gemini-cli installed can't
	// leak into ResolveActive and break the pinning assertions below.
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "claude", Type: tools.ToolTypeBuiltin, Enabled: true},
			{Name: "cursor", Type: tools.ToolTypeBuiltin, Enabled: true},
			{Name: "codex", Type: tools.ToolTypeBuiltin, Enabled: true},
			{Name: "gemini", Type: tools.ToolTypeBuiltin, Enabled: false},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Write canonical store via WriteToStore so that later Install calls succeed.
	files := []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# commit")}}
	canonical, err := tools.WriteToStore("commit", files)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	// Install into claude + cursor up front so "currentTools" matches reality.
	claude := tools.ClaudeTool{}
	cursor := tools.CursorTool{}
	paths := []string{}
	p1, err := claude.Install("commit", canonical)
	if err != nil {
		t.Fatalf("claude install: %v", err)
	}
	paths = append(paths, p1...)
	p2, err := cursor.Install("commit", canonical)
	if err != nil {
		t.Fatalf("cursor install: %v", err)
	}
	paths = append(paths, p2...)

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"commit": {
				Revision:      1,
				InstalledHash: "abc",
				Tools:         []string{"claude", "cursor"},
				ToolsMode:     state.ToolsModeInherit,
				Paths:         paths,
				ManagedPaths:  append([]string(nil), paths...),
				InstalledAt:   time.Now().UTC(),
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

func TestSkillEdit_PinToSubset(t *testing.T) {
	seedSkillEnv(t)

	cmd := newSkillEditCommand()
	cmd.SetArgs([]string{"commit", "--tools", "claude", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := st.Installed["commit"]
	if !ok {
		t.Fatal("commit missing from state")
	}
	if got.ToolsMode != state.ToolsModePinned {
		t.Errorf("ToolsMode = %q, want pinned", got.ToolsMode)
	}
	if !reflect.DeepEqual(got.Tools, []string{"claude"}) {
		t.Errorf("Tools = %v, want [claude]", got.Tools)
	}

	// Cursor symlink should be gone after physical uninstall.
	home := os.Getenv("HOME")
	cursorPath := filepath.Join(home, ".cursor", "rules", "commit.mdc")
	if _, err := os.Lstat(cursorPath); err == nil {
		t.Errorf("cursor rule still present at %s", cursorPath)
	}
}

func TestSkillEdit_AddTool(t *testing.T) {
	seedSkillEnv(t)

	cmd := newSkillEditCommand()
	cmd.SetArgs([]string{"commit", "--add", "codex", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	st, _ := state.Load()
	got := st.Installed["commit"]
	if got.ToolsMode != state.ToolsModePinned {
		t.Errorf("ToolsMode = %q, want pinned after --add", got.ToolsMode)
	}
	want := []string{"claude", "cursor", "codex"}
	if !reflect.DeepEqual(got.Tools, want) {
		t.Errorf("Tools = %v, want %v", got.Tools, want)
	}
}

func TestSkillEdit_Inherit(t *testing.T) {
	seedSkillEnv(t)

	// First pin it.
	pin := newSkillEditCommand()
	pin.SetArgs([]string{"commit", "--tools", "claude", "--json"})
	if err := pin.Execute(); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// Then revert.
	revert := newSkillEditCommand()
	revert.SetArgs([]string{"commit", "--inherit", "--json"})
	if err := revert.Execute(); err != nil {
		t.Fatalf("revert: %v", err)
	}

	st, _ := state.Load()
	got := st.Installed["commit"]
	if got.ToolsMode != state.ToolsModeInherit {
		t.Errorf("ToolsMode = %q, want inherit", got.ToolsMode)
	}
	// Inherit mode replays ResolveActive, which sorts alphabetically.
	want := []string{"claude", "codex", "cursor"}
	if !reflect.DeepEqual(got.Tools, want) {
		t.Errorf("Tools = %v, want %v", got.Tools, want)
	}
}

func TestSkillEdit_RejectsUnknownTool(t *testing.T) {
	seedSkillEnv(t)

	cmd := newSkillEditCommand()
	cmd.SetArgs([]string{"commit", "--tools", "imaginary", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestSkillEdit_RejectsMissingSkill(t *testing.T) {
	seedSkillEnv(t)

	cmd := newSkillEditCommand()
	cmd.SetArgs([]string{"nonexistent", "--tools", "claude", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
}

func TestSkillRepair_ManagedWins(t *testing.T) {
	seedSkillEnv(t)

	home := os.Getenv("HOME")
	canonical := filepath.Join(home, ".scribe", "skills", "commit")
	codexPath := filepath.Join(home, ".codex", "skills", "commit")
	if err := os.MkdirAll(codexPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexPath, "SKILL.md"), []byte("# commit\nlocal drift\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	skill := st.Installed["commit"]
	skill.Tools = []string{"claude", "cursor", "codex"}
	skill.ToolsMode = state.ToolsModePinned
	skill.Conflicts = []state.ProjectionConflict{{Tool: "codex", Path: codexPath, FoundHash: "deadbeef", SeenAt: time.Now().UTC()}}
	st.Installed["commit"] = skill
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cmd := newSkillRepairCommand()
	cmd.SetArgs([]string{"commit", "--tool", "codex", "--from", "managed", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(codexPath)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)
	if resolved != canonical {
		t.Fatalf("codex path resolves to %q, want %q", resolved, canonical)
	}

	st, err = state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installed["commit"].Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, want cleared", st.Installed["commit"].Conflicts)
	}
}

func TestSkillRepair_ToolWins(t *testing.T) {
	seedSkillEnv(t)

	home := os.Getenv("HOME")
	codexPath := filepath.Join(home, ".codex", "skills", "commit")
	if err := os.MkdirAll(codexPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	toolContent := []byte("# commit\npromoted\n")
	if err := os.WriteFile(filepath.Join(codexPath, "SKILL.md"), toolContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	skill := st.Installed["commit"]
	skill.Tools = []string{"claude", "cursor", "codex"}
	skill.ToolsMode = state.ToolsModePinned
	skill.Conflicts = []state.ProjectionConflict{{Tool: "codex", Path: codexPath, FoundHash: "deadbeef", SeenAt: time.Now().UTC()}}
	st.Installed["commit"] = skill
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cmd := newSkillRepairCommand()
	cmd.SetArgs([]string{"commit", "--tool", "codex", "--from", "tool", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".scribe", "skills", "commit", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(toolContent) {
		t.Fatalf("canonical SKILL.md = %q, want %q", string(data), string(toolContent))
	}
}
