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

func TestReconcileUsesProjectRootForCodexProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", os.Getenv("PATH"))
	projectRoot := t.TempDir()

	canonical, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)

	projectPath := filepath.Join(projectRoot, ".codex", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(canonical, projectPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {
			Revision:     1,
			Tools:        []string{"codex"},
			ToolsMode:    state.ToolsModePinned,
			ManagedPaths: []string{projectPath},
		},
	}}

	engine := reconcile.Engine{
		Tools:       []tools.Tool{tools.CodexTool{}},
		ProjectRoot: projectRoot,
		Now:         func() time.Time { return time.Unix(1, 0).UTC() },
	}
	summary, actions, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Installed != 0 || summary.Removed != 0 || summary.Relinked != 0 || len(summary.Conflicts) != 0 {
		t.Fatalf("summary = %+v, want no changes", summary)
	}
	if len(actions) != 1 || actions[0].Kind != reconcile.ActionUnchanged || actions[0].Path != projectPath {
		t.Fatalf("actions = %+v, want unchanged project projection", actions)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", "recap")); !os.IsNotExist(err) {
		t.Fatalf("global codex projection exists or stat failed: %v", err)
	}
	if got := st.Installed["recap"].ManagedPaths; len(got) != 1 || got[0] != projectPath {
		t.Fatalf("ManagedPaths = %v, want [%s]", got, projectPath)
	}
}

func TestReconcileUsesGlobalClaudeProjectionForBootstrapSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()

	canonical, err := tools.WriteToStore("scribe", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# scribe\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"scribe": {Revision: 1, Tools: []string{"claude"}, ToolsMode: state.ToolsModePinned},
	}}

	engine := reconcile.Engine{
		Tools:       []tools.Tool{tools.ClaudeTool{}},
		ProjectRoot: projectRoot,
		Now:         func() time.Time { return time.Unix(1, 0).UTC() },
	}
	summary, actions, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Installed != 1 || summary.Removed != 0 || summary.Relinked != 0 || len(summary.Conflicts) != 0 {
		t.Fatalf("summary = %+v, want one global install", summary)
	}

	globalPath := filepath.Join(home, ".claude", "skills", "scribe")
	if len(actions) != 1 || actions[0].Kind != reconcile.ActionInstalled || actions[0].Path != globalPath {
		t.Fatalf("actions = %+v, want installed global projection %s", actions, globalPath)
	}
	if resolved, err := filepath.EvalSymlinks(globalPath); err != nil || resolved != canonical {
		t.Fatalf("global claude skill link = %q, %v; want %q", resolved, err, canonical)
	}
	projectPath := filepath.Join(projectRoot, ".claude", "skills", "scribe")
	if _, err := os.Lstat(projectPath); !os.IsNotExist(err) {
		t.Fatalf("project-local claude projection exists or stat failed: %v", err)
	}
	if got := st.Installed["scribe"].ManagedPaths; len(got) != 1 || got[0] != globalPath {
		t.Fatalf("ManagedPaths = %v, want [%s]", got, globalPath)
	}
}

func TestReconcileClaudeUnchangedOnSecondPass(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonical, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}})
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	canonical, _ = filepath.EvalSymlinks(canonical)

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"recap": {Revision: 1, Tools: []string{"claude"}, ToolsMode: state.ToolsModePinned},
	}}

	engine := reconcile.Engine{Tools: []tools.Tool{tools.ClaudeTool{}}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	if _, _, err := engine.Run(st); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	link := filepath.Join(home, ".claude", "skills", "recap")
	if resolved, err := filepath.EvalSymlinks(link); err != nil || resolved != canonical {
		t.Fatalf("claude skill link = %q, %v; want %q", resolved, err, canonical)
	}

	summary, actions, err := engine.Run(st)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if summary.Installed != 0 || summary.Relinked != 0 || len(summary.Conflicts) != 0 {
		t.Fatalf("second pass summary = %+v, want no changes", summary)
	}
	if len(actions) != 1 || actions[0].Kind != reconcile.ActionUnchanged {
		t.Fatalf("actions = %+v, want single Unchanged", actions)
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

func TestReconcileSkipsPackages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Stage a tree package as if sync wrote it.
	pkgsDir, err := tools.PackagesDir()
	if err != nil {
		t.Fatalf("PackagesDir: %v", err)
	}
	pkgDir := filepath.Join(pkgsDir, "gstack")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "SKILL.md"), []byte("# pkg\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"gstack": {Revision: 1, Kind: state.KindPackage, Tools: []string{}, Paths: []string{}},
	}}

	engine := reconcile.Engine{Tools: []tools.Tool{tools.ClaudeTool{}, tools.CodexTool{}}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	summary, actions, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No projections should be attempted for a package.
	if summary.Installed != 0 {
		t.Fatalf("Installed = %d, want 0 for a package", summary.Installed)
	}
	if summary.Relinked != 0 || summary.Removed != 0 {
		t.Fatalf("summary = %+v, want all-zero for a package", summary)
	}
	for _, a := range actions {
		if a.Name == "gstack" {
			t.Fatalf("unexpected action for package: %+v", a)
		}
	}

	// No tool-side symlinks should have been created.
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "gstack")); err == nil {
		t.Error("claude skills/gstack symlink was created for a package")
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", "gstack")); err == nil {
		t.Error("codex skills/gstack symlink was created for a package")
	}
}

func TestReconcileRemovesStalePackageProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Simulate a legacy install: skill was projected into ~/.claude/skills/
	// and then reclassified into packages/. Reconcile should clean the
	// stale projection up next pass.
	pkgsDir, _ := tools.PackagesDir()
	pkgDir := filepath.Join(pkgsDir, "gstack")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "SKILL.md"), []byte("# pkg\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stale := filepath.Join(home, ".claude", "skills", "gstack")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(pkgDir, stale); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	st := &state.State{SchemaVersion: 4, Installed: map[string]state.InstalledSkill{
		"gstack": {
			Revision:     1,
			Kind:         state.KindPackage,
			ManagedPaths: []string{stale},
		},
	}}
	engine := reconcile.Engine{Tools: []tools.Tool{tools.ClaudeTool{}}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	summary, _, err := engine.Run(st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Removed != 1 {
		t.Fatalf("Removed = %d, want 1", summary.Removed)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Errorf("stale package projection still exists: %v", err)
	}
}
