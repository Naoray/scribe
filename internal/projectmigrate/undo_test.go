package projectmigrate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
)

func TestUndo_RoundTrip_ByteEqual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project, link := setupUndoFixture(t, home, "claude", "tdd", []byte("add:\n  - old\n"))
	beforeState, err := os.ReadFile(filepath.Join(home, ".scribe", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeProject, err := os.ReadFile(filepath.Join(project, projectfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	discovery := undoDiscovery(home, project, link, "claude", "tdd")
	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := Apply(plan, discovery.Projects); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	snapshotPath, err := LatestSnapshotPath()
	if err != nil {
		t.Fatalf("LatestSnapshotPath() error = %v", err)
	}
	snapshot, err := LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	result, err := Undo(snapshot, snapshotPath)
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	afterState, err := os.ReadFile(filepath.Join(home, ".scribe", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	afterProject, err := os.ReadFile(filepath.Join(project, projectfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterState, beforeState) {
		t.Fatalf("state bytes changed after undo\nbefore=%s\nafter=%s", beforeState, afterState)
	}
	if !bytes.Equal(afterProject, beforeProject) {
		t.Fatalf("project bytes changed after undo\nbefore=%s\nafter=%s", beforeProject, afterProject)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should be restored: %v", err)
	}
	if result.RestoredLinks != 1 || result.RestoredProjectFiles != 1 {
		t.Fatalf("result = %#v, want restored link and project file", result)
	}
}

func TestUndo_NoSnapshot_Errors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := LatestSnapshotPath(); err == nil {
		t.Fatal("LatestSnapshotPath() error = nil, want no migration to undo")
	}
}

func TestUndo_RestoresMissingProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project, link := setupUndoFixture(t, home, "claude", "tdd", nil)
	discovery := undoDiscovery(home, project, link, "claude", "tdd")
	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := Apply(plan, discovery.Projects); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	snapshotPath, err := LatestSnapshotPath()
	if err != nil {
		t.Fatalf("LatestSnapshotPath() error = %v", err)
	}
	snapshot, err := LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	result, err := Undo(snapshot, snapshotPath)
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, projectfile.Filename)); !os.IsNotExist(err) {
		t.Fatalf("project file should be deleted after undo, stat err = %v", err)
	}
	if result.DeletedProjectFiles != 1 {
		t.Fatalf("DeletedProjectFiles = %d, want 1", result.DeletedProjectFiles)
	}
}

func setupUndoFixture(t *testing.T, home, tool, skill string, projectFile []byte) (project, link string) {
	t.Helper()
	project = filepath.Join(home, "project")
	storeSkill := filepath.Join(home, ".scribe", "skills", skill)
	link = filepath.Join(home, "."+tool, "skills", skill)
	for _, dir := range []string{project, storeSkill, filepath.Dir(link)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if projectFile != nil {
		if err := os.WriteFile(filepath.Join(project, projectfile.Filename), projectFile, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(storeSkill, "SKILL.md"), []byte("---\nname: "+skill+"\ndescription: Test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(storeSkill, link); err != nil {
		t.Fatal(err)
	}
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			skill: {Projections: []state.ProjectionEntry{{Project: "", Tools: []string{tool}}}},
		},
		Kits:               map[string]state.InstalledKit{},
		Snippets:           map[string]state.InstalledSnippet{},
		RemovedByUser:      []state.RemovedSkill{},
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]state.RegistryFailure{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	return project, link
}

func undoDiscovery(home, project, link, tool, skill string) Discovery {
	return Discovery{
		GlobalSymlinks: []GlobalSymlink{{
			Tool:          tool,
			Skill:         skill,
			Path:          link,
			CanonicalPath: filepath.Join(home, ".scribe", "skills", skill),
		}},
		Projects: []ProjectCandidate{{Path: project, Source: "search_root"}},
		Skills:   []string{skill},
	}
}
