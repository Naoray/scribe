package projectmigrate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestWriteAndLoadSnapshot_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "project")
	file := filepath.Join(project, ".scribe.yaml")
	snapshot := Snapshot{
		Version:   1,
		Timestamp: time.Date(2026, 5, 2, 12, 34, 56, 789000000, time.UTC),
		Discovery: Discovery{
			GlobalSymlinks: []GlobalSymlink{{
				Tool:          "claude",
				Skill:         "tdd",
				Path:          filepath.Join(home, ".claude", "skills", "tdd"),
				CanonicalPath: filepath.Join(home, ".scribe", "skills", "tdd"),
			}},
			Projects: []ProjectCandidate{{Path: project, Source: "search_root"}},
			Skills:   []string{"tdd"},
		},
		Plan: MigrationPlan{
			ProjectFiles: []ProjectChange{{
				Project: project,
				File:    file,
				Skills:  []string{"tdd"},
				Changed: true,
			}},
		},
		PreviousProjectFiles: map[string][]byte{
			file: []byte("add:\n  - old\n"),
		},
		PreviousProjections: map[string][]state.ProjectionEntry{
			"tdd": {{Project: "", Tools: []string{"claude"}}},
		},
	}

	path, err := WriteSnapshot(snapshot)
	if err != nil {
		t.Fatalf("WriteSnapshot() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(*loaded, snapshot) {
		t.Fatalf("snapshot = %#v, want %#v", *loaded, snapshot)
	}
}

func TestSnapshotRetention_KeepsLatest10(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		if _, err := WriteSnapshot(Snapshot{
			Version:   1,
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
		}); err != nil {
			t.Fatalf("WriteSnapshot(%d) error = %v", i, err)
		}
	}
	dir, err := state.MigrationSnapshotsDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("snapshot count = %d, want 10", len(entries))
	}
	latest, err := LatestSnapshotPath()
	if err != nil {
		t.Fatalf("LatestSnapshotPath() error = %v", err)
	}
	if filepath.Base(latest) != "2026-05-02T12:00:00.011Z.json" {
		t.Fatalf("latest = %s, want final snapshot", filepath.Base(latest))
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-05-02T12:00:00.000Z.json")); !os.IsNotExist(err) {
		t.Fatalf("oldest snapshot should be pruned, stat err = %v", err)
	}
}

func TestApplyWritesSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "project")
	store := filepath.Join(home, ".scribe", "skills")
	link := filepath.Join(home, ".claude", "skills", "tdd")
	mustMkdir(t, filepath.Join(store, "tdd"))
	mustMkdir(t, filepath.Dir(link))
	mustMkdir(t, project)
	mustSymlink(t, filepath.Join(store, "tdd"), link)
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"tdd": {Projections: []state.ProjectionEntry{{Project: "", Tools: []string{"claude"}}}},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	discovery := Discovery{
		GlobalSymlinks: []GlobalSymlink{{
			Tool:          "claude",
			Skill:         "tdd",
			Path:          link,
			CanonicalPath: filepath.Join(store, "tdd"),
		}},
		Projects: []ProjectCandidate{{Path: project, Source: "search_root"}},
		Skills:   []string{"tdd"},
	}
	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := Apply(plan, discovery.Projects); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	path, err := LatestSnapshotPath()
	if err != nil {
		t.Fatalf("LatestSnapshotPath() error = %v", err)
	}
	snapshot, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	file := filepath.Join(project, ".scribe.yaml")
	if _, ok := snapshot.PreviousProjectFiles[file]; !ok {
		t.Fatalf("snapshot missing previous project file entry for %s", file)
	}
	if got := snapshot.PreviousProjections["tdd"]; !reflect.DeepEqual(got, []state.ProjectionEntry{{Project: "", Tools: []string{"claude"}}}) {
		t.Fatalf("previous projections = %#v, want legacy projection", got)
	}
}

func TestApplyDeletesSnapshotOnFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "project")
	store := filepath.Join(home, ".scribe", "skills")
	linkDir := filepath.Join(home, ".claude", "skills")
	link := filepath.Join(linkDir, "tdd")
	mustMkdir(t, filepath.Join(store, "tdd"))
	mustMkdir(t, linkDir)
	mustMkdir(t, project)
	mustSymlink(t, filepath.Join(store, "tdd"), link)
	if err := os.Chmod(linkDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(linkDir, 0o700)
	})
	discovery := Discovery{
		GlobalSymlinks: []GlobalSymlink{{
			Tool:          "claude",
			Skill:         "tdd",
			Path:          link,
			CanonicalPath: filepath.Join(store, "tdd"),
		}},
		Projects: []ProjectCandidate{{Path: project, Source: "search_root"}},
		Skills:   []string{"tdd"},
	}
	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := Apply(plan, discovery.Projects); err == nil {
		t.Fatal("Apply() error = nil, want remove failure")
	}
	dir, err := state.MigrationSnapshotsDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("snapshot count = %d, want 0 after failed apply", len(entries))
	}
}
