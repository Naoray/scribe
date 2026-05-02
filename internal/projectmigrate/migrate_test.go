package projectmigrate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
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

func TestBuildPlan_FailsBudget_NoForce(t *testing.T) {
	home, project, link := setupBudgetMigrationFixture(t, "claude", "oversized", 200)
	t.Setenv("HOME", home)
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"claude": 20}
	t.Cleanup(func() { budget.AgentBudgets = old })
	discovery := undoDiscovery(home, project, link, "claude", "oversized")
	_, err := BuildPlan(discovery, []string{project}, false)
	if err == nil {
		t.Fatal("BuildPlan() error = nil, want budget refusal")
	}
	if !strings.Contains(err.Error(), "project "+project+" exceeds claude budget") || !strings.Contains(err.Error(), "pass --force to proceed") {
		t.Fatalf("error = %q, want budget refusal", err.Error())
	}
}

func TestBuildPlan_FailsBudget_PassesWithForce(t *testing.T) {
	home, project, link := setupBudgetMigrationFixture(t, "claude", "oversized", 200)
	t.Setenv("HOME", home)
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"claude": 20}
	t.Cleanup(func() { budget.AgentBudgets = old })
	discovery := undoDiscovery(home, project, link, "claude", "oversized")
	plan, err := BuildPlan(discovery, []string{project}, false, true)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if got := plan.ProjectFiles[0].BudgetPerAgent["claude"].Status; got != budget.StatusRefuse {
		t.Fatalf("budget status = %s, want refuse", got)
	}
}

func TestApply_SetsMigrationSource(t *testing.T) {
	home, project, link := setupBudgetMigrationFixture(t, "claude", "tdd", 10)
	t.Setenv("HOME", home)
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"tdd": {Projections: []state.ProjectionEntry{{Project: "", Tools: []string{"claude"}}}},
		},
		Kits:               map[string]state.InstalledKit{},
		Snippets:           map[string]state.InstalledSnippet{},
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]state.RegistryFailure{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	if err := st.Save(); err != nil {
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
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, projection := range loaded.Installed["tdd"].Projections {
		if projection.Project == project && projection.Source == state.SourceMigration {
			found = true
		}
	}
	if !found {
		t.Fatalf("projections = %#v, want migration source for project", loaded.Installed["tdd"].Projections)
	}
}

func TestApply_ClearsLegacyGlobalProjections(t *testing.T) {
	home, project, link := setupBudgetMigrationFixture(t, "claude", "tdd", 10)
	t.Setenv("HOME", home)
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"tdd": {Projections: []state.ProjectionEntry{{Project: "", Tools: []string{"claude"}}}},
		},
		Kits:               map[string]state.InstalledKit{},
		Snippets:           map[string]state.InstalledSnippet{},
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]state.RegistryFailure{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	if err := st.Save(); err != nil {
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
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range loaded.Installed["tdd"].Projections {
		if projection.Project == "" {
			t.Fatalf("legacy projection still present: %#v", loaded.Installed["tdd"].Projections)
		}
	}
}

func TestApply_RecordsProjectScopedProjections(t *testing.T) {
	home, project, link := setupBudgetMigrationFixture(t, "codex", "review", 10)
	t.Setenv("HOME", home)
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"review": {Projections: []state.ProjectionEntry{{Project: "", Tools: []string{"codex"}}}},
		},
		Kits:               map[string]state.InstalledKit{},
		Snippets:           map[string]state.InstalledSnippet{},
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]state.RegistryFailure{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	discovery := undoDiscovery(home, project, link, "codex", "review")
	plan, err := BuildPlan(discovery, []string{project}, false)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := Apply(plan, discovery.Projects); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Installed["review"].Projections, []state.ProjectionEntry{{Project: project, Tools: []string{"codex"}, Source: state.SourceMigration}}) {
		t.Fatalf("projections = %#v, want project migration projection", loaded.Installed["review"].Projections)
	}
}

func setupBudgetMigrationFixture(t *testing.T, tool, skill string, descriptionBytes int) (home, project, link string) {
	t.Helper()
	home = t.TempDir()
	project = filepath.Join(home, "project")
	storeSkill := filepath.Join(home, ".scribe", "skills", skill)
	link = filepath.Join(home, "."+tool, "skills", skill)
	for _, dir := range []string{project, storeSkill, filepath.Dir(link)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("---\nname: " + skill + "\ndescription: " + strings.Repeat("x", descriptionBytes) + "\n---\n")
	if err := os.WriteFile(filepath.Join(storeSkill, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(storeSkill, link); err != nil {
		t.Fatal(err)
	}
	return home, project, link
}
