package sync_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
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

	installed, ok := st.Installed["test-repo/superpowers"]
	if !ok {
		t.Fatal("superpowers not in state after install")
	}
	if installed.Type != "package" {
		t.Errorf("type: got %q, want %q", installed.Type, "package")
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

	if _, ok := st.Installed["test-repo/broken-pkg"]; ok {
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
		"test-repo/superpowers": {
			Type:       "package",
			Source:     "github:obra/superpowers@main",
			InstallCmd: installCmd,
			UpdateCmd:  updateCmd,
			CmdHash:    hash,
			Approval:   "approved",
			CommitSHA:  "oldsha",
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
		"test-repo/minimal-pkg": {
			Type:      "package",
			Source:    "github:example/minimal@main",
			Approval:  "approved",
			CommitSHA: "oldsha",
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
