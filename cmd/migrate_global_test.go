package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/budget"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	clischema "github.com/Naoray/scribe/internal/cli/schema"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/projectmigrate"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type fakeProjectSelector struct {
	selected []string
}

func (f fakeProjectSelector) SelectProjects(_ []projectmigrate.ProjectCandidate) ([]string, error) {
	return f.selected, nil
}

func TestGlobalToProjectsJSONDryRunDoesNotMutateHome(t *testing.T) {
	home, project, link := setupGlobalToProjectsFixture(t, "claude", "tdd")
	t.Setenv("HOME", home)
	t.Chdir(project)

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "migrate", "global-to-projects", "--dry-run", "--project", project})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env struct {
		Status string `json:"status"`
		Data   struct {
			DryRun                    bool `json:"dry_run"`
			PlannedProjectFileWrites  int  `json:"planned_project_file_writes"`
			PlannedGlobalLinkRemovals int  `json:"planned_global_link_removals"`
			WroteProjectFiles         int  `json:"wrote_project_files"`
			RemovedGlobalLinks        int  `json:"removed_global_links"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if env.Status != "no_change" || !env.Data.DryRun {
		t.Fatalf("env = %#v, want no_change dry-run", env)
	}
	if env.Data.PlannedProjectFileWrites != 1 || env.Data.PlannedGlobalLinkRemovals != 1 {
		t.Fatalf("planned counts = %#v, want one write/removal", env.Data)
	}
	if env.Data.WroteProjectFiles != 0 || env.Data.RemovedGlobalLinks != 0 {
		t.Fatalf("applied counts = %#v, want dry-run no mutation", env.Data)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should remain after dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, projectfile.Filename)); !os.IsNotExist(err) {
		t.Fatalf(".scribe.yaml should not exist after dry-run, stat err = %v", err)
	}
}

func TestGlobalToProjectsSchema_MatchesEnvelope(t *testing.T) {
	home, project, _ := setupGlobalToProjectsFixture(t, "claude", "tdd")
	t.Setenv("HOME", home)
	t.Chdir(project)
	rawSchema, ok := clischema.Get("scribe migrate global-to-projects")
	if !ok {
		t.Fatal("missing migrate global-to-projects output schema")
	}
	compiled, err := jsonschema.CompileString("migrate-global-to-projects.schema.json", rawSchema)
	if err != nil {
		t.Fatalf("compile schema: %v\n%s", err, rawSchema)
	}
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "migrate", "global-to-projects", "--dry-run", "--project", project})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
	}
	var data any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v\n%s", err, string(env.Data))
	}
	if err := compiled.Validate(data); err != nil {
		t.Fatalf("schema validation: %v\ndata=%s\nschema=%s", err, string(env.Data), rawSchema)
	}
}

func TestGlobalToProjects_RefusesWithoutProject(t *testing.T) {
	home, project, link := setupGlobalToProjectsFixture(t, "claude", "tdd")
	t.Setenv("HOME", home)
	t.Chdir(project)

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate", "global-to-projects"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want refusal\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitUsage {
		t.Fatalf("exit code = %d, want %d; err=%v", got, clierrors.ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "must pass --project <path>; refusing to remove global symlinks") {
		t.Fatalf("error = %q, want project refusal", err.Error())
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should remain after refusal: %v", err)
	}
}

func TestGlobalToProjects_DryRunRefusesWithoutProject(t *testing.T) {
	home, project, link := setupGlobalToProjectsFixture(t, "claude", "tdd")
	t.Setenv("HOME", home)
	t.Chdir(project)

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "migrate", "global-to-projects", "--dry-run"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want refusal\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitUsage {
		t.Fatalf("exit code = %d, want %d; err=%v", got, clierrors.ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "must pass --project <path>; refusing to remove global symlinks") {
		t.Fatalf("error = %q, want project refusal", err.Error())
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should remain after refusal: %v", err)
	}
}

func TestGlobalToProjects_NoLinksSucceedsSilent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Chdir(project)

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "migrate", "global-to-projects"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var env struct {
		Status string `json:"status"`
		Data   struct {
			FoundGlobalLinks int `json:"found_global_links"`
			SelectedProjects int `json:"selected_projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if env.Status != "no_change" || env.Data.FoundGlobalLinks != 0 || env.Data.SelectedProjects != 0 {
		t.Fatalf("env = %#v, want silent no_change", env)
	}
}

func TestGlobalToProjectsInteractiveSelectorAppliesMigration(t *testing.T) {
	home, project, link := setupGlobalToProjectsFixture(t, "codex", "review")
	t.Setenv("HOME", home)
	t.Chdir(project)

	oldTerminal := globalToProjectsIsTerminal
	globalToProjectsIsTerminal = func() bool { return true }
	t.Cleanup(func() { globalToProjectsIsTerminal = oldTerminal })

	cmd := newGlobalToProjectsCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := runGlobalToProjectsWithSelector(cmd, nil, fakeProjectSelector{selected: []string{project}}); err != nil {
		t.Fatalf("runGlobalToProjectsWithSelector() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("global symlink still exists or unexpected stat error: %v", err)
	}
	pf, err := projectfile.Load(filepath.Join(project, projectfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pf.Add, []string{"review"}) {
		t.Fatalf("project add = %v, want [review]", pf.Add)
	}
}

func TestGlobalToProjects_UndoFlag(t *testing.T) {
	home, project, link := setupGlobalToProjectsFixture(t, "claude", "tdd")
	t.Setenv("HOME", home)
	t.Chdir(project)

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate", "global-to-projects", "--project", project})
	if err := root.Execute(); err != nil {
		t.Fatalf("migrate Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("global symlink should be removed after migrate, stat err = %v", err)
	}

	root = newRootCmd()
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "migrate", "global-to-projects", "--undo"})
	if err := root.Execute(); err != nil {
		t.Fatalf("undo Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var env struct {
		Status string `json:"status"`
		Data   struct {
			RestoredLinks int    `json:"restored_links"`
			Snapshot      string `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if env.Status != "ok" || env.Data.RestoredLinks != 1 || env.Data.Snapshot == "" {
		t.Fatalf("env = %#v, want undo result", env)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("global symlink should be restored after undo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, projectfile.Filename)); !os.IsNotExist(err) {
		t.Fatalf(".scribe.yaml should be deleted after undo, stat err = %v", err)
	}
}

func TestPrintGlobalToProjectsResult_DryRunShowsPaths(t *testing.T) {
	cmd := newGlobalToProjectsCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	printGlobalToProjectsResult(cmd, projectmigrate.MigrationResult{
		DryRun:                    true,
		FoundGlobalLinks:          6,
		FoundSkills:               2,
		PlannedProjectFileWrites:  1,
		PlannedGlobalLinkRemovals: 6,
		ProjectFiles: []projectmigrate.ProjectChange{{
			Project:     "/tmp/project",
			File:        "/tmp/project/.scribe.yaml",
			Skills:      []string{"review", "tdd"},
			AddedSkills: []string{"tdd"},
			Changed:     true,
			BudgetPerAgent: map[string]budget.Result{
				"claude": {Agent: "claude", Limit: 8192, Used: 6348, Status: budget.StatusWarn},
			},
		}},
		RemovedLinks: []projectmigrate.GlobalSymlink{
			{Path: "/tmp/home/.claude/skills/a", CanonicalPath: "/tmp/home/.scribe/skills/a"},
			{Path: "/tmp/home/.claude/skills/b", CanonicalPath: "/tmp/home/.scribe/skills/b"},
			{Path: "/tmp/home/.claude/skills/c", CanonicalPath: "/tmp/home/.scribe/skills/c"},
			{Path: "/tmp/home/.claude/skills/d", CanonicalPath: "/tmp/home/.scribe/skills/d"},
			{Path: "/tmp/home/.claude/skills/e", CanonicalPath: "/tmp/home/.scribe/skills/e"},
			{Path: "/tmp/home/.claude/skills/f", CanonicalPath: "/tmp/home/.scribe/skills/f"},
		},
	})
	out := stdout.String()
	for _, want := range []string{
		"  write /tmp/project/.scribe.yaml (2 skills, 1 added)",
		"  budget: claude PASS 6.2KB / 8KB",
		"    - /tmp/home/.claude/skills/a → /tmp/home/.scribe/skills/a",
		"    ... and 1 more",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}

func TestPrintGlobalToProjectsResult_OverBudget(t *testing.T) {
	cmd := newGlobalToProjectsCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	printGlobalToProjectsResult(cmd, projectmigrate.MigrationResult{
		DryRun:                    true,
		FoundGlobalLinks:          1,
		FoundSkills:               1,
		PlannedProjectFileWrites:  1,
		PlannedGlobalLinkRemovals: 1,
		ProjectFiles: []projectmigrate.ProjectChange{{
			File:        "/tmp/project/.scribe.yaml",
			Skills:      []string{"oversized"},
			AddedSkills: []string{"oversized"},
			Changed:     true,
			BudgetPerAgent: map[string]budget.Result{
				"claude": {Agent: "claude", Limit: 8192, Used: 35533, Status: budget.StatusRefuse},
			},
		}},
	})
	out := stdout.String()
	if !strings.Contains(out, "  budget: claude REFUSE 34.7KB / 8KB (+26.7KB)") {
		t.Fatalf("output = %s, want overbudget line", out)
	}
}

func setupGlobalToProjectsFixture(t *testing.T, tool, skill string) (home, project, link string) {
	t.Helper()
	tmp := t.TempDir()
	home = filepath.Join(tmp, "home")
	project = filepath.Join(tmp, "project")
	storeSkill := filepath.Join(home, ".scribe", "skills", skill)
	link = filepath.Join(home, "."+tool, "skills", skill)
	for _, dir := range []string{project, storeSkill, filepath.Dir(link)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(storeSkill, "SKILL.md"), []byte("---\nname: "+skill+"\ndescription: Test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(storeSkill, link); err != nil {
		t.Fatal(err)
	}
	return home, project, link
}
