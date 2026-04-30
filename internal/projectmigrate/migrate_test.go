package projectmigrate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Naoray/scribe/internal/projectfile"
)

func TestApplyWritesProjectFileRemovesGlobalSymlinksAndIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	store := filepath.Join(tmp, "home", ".scribe", "skills")
	link := filepath.Join(tmp, "home", ".claude", "skills", "tdd")
	mustMkdir(t, filepath.Join(store, "tdd"))
	mustMkdir(t, filepath.Dir(link))
	mustMkdir(t, project)
	mustSymlink(t, filepath.Join(store, "tdd"), link)

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
	result, err := Apply(plan, discovery.Projects)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.WroteProjectFiles != 1 || result.RemovedGlobalLinks != 1 {
		t.Fatalf("result = %#v, want one write and one removal", result)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("global symlink still exists or unexpected stat error: %v", err)
	}
	pf, err := projectfile.Load(filepath.Join(project, projectfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pf.Add, []string{"tdd"}) {
		t.Fatalf("project add = %v, want [tdd]", pf.Add)
	}

	plan, err = BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() second run error = %v", err)
	}
	result, err = Apply(plan, discovery.Projects)
	if err != nil {
		t.Fatalf("Apply() second run error = %v", err)
	}
	if result.WroteProjectFiles != 0 || result.RemovedGlobalLinks != 0 || result.SkippedGlobalLinks != 1 {
		t.Fatalf("second result = %#v, want idempotent no-op with skipped link", result)
	}
}

func TestMigrationPreservesScribeAgentGlobalSymlink(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	store := filepath.Join(home, ".scribe", "skills")
	project := filepath.Join(tmp, "project")
	normalLink := filepath.Join(home, ".claude", "skills", "tdd")
	scribeAgentLink := filepath.Join(home, ".claude", "skills", "scribe")

	mustMkdir(t, filepath.Join(store, "tdd"))
	mustMkdir(t, filepath.Join(store, "scribe"))
	mustMkdir(t, filepath.Dir(normalLink))
	mustMkdir(t, project)
	mustSymlink(t, filepath.Join(store, "tdd"), normalLink)
	mustSymlink(t, filepath.Join(store, "scribe"), scribeAgentLink)

	discovery, err := Discover(DiscoveryOptions{
		HomeDir:     home,
		StoreDir:    store,
		ToolNames:   []string{"claude"},
		SearchRoots: []string{project},
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !reflect.DeepEqual(discovery.Skills, []string{"tdd"}) {
		t.Fatalf("discovered skills = %v, want [tdd]", discovery.Skills)
	}

	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	result, err := Apply(plan, discovery.Projects)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if result.RemovedGlobalLinks != 1 {
		t.Fatalf("RemovedGlobalLinks = %d, want 1", result.RemovedGlobalLinks)
	}
	if _, err := os.Lstat(normalLink); !os.IsNotExist(err) {
		t.Fatalf("normal global symlink still exists or unexpected stat error: %v", err)
	}
	if _, err := os.Lstat(scribeAgentLink); err != nil {
		t.Fatalf("scribe global symlink should remain: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(project, projectfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pf.Add, []string{"tdd"}) {
		t.Fatalf("project add = %v, want [tdd]", pf.Add)
	}
	for _, link := range plan.RemovedLinks {
		if link.Skill == "scribe" {
			t.Fatalf("scribe should not be scheduled for removal: %#v", plan.RemovedLinks)
		}
	}
}

func TestApplyDryRunDoesNotMutateFilesystem(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	store := filepath.Join(tmp, "home", ".scribe", "skills")
	link := filepath.Join(tmp, "home", ".codex", "skills", "review")
	mustMkdir(t, filepath.Join(store, "review"))
	mustMkdir(t, filepath.Dir(link))
	mustMkdir(t, project)
	mustSymlink(t, filepath.Join(store, "review"), link)

	discovery := Discovery{
		GlobalSymlinks: []GlobalSymlink{{
			Tool:          "codex",
			Skill:         "review",
			Path:          link,
			CanonicalPath: filepath.Join(store, "review"),
		}},
		Projects: []ProjectCandidate{{Path: project, Source: "search_root"}},
		Skills:   []string{"review"},
	}

	plan, err := BuildPlan(discovery, []string{project}, true)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	result, err := Apply(plan, discovery.Projects)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.PlannedProjectFileWrites != 1 || result.PlannedGlobalLinkRemovals != 1 {
		t.Fatalf("dry-run result = %#v, want planned write and removal", result)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should remain in dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, projectfile.Filename)); !os.IsNotExist(err) {
		t.Fatalf(".scribe.yaml should not exist after dry-run, stat err = %v", err)
	}
}
