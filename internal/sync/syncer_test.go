package sync_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type mockExecutor struct {
	commands []string
	stdout   string
	stderr   string
	err      error
}

func (m *mockExecutor) Execute(ctx context.Context, command string, timeout time.Duration) (string, string, error) {
	m.commands = append(m.commands, command)
	return m.stdout, m.stderr, m.err
}

type syncTestFetcher struct {
	files []tools.SkillFile
}

func (f *syncTestFetcher) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return nil, nil
}

func (f *syncTestFetcher) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]tools.SkillFile, error) {
	return f.files, nil
}

func (f *syncTestFetcher) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	return "", nil
}

func (f *syncTestFetcher) GetTree(ctx context.Context, owner, repo, ref string) ([]provider.TreeEntry, error) {
	return nil, nil
}

// mockProvider implements provider.Provider for tests.
type mockProvider struct {
	entries []manifest.Entry
	files   []tools.SkillFile
}

func (m *mockProvider) Discover(_ context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{Entries: m.entries, IsTeam: true}, nil
}

func (m *mockProvider) Fetch(_ context.Context, entry manifest.Entry) ([]provider.File, error) {
	out := make([]provider.File, len(m.files))
	for i, f := range m.files {
		out[i] = provider.File{Path: f.Path, Content: f.Content}
	}
	return out, nil
}

func TestRun_KitFilterLimitsProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()

	prov := &mockProvider{
		entries: []manifest.Entry{
			{Name: "recap", Source: "github:acme/skills@main"},
			{Name: "debugger", Source: "github:acme/skills@main"},
			{Name: "coder", Source: "github:acme/skills@main"},
		},
		files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# skill\n")}},
	}

	syncer := &sync.Syncer{
		Client:           &syncTestFetcher{},
		Provider:         prov,
		Tools:            []tools.Tool{tools.ClaudeTool{}},
		ProjectRoot:      projectRoot,
		KitFilter:        []string{"recap", "coder"},
		KitFilterEnabled: true,
		SkipMissing:      false,
	}
	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	if err := syncer.Run(context.Background(), "acme/skills", st); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, ok := st.Installed["recap"]; !ok {
		t.Error("recap should be installed (in kit)")
	}
	if _, ok := st.Installed["coder"]; !ok {
		t.Error("coder should be installed (in kit)")
	}
	if _, ok := st.Installed["debugger"]; ok {
		t.Error("debugger should NOT be installed (not in kit)")
	}

	recapLink := filepath.Join(projectRoot, ".claude", "skills", "recap")
	coderLink := filepath.Join(projectRoot, ".claude", "skills", "coder")
	debuggerLink := filepath.Join(projectRoot, ".claude", "skills", "debugger")

	if _, err := os.Lstat(recapLink); err != nil {
		t.Errorf("recap symlink missing: %v", err)
	}
	if _, err := os.Lstat(coderLink); err != nil {
		t.Errorf("coder symlink missing: %v", err)
	}
	if _, err := os.Lstat(debuggerLink); err == nil {
		t.Error("debugger symlink should not exist")
	}
}

func TestRunWithDiff_DoesNotOverwriteTargetReadme(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	projectDir := t.TempDir()
	readmePath := filepath.Join(projectDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# project readme\n"), 0o644); err != nil {
		t.Fatalf("write project readme: %v", err)
	}

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# skill\n")},
				{Path: "README.md", Content: []byte("# scribe readme\n")},
			},
		},
		Tools: []tools.Tool{tools.CommandTool{
			ToolName:         "project-copy",
			InstallCommand:   "cp -R \"{{canonical_dir}}\"/. \"" + projectDir + "\"",
			UninstallCommand: "true",
			PathTemplate:     projectDir,
		}},
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "repo-root-skill",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:   "repo-root-skill",
			Source: "github:acme/skills@main",
		},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/team", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	got, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read project readme: %v", err)
	}
	if string(got) != "# project readme\n" {
		t.Fatalf("README.md overwritten: got %q", string(got))
	}
}

func TestRunWithDiff_SkipsRemovedByUser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}},
		},
		Emit: func(msg any) { events = append(events, msg) },
	}
	st := &state.State{
		Installed:     map[string]state.InstalledSkill{},
		RemovedByUser: []state.RemovedSkill{{Name: "recap", Registry: "acme/skills", RemovedAt: time.Now()}},
	}
	statuses := []sync.SkillStatus{{
		Name:   "recap",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}
	if _, ok := st.Installed["recap"]; ok {
		t.Fatal("recap should not be installed when deny-listed")
	}

	found := false
	for _, ev := range events {
		if msg, ok := ev.(sync.SkillSkippedByDenyListMsg); ok {
			found = true
			if msg.Name != "recap" || msg.Registry != "acme/skills" {
				t.Fatalf("deny-list skip msg = %+v", msg)
			}
		}
	}
	if !found {
		t.Fatal("expected SkillSkippedByDenyListMsg")
	}
}

func TestRunWithDiff_RemovedByUserIsRegistryScoped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}},
		},
	}
	st := &state.State{
		Installed:     map[string]state.InstalledSkill{},
		RemovedByUser: []state.RemovedSkill{{Name: "recap", Registry: "acme/skills", RemovedAt: time.Now()}},
	}
	statuses := []sync.SkillStatus{{
		Name:   "recap",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "recap", Source: "github:example/registry@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff acme: %v", err)
	}
	if _, ok := st.Installed["recap"]; ok {
		t.Fatal("recap should not install from deny-listed acme/skills")
	}

	if err := syncer.RunWithDiff(context.Background(), "other/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff other: %v", err)
	}
	installed, ok := st.Installed["recap"]
	if !ok {
		t.Fatal("recap should install from other/skills")
	}
	if len(installed.Sources) != 1 || installed.Sources[0].Registry != "other/skills" {
		t.Fatalf("installed sources = %+v, want other/skills", installed.Sources)
	}
}

func TestRunWithDiff_RecordsProjectProjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}},
		},
		Tools:       []tools.Tool{tools.ClaudeTool{}},
		ProjectRoot: projectRoot,
	}
	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "recap",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	installed := st.Installed["recap"]
	if len(installed.Projections) != 1 {
		t.Fatalf("Projections = %#v, want one entry", installed.Projections)
	}
	if installed.Projections[0].Project != projectRoot {
		t.Fatalf("Projection project = %q, want %q", installed.Projections[0].Project, projectRoot)
	}
	if got := installed.Projections[0].Tools; len(got) != 1 || got[0] != "claude" {
		t.Fatalf("Projection tools = %v, want [claude]", got)
	}

	wantPath := filepath.Join(projectRoot, ".claude", "skills", "recap")
	if len(installed.ManagedPaths) != 1 || installed.ManagedPaths[0] != wantPath {
		t.Fatalf("ManagedPaths = %v, want [%s]", installed.ManagedPaths, wantPath)
	}
}

func TestRunWithDiff_RecordsGlobalProjectionWhenProjectRootEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}},
		},
		Tools: []tools.Tool{tools.ClaudeTool{}},
	}
	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "recap",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	installed := st.Installed["recap"]
	if len(installed.Projections) != 1 || installed.Projections[0].Project != "" {
		t.Fatalf("Projections = %#v, want one global entry", installed.Projections)
	}
	wantPath := filepath.Join(home, ".claude", "skills", "recap")
	if len(installed.ManagedPaths) != 1 || installed.ManagedPaths[0] != wantPath {
		t.Fatalf("ManagedPaths = %v, want [%s]", installed.ManagedPaths, wantPath)
	}
}

func TestSync_PromotesGlobalProjectionWhenProjectFileExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	writeStoredSkill(t, storeDir, "recap", "project recap")

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, ".scribe.yaml"), []byte("kits:\n  - core\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	syncer := &sync.Syncer{
		Tools:       []tools.Tool{tools.ClaudeTool{}},
		ProjectRoot: projectRoot,
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"recap": {
			InstalledHash: sync.ComputeFileHash(skillContent("recap", "project recap")),
			Tools:         []string{"claude"},
			Projections: []state.ProjectionEntry{{
				Project: "",
				Tools:   []string{"claude"},
			}},
		},
	}}
	current := st.Installed["recap"]
	statuses := []sync.SkillStatus{{
		Name:      "recap",
		Status:    sync.StatusCurrent,
		Installed: &current,
		Entry:     &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	installed := st.Installed["recap"]
	foundGlobal := false
	foundProject := false
	for _, projection := range installed.Projections {
		if projection.Project == "" && reflect.DeepEqual(projection.Tools, []string{"claude"}) {
			foundGlobal = true
		}
		if projection.Project == projectRoot && reflect.DeepEqual(projection.Tools, []string{"claude"}) {
			foundProject = true
		}
	}
	if !foundGlobal {
		t.Fatalf("Projections = %#v, want retained global projection", installed.Projections)
	}
	if !foundProject {
		t.Fatalf("Projections = %#v, want project projection", installed.Projections)
	}
	if _, err := os.Lstat(filepath.Join(projectRoot, ".claude", "skills", "recap")); err != nil {
		t.Fatalf("project symlink missing: %v", err)
	}
}

func TestSync_PromotesGlobalProjectionFromCurrentProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	writeStoredSkill(t, storeDir, "recap", "project recap")

	projectRoot := t.TempDir()
	t.Chdir(projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, ".scribe.yaml"), []byte("add:\n  - recap\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	syncer := &sync.Syncer{
		Tools: []tools.Tool{tools.ClaudeTool{}},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"recap": {
			InstalledHash: sync.ComputeFileHash(skillContent("recap", "project recap")),
			Tools:         []string{"claude"},
			Projections: []state.ProjectionEntry{{
				Project: "",
				Tools:   []string{"claude"},
			}},
		},
	}}
	current := st.Installed["recap"]
	statuses := []sync.SkillStatus{{
		Name:      "recap",
		Status:    sync.StatusCurrent,
		Installed: &current,
		Entry:     &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	installed := st.Installed["recap"]
	foundProject := false
	for _, projection := range installed.Projections {
		if projection.Project == projectRoot && reflect.DeepEqual(projection.Tools, []string{"claude"}) {
			foundProject = true
		}
	}
	if !foundProject {
		t.Fatalf("Projections = %#v, want project projection from cwd .scribe.yaml", installed.Projections)
	}
	if _, err := os.Lstat(filepath.Join(projectRoot, ".claude", "skills", "recap")); err != nil {
		t.Fatalf("project symlink missing: %v", err)
	}
}

func TestRunWithDiff_MultiProjectProjectionPathsAreIsolated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectOne := t.TempDir()
	projectTwo := t.TempDir()
	files := []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# recap\n")}}
	statuses := []sync.SkillStatus{{
		Name:   "recap",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "recap", Source: "github:acme/skills@main"},
	}}
	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	for _, projectRoot := range []string{projectOne, projectTwo} {
		syncer := &sync.Syncer{
			Client:      &syncTestFetcher{files: files},
			Tools:       []tools.Tool{tools.ClaudeTool{}},
			ProjectRoot: projectRoot,
		}
		if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
			t.Fatalf("RunWithDiff %s: %v", projectRoot, err)
		}
	}

	installed := st.Installed["recap"]
	if len(installed.Projections) != 2 {
		t.Fatalf("Projections = %#v, want two project entries", installed.Projections)
	}
	pathOne := filepath.Join(projectOne, ".claude", "skills", "recap")
	pathTwo := filepath.Join(projectTwo, ".claude", "skills", "recap")
	if _, err := os.Lstat(pathOne); err != nil {
		t.Fatalf("project one projection missing: %v", err)
	}
	if _, err := os.Lstat(pathTwo); err != nil {
		t.Fatalf("project two projection missing: %v", err)
	}

	if err := os.Remove(pathOne); err != nil {
		t.Fatalf("remove project one projection: %v", err)
	}
	if _, err := os.Lstat(pathTwo); err != nil {
		t.Fatalf("project two projection affected by project one removal: %v", err)
	}
}

func TestApply_PackageMissing_Approved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{}
	var events []any

	syncer := &sync.Syncer{
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
		TrustAll: true,
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	statuses := []sync.SkillStatus{{
		Name:   "superpowers",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:    "superpowers",
			Type:    "package",
			Source:  "github:obra/superpowers@main",
			Install: "claude plugin install superpowers",
			Update:  "claude plugin update superpowers",
		},
		IsPackage: true,
	}}

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executor.commands) != 1 {
		t.Fatalf("expected 1 command executed, got %d", len(executor.commands))
	}
	if executor.commands[0] != "claude plugin install superpowers" {
		t.Errorf("wrong command: %q", executor.commands[0])
	}

	// Bare name in state (flat storage model).
	installed, ok := st.Installed["superpowers"]
	if !ok {
		t.Fatal("superpowers not in state after install")
	}
	if installed.Type != "package" {
		t.Errorf("type: got %q, want %q", installed.Type, "package")
	}
	// Verify source tracking.
	if len(installed.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(installed.Sources))
	}
	if installed.Sources[0].Registry != "test/repo" {
		t.Errorf("source registry: got %q, want %q", installed.Sources[0].Registry, "test/repo")
	}
}

func TestApply_PackageMissing_NeedsApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var events []any

	syncer := &sync.Syncer{
		Executor: &mockExecutor{},
		Emit:     func(msg any) { events = append(events, msg) },
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	statuses := []sync.SkillStatus{{
		Name:   "superpowers",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:    "superpowers",
			Type:    "package",
			Source:  "github:obra/superpowers@main",
			Install: "claude plugin install superpowers",
		},
		IsPackage: true,
	}}

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, ev := range events {
		if skip, ok := ev.(sync.PackageSkippedMsg); ok {
			found = true
			if skip.Reason != "approval_required" {
				t.Errorf("reason: got %q, want %q", skip.Reason, "approval_required")
			}
		}
	}
	if !found {
		t.Error("expected PackageSkippedMsg event")
	}
}

func TestApply_PackageInstall_Error(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{
		stderr: "command not found",
		err:    fmt.Errorf("exit status 1"),
	}
	var events []any

	syncer := &sync.Syncer{
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
		TrustAll: true,
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	statuses := []sync.SkillStatus{{
		Name:   "broken-pkg",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:    "broken-pkg",
			Type:    "package",
			Source:  "github:example/broken@main",
			Install: "broken-command",
		},
		IsPackage: true,
	}}

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, ev := range events {
		if pe, ok := ev.(sync.PackageErrorMsg); ok {
			found = true
			if pe.Stderr != "command not found" {
				t.Errorf("stderr: got %q", pe.Stderr)
			}
		}
	}
	if !found {
		t.Error("expected PackageErrorMsg event")
	}

	if _, ok := st.Installed["broken-pkg"]; ok {
		t.Error("broken package should not be in state")
	}
}

func TestApply_PackageOutdated_WithUpdateCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{}
	var events []any

	syncer := &sync.Syncer{
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
		TrustAll: true,
	}

	installCmd := "claude plugin install superpowers"
	updateCmd := "claude plugin update superpowers"
	hash := sync.CommandHash(installCmd, updateCmd, nil, nil)

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"superpowers": {
			Type: "package",
			Sources: []state.SkillSource{{
				Registry: "test/repo",
				Ref:      "main",
				LastSHA:  "oldsha",
			}},
			InstallCmd: installCmd,
			UpdateCmd:  updateCmd,
			CmdHash:    hash,
			Approval:   "approved",
		},
	}}

	statuses := []sync.SkillStatus{{
		Name:   "superpowers",
		Status: sync.StatusOutdated,
		Entry: &manifest.Entry{
			Name:    "superpowers",
			Type:    "package",
			Source:  "github:obra/superpowers@main",
			Install: installCmd,
			Update:  updateCmd,
		},
		IsPackage: true,
		LatestSHA: "newsha",
	}}

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executor.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(executor.commands))
	}
	if executor.commands[0] != updateCmd {
		t.Errorf("expected update command, got %q", executor.commands[0])
	}
}

func TestApply_PackageOutdated_NoUpdateCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{}
	var events []any

	syncer := &sync.Syncer{
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
		TrustAll: true,
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"minimal-pkg": {
			Type: "package",
			Sources: []state.SkillSource{{
				Registry: "test/repo",
				Ref:      "main",
				LastSHA:  "oldsha",
			}},
			Approval: "approved",
		},
	}}

	statuses := []sync.SkillStatus{{
		Name:   "minimal-pkg",
		Status: sync.StatusOutdated,
		Entry: &manifest.Entry{
			Name:    "minimal-pkg",
			Type:    "package",
			Source:  "github:example/minimal@main",
			Install: "install-it",
		},
		IsPackage: true,
	}}

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, ev := range events {
		if skip, ok := ev.(sync.PackageSkippedMsg); ok {
			found = true
			if skip.Reason != "no update command" {
				t.Errorf("reason: got %q", skip.Reason)
			}
		}
	}
	if !found {
		t.Error("expected PackageSkippedMsg for missing update command")
	}

	if len(executor.commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(executor.commands))
	}
}

func TestRunWithDiff_BudgetUsesCurrentProjectProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	writeStoredSkill(t, storeDir, "unrelated", strings.Repeat("x", 6000))

	projectRoot := t.TempDir()
	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: skillContent("incoming", "small")}},
		},
		Tools:       []tools.Tool{tools.CodexTool{}},
		ProjectRoot: projectRoot,
		Emit:        func(msg any) { events = append(events, msg) },
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"unrelated": {
			Tools: []string{"codex"},
			Projections: []state.ProjectionEntry{{
				Project: t.TempDir(),
				Tools:   []string{"codex"},
			}},
		},
	}}
	statuses := []sync.SkillStatus{{
		Name:   "incoming",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "incoming", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}
	for _, event := range events {
		if msg, ok := event.(sync.SkillErrorMsg); ok {
			t.Fatalf("unexpected SkillErrorMsg: %v", msg.Err)
		}
	}
	if _, ok := st.Installed["incoming"]; !ok {
		t.Fatal("incoming skill was not installed")
	}
}

func TestRunWithDiff_EmitsBudgetWarningForPostChangeProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	projectRoot := t.TempDir()
	writeStoredSkill(t, storeDir, "existing", strings.Repeat("x", 3600))

	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: skillContent("incoming", strings.Repeat("y", 220))}},
		},
		Tools:       []tools.Tool{tools.CodexTool{}},
		ProjectRoot: projectRoot,
		Emit:        func(msg any) { events = append(events, msg) },
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"existing": {
			Tools: []string{"codex"},
			Projections: []state.ProjectionEntry{{
				Project: projectRoot,
				Tools:   []string{"codex"},
			}},
		},
	}}
	statuses := []sync.SkillStatus{{
		Name:   "incoming",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "incoming", Source: "github:acme/skills@main"},
	}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	for _, event := range events {
		if msg, ok := event.(sync.BudgetWarningMsg); ok {
			if msg.Agent != "codex" {
				t.Fatalf("Agent = %q, want codex", msg.Agent)
			}
			if !strings.Contains(msg.Message, "Codex budget") {
				t.Fatalf("Message = %q, want Codex budget warning", msg.Message)
			}
			return
		}
	}
	t.Fatal("expected BudgetWarningMsg")
}

func TestRunWithDiff_SkipsBudgetOnMigrationSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"codex": 20}
	t.Cleanup(func() { budget.AgentBudgets = old })
	projectRoot := t.TempDir()
	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{{Path: "SKILL.md", Content: skillContent("incoming", strings.Repeat("y", 200))}},
		},
		Tools:       []tools.Tool{tools.CodexTool{}},
		ProjectRoot: projectRoot,
		Emit:        func(msg any) { events = append(events, msg) },
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"incoming": {
			Projections: []state.ProjectionEntry{{
				Project: projectRoot,
				Tools:   []string{"codex"},
				Source:  state.SourceMigration,
			}},
		},
	}}
	statuses := []sync.SkillStatus{{
		Name:   "incoming",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "incoming", Source: "github:acme/skills@main"},
	}}
	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}
	for _, event := range events {
		if msg, ok := event.(sync.SkillErrorMsg); ok {
			t.Fatalf("unexpected SkillErrorMsg: %v", msg.Err)
		}
	}
}

// TestApply_RealDirectoryAtProjectionPath verifies that sync emits an actionable
// SkillErrorMsg and preserves the real directory when a non-scribe directory
// exists at the tool projection path.
func TestApply_RealDirectoryAtProjectionPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a real (non-symlink) directory at ~/.claude/skills/qa to simulate
	// a skill installed by another tool or manually.
	realSkillPath := filepath.Join(home, ".claude", "skills", "qa")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realSkillPath, "SKILL.md"), []byte("# manual qa skill\n"), 0o644); err != nil {
		t.Fatalf("write existing SKILL.md: %v", err)
	}

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# qa from registry\n")},
			},
		},
		Tools: []tools.Tool{tools.ClaudeTool{}},
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:   "qa",
			Source: "github:acme/skills@main",
		},
	}}

	err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var conflict *sync.NameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want NameConflictError", err, err)
	}
	if !strings.Contains(err.Error(), "real directory") {
		t.Errorf("error should mention 'real directory', got: %v", err)
	}

	// Real directory must be preserved.
	if _, err := os.Stat(filepath.Join(realSkillPath, "SKILL.md")); err != nil {
		t.Errorf("real directory was destroyed: %v", err)
	}

	// Skill must not be recorded as installed in state.
	if _, ok := st.Installed["qa"]; ok {
		t.Error("skill should not be in state when install failed")
	}
}

func TestRunWithDiff_NameConflictWithoutResolverReturnsConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realSkillPath := filepath.Join(home, ".test-tool", "qa")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# qa\n")}}},
		Tools:  []tools.Tool{testProjectionTool{root: filepath.Join(home, ".test-tool")}},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	err := syncer.RunWithDiff(context.Background(), "acme/skills", []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "qa", Source: "github:acme/skills@main"},
	}}, st)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var conflict *sync.NameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want NameConflictError", err, err)
	}
	if conflict.Resolution.Action != sync.NameConflictActionUnresolved {
		t.Fatalf("resolution action = %q, want unresolved", conflict.Resolution.Action)
	}
	if _, ok := st.Installed["qa"]; ok {
		t.Fatal("qa should not be installed on unresolved conflict")
	}
}

func TestRunWithDiff_NameConflictAliasInstallsIncomingUnderAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realSkillPath := filepath.Join(home, ".test-tool", "qa")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}

	var events []any
	syncer := &sync.Syncer{
		Client:    &syncTestFetcher{files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# qa\n")}}},
		Tools:     []tools.Tool{testProjectionTool{root: filepath.Join(home, ".test-tool")}},
		AliasName: "qa-registry",
		Emit:      func(msg any) { events = append(events, msg) },
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "qa", Source: "github:acme/skills@main"},
	}}, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	if _, ok := st.Installed["qa"]; ok {
		t.Fatal("original name should not be installed")
	}
	if _, ok := st.Installed["qa-registry"]; !ok {
		t.Fatal("alias should be installed")
	}
	if _, err := os.Lstat(filepath.Join(home, ".test-tool", "qa-registry")); err != nil {
		t.Fatalf("alias projection missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(realSkillPath)); err != nil {
		t.Fatalf("original real directory was touched: %v", err)
	}
	assertConflictResolutionEvent(t, events, sync.NameConflictActionAlias, "qa-registry")
}

func TestRunWithDiff_NameConflictAliasCollisionReturnsConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".test-tool", "qa"), 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}

	syncer := &sync.Syncer{
		Client:    &syncTestFetcher{files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# qa\n")}}},
		Tools:     []tools.Tool{testProjectionTool{root: filepath.Join(home, ".test-tool")}},
		AliasName: "existing",
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"existing": {Revision: 1},
	}}

	err := syncer.RunWithDiff(context.Background(), "acme/skills", []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "qa", Source: "github:acme/skills@main"},
	}}, st)
	if err == nil {
		t.Fatal("expected alias collision conflict")
	}
	var conflict *sync.NameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want NameConflictError", err, err)
	}
	if conflict.Resolution.Action != sync.NameConflictActionAlias || conflict.Resolution.Alias != "existing" {
		t.Fatalf("resolution = %+v, want alias existing", conflict.Resolution)
	}
}

func TestRunWithDiff_NameConflictResolverSkipLeavesExistingUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realSkillPath := filepath.Join(home, ".test-tool", "qa")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}
	marker := filepath.Join(realSkillPath, "SKILL.md")
	if err := os.WriteFile(marker, []byte("# manual\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# qa\n")}}},
		Tools:  []tools.Tool{testProjectionTool{root: filepath.Join(home, ".test-tool")}},
		NameConflictResolver: func(sync.NameConflict) (sync.NameConflictResolution, error) {
			return sync.NameConflictResolution{Action: sync.NameConflictActionSkip}, nil
		},
		Emit: func(msg any) { events = append(events, msg) },
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "qa", Source: "github:acme/skills@main"},
	}}, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "# manual\n" {
		t.Fatalf("existing marker = %q, %v; want untouched", got, err)
	}
	if len(st.Installed) != 0 {
		t.Fatalf("state installed = %v, want empty", st.Installed)
	}
	assertConflictResolutionEvent(t, events, sync.NameConflictActionSkip, "")
}

func TestRunWithDiff_NameConflictResolverAdoptReturnsGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".test-tool", "qa"), 0o755); err != nil {
		t.Fatalf("mkdir real skill dir: %v", err)
	}

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{files: []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# qa\n")}}},
		Tools:  []tools.Tool{testProjectionTool{root: filepath.Join(home, ".test-tool")}},
		NameConflictResolver: func(sync.NameConflict) (sync.NameConflictResolution, error) {
			return sync.NameConflictResolution{Action: sync.NameConflictActionAdopt}, nil
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	err := syncer.RunWithDiff(context.Background(), "acme/skills", []sync.SkillStatus{{
		Name:   "qa",
		Status: sync.StatusMissing,
		Entry:  &manifest.Entry{Name: "qa", Source: "github:acme/skills@main"},
	}}, st)
	if err == nil {
		t.Fatal("expected adopt guidance conflict")
	}
	if !strings.Contains(err.Error(), "scribe adopt qa") {
		t.Fatalf("error = %v, want adopt guidance", err)
	}
	var conflict *sync.NameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want NameConflictError", err, err)
	}
	if conflict.Resolution.Action != sync.NameConflictActionAdopt {
		t.Fatalf("resolution action = %q, want adopt", conflict.Resolution.Action)
	}
}

type testProjectionTool struct {
	root string
}

func (t testProjectionTool) Name() string { return "test-tool" }

func (t testProjectionTool) Install(skillName, canonicalDir, _ string) ([]string, error) {
	path, err := t.SkillPath(skillName, "")
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("%w: %s", tools.ErrRealDirectoryExists, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Symlink(canonicalDir, path); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func (t testProjectionTool) Uninstall(skillName string) error {
	path, err := t.SkillPath(skillName, "")
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (t testProjectionTool) Detect() bool { return true }

func (t testProjectionTool) SkillPath(skillName, projectRoot string) (string, error) {
	return filepath.Join(t.root, skillName), nil
}

func (t testProjectionTool) CanonicalTarget(canonicalDir string) (string, bool) {
	return canonicalDir, true
}

func assertConflictResolutionEvent(t *testing.T, events []any, action sync.NameConflictAction, alias string) {
	t.Helper()
	for _, ev := range events {
		msg, ok := ev.(sync.NameConflictResolvedMsg)
		if !ok {
			continue
		}
		if msg.Resolution.Action != action || msg.Resolution.Alias != alias {
			t.Fatalf("resolution event = %+v, want action=%q alias=%q", msg.Resolution, action, alias)
		}
		return
	}
	t.Fatal("expected NameConflictResolvedMsg")
}

func writeStoredSkill(t *testing.T, storeDir, name, description string) {
	t.Helper()
	skillDir := filepath.Join(storeDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir stored skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContent(name, description), 0o644); err != nil {
		t.Fatalf("write stored skill: %v", err)
	}
}

func skillContent(name, description string) []byte {
	return []byte("---\n" +
		"name: " + name + "\n" +
		"description: " + description + "\n" +
		"---\n")
}
