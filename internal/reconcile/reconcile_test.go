package reconcile_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/reconcile"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func TestReconcileRepairsMissingCodexProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", os.Getenv("PATH"))

	canonical, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {Revision: 1, Tools: []string{"codex"}, ToolsMode: state.ToolsModePinned},
	}}

	engine := reconcile.Engine{Tools: []tools.Tool{tools.CodexTool{}}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Installed != 1 {
		t.Fatalf("Installed = %d, want 1", summary.Installed)
	}

	path := filepath.Join(home, ".codex", "skills", "recap")
	if resolved, err := filepath.EvalSymlinks(path); err != nil || resolved != canonical {
		t.Fatalf("codex skill link = %q, %v; want %q", resolved, err, canonical)
	}
	if len(st.Installed["recap"].ManagedPaths) != 1 || st.Installed["recap"].ManagedPaths[0] != path {
		t.Fatalf("ManagedPaths = %v", st.Installed["recap"].ManagedPaths)
	}
}

func TestReconcileNormalizesSameHashDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonical, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\nsame\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)
	toolPath := filepath.Join(home, ".codex", "skills", "recap")
	if err := os.MkdirAll(toolPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolPath, "SKILL.md"), []byte("# recap\nsame\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {Revision: 1, Tools: []string{"codex"}, ToolsMode: state.ToolsModePinned},
	}}
	engine := reconcile.Engine{Tools: []tools.Tool{tools.CodexTool{}}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Relinked != 1 {
		t.Fatalf("Relinked = %d, want 1", summary.Relinked)
	}
	if resolved, err := filepath.EvalSymlinks(toolPath); err != nil || resolved != canonical {
		t.Fatalf("codex skill link = %q, %v; want %q", resolved, err, canonical)
	}
}

func TestReconcilePreservesDivergentDirectoryAsConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\ncanonical\n")}}); err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	toolPath := filepath.Join(home, ".codex", "skills", "recap")
	if err := os.MkdirAll(toolPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolPath, "SKILL.md"), []byte("# recap\nlocal drift\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {Revision: 1, Tools: []string{"codex"}, ToolsMode: state.ToolsModePinned},
	}}
	engine := reconcile.Engine{Tools: []tools.Tool{tools.CodexTool{}}, Now: func() time.Time { return time.Unix(2, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(summary.Conflicts) != 1 {
		t.Fatalf("Conflicts = %d, want 1", len(summary.Conflicts))
	}
	if len(st.Installed["recap"].ManagedPaths) != 0 {
		t.Fatalf("ManagedPaths = %v, want empty after divergent conflict", st.Installed["recap"].ManagedPaths)
	}
	info, err := os.Stat(toolPath)
	if err != nil || !info.IsDir() {
		t.Fatalf("toolPath stat = %v, %v; want preserved directory", info, err)
	}
}

func TestReconcileDetectsCodexSubfileDrift(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	files := []tools.SkillFile{
		{Path: "SKILL.md", Content: []byte("# recap\ncanonical\n")},
		{Path: "scripts/run.sh", Content: []byte("#!/bin/sh\necho canonical\n")},
	}
	if _, err := tools.WriteToStore("recap", files); err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	toolPath := filepath.Join(home, ".codex", "skills", "recap")
	if err := os.MkdirAll(filepath.Join(toolPath, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Identical SKILL.md — old hash strategy would call these equal.
	if err := os.WriteFile(filepath.Join(toolPath, "SKILL.md"), []byte("# recap\ncanonical\n"), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}
	// Drift buried in a subfile — must be caught by the tree hash.
	if err := os.WriteFile(filepath.Join(toolPath, "scripts", "run.sh"), []byte("#!/bin/sh\necho drifted\n"), 0o644); err != nil {
		t.Fatalf("WriteFile scripts/run.sh: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {Revision: 1, Tools: []string{"codex"}, ToolsMode: state.ToolsModePinned},
	}}
	engine := reconcile.Engine{Tools: []tools.Tool{tools.CodexTool{}}, Now: func() time.Time { return time.Unix(4, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Relinked != 0 {
		t.Fatalf("Relinked = %d, want 0 — subfile drift must not be silently relinked", summary.Relinked)
	}
	if len(summary.Conflicts) != 1 {
		t.Fatalf("Conflicts = %d, want 1 (subfile drift)", len(summary.Conflicts))
	}
}

func TestReconcileRemovesStaleManagedProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonical, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	toolPath := filepath.Join(home, ".codex", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(canonical, toolPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {
			Revision:     1,
			Tools:        []string{"codex"},
			ToolsMode:    state.ToolsModePinned,
			ManagedPaths: []string{toolPath},
		},
	}}
	engine := reconcile.Engine{Tools: nil, Now: func() time.Time { return time.Unix(3, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Removed != 1 {
		t.Fatalf("Removed = %d, want 1", summary.Removed)
	}
	if _, err := os.Lstat(toolPath); !os.IsNotExist(err) {
		t.Fatalf("toolPath still exists: %v", err)
	}
}
