package sync_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	var events []any
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# qa from registry\n")},
			},
		},
		Tools: []tools.Tool{tools.ClaudeTool{}},
		Emit:  func(msg any) { events = append(events, msg) },
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

	if err := syncer.RunWithDiff(context.Background(), "acme/skills", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	// Expect a SkillErrorMsg with adoption guidance.
	var errMsg *sync.SkillErrorMsg
	for _, ev := range events {
		if e, ok := ev.(sync.SkillErrorMsg); ok {
			errMsg = &e
			break
		}
	}
	if errMsg == nil {
		t.Fatal("expected SkillErrorMsg, none emitted")
	}
	if !strings.Contains(errMsg.Err.Error(), "real directory") {
		t.Errorf("error should mention 'real directory', got: %v", errMsg.Err)
	}
	if !strings.Contains(errMsg.Err.Error(), "scribe adopt qa") {
		t.Errorf("error should mention 'scribe adopt qa', got: %v", errMsg.Err)
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
