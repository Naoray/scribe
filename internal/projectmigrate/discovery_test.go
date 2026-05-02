package projectmigrate

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/Naoray/scribe/internal/state"
)

func TestDiscoverGlobalSymlinksFindsStoreLinksOnly(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	store := filepath.Join(home, ".scribe", "skills")
	mustMkdir(t, filepath.Join(store, "tdd"))
	mustMkdir(t, filepath.Join(store, "review"))
	mustMkdir(t, filepath.Join(home, ".claude", "skills"))
	mustMkdir(t, filepath.Join(home, ".codex", "skills"))

	mustSymlink(t, filepath.Join(store, "tdd"), filepath.Join(home, ".claude", "skills", "tdd"))
	mustSymlink(t, filepath.Join(store, "review"), filepath.Join(home, ".codex", "skills", "review"))
	mustSymlink(t, filepath.Join(tmp, "elsewhere"), filepath.Join(home, ".claude", "skills", "external"))
	if err := os.WriteFile(filepath.Join(home, ".codex", "skills", "real"), []byte("not a symlink"), 0o644); err != nil {
		t.Fatal(err)
	}

	links, err := DiscoverGlobalSymlinks(home, store, []string{"claude", "codex"})
	if err != nil {
		t.Fatalf("DiscoverGlobalSymlinks() error = %v", err)
	}

	got := []string{}
	for _, link := range links {
		got = append(got, link.Tool+":"+link.Skill)
		if link.CanonicalPath == "" || link.Path == "" {
			t.Fatalf("link paths should be populated: %#v", link)
		}
	}
	want := []string{"claude:tdd", "codex:review"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

func TestDiscoverCandidateProjectsCombinesSearchRootsAndState(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "workspace")
	app := filepath.Join(root, "app")
	nested := filepath.Join(root, "nested")
	fromState := filepath.Join(tmp, "from-state")
	mustMkdir(t, app)
	mustMkdir(t, nested)
	mustMkdir(t, fromState)
	if err := os.WriteFile(filepath.Join(app, ".scribe.yaml"), []byte("add:\n  - tdd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".scribe.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"tdd": {
				Projections: []state.ProjectionEntry{{Project: fromState, Tools: []string{"claude"}}},
			},
		},
	}

	projects, err := DiscoverCandidateProjects([]string{root}, st)
	if err != nil {
		t.Fatalf("DiscoverCandidateProjects() error = %v", err)
	}

	got := []string{}
	for _, project := range projects {
		got = append(got, project.Path)
	}
	want := []string{fromState, app, nested}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
}

func TestDiscoverCandidateProjectsIncludesEmptySearchRoot(t *testing.T) {
	root := t.TempDir()

	projects, err := DiscoverCandidateProjects([]string{root}, nil)
	if err != nil {
		t.Fatalf("DiscoverCandidateProjects() error = %v", err)
	}

	if len(projects) != 1 || projects[0].Path != root || projects[0].Source != "search_root" {
		t.Fatalf("projects = %#v, want search root candidate", projects)
	}
}

func TestDiscoverCandidateProjectsSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "app")
	hiddenLooksLikeProject := filepath.Join(root, ".cache", "fake-project")
	mustMkdir(t, visible)
	mustMkdir(t, hiddenLooksLikeProject)
	if err := os.WriteFile(filepath.Join(visible, ".scribe.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenLooksLikeProject, ".scribe.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverCandidateProjects([]string{root}, nil)
	if err != nil {
		t.Fatalf("DiscoverCandidateProjects() error = %v, want nil", err)
	}

	got := []string{}
	for _, project := range projects {
		got = append(got, project.Path)
	}
	want := []string{visible}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
}

func TestDiscoverCandidateProjectsTolerantOfTransientErrors(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	weird := filepath.Join(root, "weird")
	mustMkdir(t, app)
	mustMkdir(t, weird)
	if err := os.WriteFile(filepath.Join(app, ".scribe.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(weird, "missing-target"), filepath.Join(weird, "broken"))

	projects, err := DiscoverCandidateProjects([]string{root}, nil)
	if err != nil {
		t.Fatalf("DiscoverCandidateProjects() error = %v, want nil", err)
	}

	got := []string{}
	for _, project := range projects {
		got = append(got, project.Path)
	}
	want := []string{app}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
}

func TestDiscoverCandidateProjectsSkipsUnreadableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	root := t.TempDir()
	app := filepath.Join(root, "app")
	locked := filepath.Join(root, "locked")
	mustMkdir(t, app)
	mustMkdir(t, locked)
	if err := os.WriteFile(filepath.Join(app, ".scribe.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate a macOS-style protected dir (e.g. ~/.Trash).
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	projects, err := DiscoverCandidateProjects([]string{root}, nil)
	if err != nil {
		t.Fatalf("DiscoverCandidateProjects() error = %v, want nil", err)
	}

	got := []string{}
	for _, project := range projects {
		got = append(got, project.Path)
	}
	want := []string{app}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatal(err)
	}
}
