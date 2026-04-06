# Plan 4: Package Sync & Polish

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the package install/update flow in the sync engine, add a TOFU trust prompt, implement `scribe upgrade` for self-updates, create the Scribe agent skill, and polish the README.

**Architecture:** The sync engine (`internal/sync/`) gains a `CommandExecutor` interface for running shell commands and a trust-prompt flow integrated via the existing event system. The `Syncer.apply()` method replaces its "not yet implemented" skip with real install/update logic. A new `internal/upgrade/` package handles self-update via install method detection. The formatter interface grows package-specific callbacks.

**Tech Stack:** Go 1.26, existing Cobra + Charm stack, `os/exec` for command execution, `github.com/google/go-github/v69` for release API.

**Depends on:** Plans 1-3 (catalog/YAML manifest, package events, state fields, trust.go).

---

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Executor | `internal/sync/executor.go` | Create | CommandExecutor interface, ShellExecutor implementation |
| Executor | `internal/sync/executor_test.go` | Create | Timeout, success, failure, stderr capture tests |
| Sync Engine | `internal/sync/syncer.go` | Modify | Replace package skip with install/update flow |
| Sync Engine | `internal/sync/syncer_test.go` | Create | Package install/update/deny/hash-mismatch tests |
| Events | `internal/sync/events.go` | Modify | Add PackageHashMismatchMsg, PackageUpdateMsg |
| Formatter | `internal/workflow/formatter.go` | Modify | Add package lifecycle callbacks |
| Formatter | `internal/workflow/formatter_text.go` | Modify | Package output: prompt result, install progress, errors |
| Formatter | `internal/workflow/formatter_json.go` | Modify | Package results in JSON output |
| Formatter | `internal/workflow/formatter_test.go` | Modify | Package event coverage |
| Workflow | `internal/workflow/sync.go` | Modify | Wire package events in Emit callback, pass TrustAll flag |
| Workflow | `internal/workflow/bag.go` | Modify | Add TrustAllFlag field |
| Cmd | `cmd/sync.go` | Modify | Add `--trust-all` flag |
| Upgrade | `internal/upgrade/upgrade.go` | Create | Upgrader: Check, Apply, install method detection |
| Upgrade | `internal/upgrade/upgrade_test.go` | Create | Version comparison, method detection tests |
| Cmd | `cmd/upgrade.go` | Create | `scribe upgrade` and `scribe upgrade --check` |
| Cmd | `cmd/root.go` | Modify | Register upgrade command |
| Skill | `skills/scribe-agent/SKILL.md` | Create | Agent skill teaching AI agents how to use Scribe |
| Docs | `README.md` | Modify | Updated commands, upgrade section, scribe.yaml examples |

---

### Task 1: Command executor

**Files:**
- Create: `internal/sync/executor.go`
- Create: `internal/sync/executor_test.go`

- [ ] **Step 1: Write failing tests for ShellExecutor**

```go
package sync_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/sync"
)

func TestShellExecutor_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	stdout, stderr, err := exec.Execute(context.Background(), "echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "hello\n")
	}
	if stderr != "" {
		t.Errorf("stderr: got %q, want empty", stderr)
	}
}

func TestShellExecutor_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	_, stderr, err := exec.Execute(context.Background(), "echo fail >&2 && exit 1", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if stderr != "fail\n" {
		t.Errorf("stderr: got %q, want %q", stderr, "fail\n")
	}
}

func TestShellExecutor_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	_, _, err := exec.Execute(context.Background(), "sleep 60", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestShellExecutor_ContextCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := &sync.ShellExecutor{}
	_, _, err := exec.Execute(ctx, "sleep 60", 5*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
```

- [ ] **Step 2: Implement executor**

```go
package sync

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// CommandExecutor runs shell commands and captures output.
type CommandExecutor interface {
	Execute(ctx context.Context, command string, timeout time.Duration) (stdout, stderr string, err error)
}

// ShellExecutor runs commands via sh -c with process group management.
type ShellExecutor struct{}

func (e *ShellExecutor) Execute(ctx context.Context, command string, timeout time.Duration) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if ctx.Err() != nil {
		// Kill the entire process group on timeout/cancel.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %s", timeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}
```

- [ ] **Step 3: Verify tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/sync/ -run TestShellExecutor -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/sync/executor.go internal/sync/executor_test.go
git commit -m "[agent] Add CommandExecutor interface and ShellExecutor implementation

Step 1 of plan4-package-polish: shell command execution with timeout and process group kill"
```

---

### Task 2: Extend events and formatter for packages

**Files:**
- Modify: `internal/sync/events.go`
- Modify: `internal/workflow/formatter.go`
- Modify: `internal/workflow/formatter_text.go`
- Modify: `internal/workflow/formatter_json.go`
- Modify: `internal/workflow/formatter_test.go`

- [ ] **Step 1: Add missing package events**

In `internal/sync/events.go`, add after the existing package events:

```go
// PackageHashMismatchMsg is sent when a previously approved command has changed.
type PackageHashMismatchMsg struct {
	Name       string
	OldCommand string
	NewCommand string
	Source     string
}

// PackageUpdateMsg is sent when a package update command begins.
type PackageUpdateMsg struct{ Name string }

// PackageUpdatedMsg is sent when a package update completes successfully.
type PackageUpdatedMsg struct{ Name string }
```

- [ ] **Step 2: Extend the Formatter interface**

In `internal/workflow/formatter.go`, add to the `Formatter` interface:

```go
	// Package lifecycle
	OnPackageInstallPrompt(name, command, source string)
	OnPackageApproved(name string)
	OnPackageDenied(name string)
	OnPackageSkipped(name, reason string)
	OnPackageInstalling(name string)
	OnPackageInstalled(name string)
	OnPackageUpdating(name string)
	OnPackageUpdated(name string)
	OnPackageError(name string, err error, stderr string)
	OnPackageHashMismatch(name, oldCmd, newCmd, source string)
```

- [ ] **Step 3: Implement text formatter package methods**

In `internal/workflow/formatter_text.go`:

```go
func (f *textFormatter) OnPackageInstallPrompt(name, command, source string) {
	fmt.Fprintf(f.errOut, "  %-20s requires approval: %s\n", name, command)
}

func (f *textFormatter) OnPackageApproved(name string) {
	fmt.Fprintf(f.out, "  %-20s approved\n", name)
}

func (f *textFormatter) OnPackageDenied(name string) {
	fmt.Fprintf(f.out, "  %-20s denied, skipping\n", name)
}

func (f *textFormatter) OnPackageSkipped(name, reason string) {
	fmt.Fprintf(f.out, "  %-20s skipped (%s)\n", name, reason)
}

func (f *textFormatter) OnPackageInstalling(name string) {
	fmt.Fprintf(f.out, "  %-20s installing...\n", name)
}

func (f *textFormatter) OnPackageInstalled(name string) {
	fmt.Fprintf(f.out, "  %-20s installed\n", name)
}

func (f *textFormatter) OnPackageUpdating(name string) {
	fmt.Fprintf(f.out, "  %-20s updating...\n", name)
}

func (f *textFormatter) OnPackageUpdated(name string) {
	fmt.Fprintf(f.out, "  %-20s updated\n", name)
}

func (f *textFormatter) OnPackageError(name string, err error, stderr string) {
	msg := err.Error()
	if stderr != "" {
		msg += ": " + stderr
	}
	fmt.Fprintf(f.errOut, "  %-20s error: %s\n", name, msg)
}

func (f *textFormatter) OnPackageHashMismatch(name, oldCmd, newCmd, source string) {
	fmt.Fprintf(f.errOut, "  %-20s command changed:\n", name)
	fmt.Fprintf(f.errOut, "    was: %s\n", oldCmd)
	fmt.Fprintf(f.errOut, "    now: %s\n", newCmd)
}
```

- [ ] **Step 4: Implement JSON formatter package methods**

In `internal/workflow/formatter_json.go`:

```go
func (f *jsonFormatter) OnPackageInstallPrompt(name, command, source string) {
	// JSON mode doesn't prompt — trust-all or skip.
}

func (f *jsonFormatter) OnPackageApproved(name string) {}

func (f *jsonFormatter) OnPackageDenied(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "denied",
	})
}

func (f *jsonFormatter) OnPackageSkipped(name, reason string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "skipped",
		Status: reason,
	})
}

func (f *jsonFormatter) OnPackageInstalling(name string) {}

func (f *jsonFormatter) OnPackageInstalled(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "package_installed",
	})
}

func (f *jsonFormatter) OnPackageUpdating(name string) {}

func (f *jsonFormatter) OnPackageUpdated(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "package_updated",
	})
}

func (f *jsonFormatter) OnPackageError(name string, err error, stderr string) {
	if f.current == nil {
		return
	}
	errMsg := err.Error()
	if stderr != "" {
		errMsg += ": " + stderr
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "error",
		Error:  errMsg,
	})
}

func (f *jsonFormatter) OnPackageHashMismatch(name, oldCmd, newCmd, source string) {}
```

- [ ] **Step 5: Verify compilation and tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./internal/workflow/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/sync/events.go internal/workflow/formatter.go internal/workflow/formatter_text.go internal/workflow/formatter_json.go internal/workflow/formatter_test.go
git commit -m "[agent] Add package lifecycle events and formatter support

Step 2 of plan4-package-polish: extend Formatter interface with package-specific callbacks"
```

---

### Task 3: Package install/update flow in syncer

**Files:**
- Modify: `internal/sync/syncer.go`
- Create: `internal/sync/syncer_test.go`

This is the core task. It replaces the "not yet implemented" skip in `Syncer.apply()` with real package handling.

- [ ] **Step 1: Write failing tests for package install flow**

```go
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

// mockExecutor records calls and returns configured results.
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

	// Pre-approve by recording in state.
	st.RecordInstall("superpowers", state.InstalledSkill{
		Type:       "package",
		InstallCmd: "claude plugin install superpowers",
		UpdateCmd:  "claude plugin update superpowers",
		CmdHash:    sync.CommandHash("claude plugin install superpowers", "claude plugin update superpowers"),
		Approval:   "approved",
		ApprovedAt: time.Now(),
	})

	err := syncer.RunWithDiff(context.Background(), statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executor.commands) != 1 {
		t.Fatalf("expected 1 command executed, got %d", len(executor.commands))
	}
	if executor.commands[0] != "claude plugin install superpowers" {
		t.Errorf("wrong command: %q", executor.commands[0])
	}

	// Should have recorded in state.
	installed, ok := st.Installed["superpowers"]
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
		// No ApprovalFunc set — simulates non-interactive.
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

	err := syncer.RunWithDiff(context.Background(), statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have emitted PackageSkippedMsg (no approval func, non-interactive).
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

	err := syncer.RunWithDiff(context.Background(), statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have emitted PackageErrorMsg.
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

	// Should NOT be recorded in state.
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
	hash := sync.CommandHash(installCmd, updateCmd)

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"superpowers": {
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
	}}

	err := syncer.RunWithDiff(context.Background(), statuses, st)
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

	err := syncer.RunWithDiff(context.Background(), statuses, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No update command -> should skip with warning.
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

	// Should NOT have executed any commands.
	if len(executor.commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(executor.commands))
	}
}
```

- [ ] **Step 2: Add Executor, TrustAll, and ApprovalFunc to Syncer**

In `internal/sync/syncer.go`, modify the `Syncer` struct:

```go
// Syncer wires manifest, github, targets, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client   GitHubFetcher
	Targets  []targets.Target
	Emit     func(any) // receives events defined in events.go
	Executor CommandExecutor

	// TrustAll skips approval prompts for packages (--trust-all flag).
	TrustAll bool

	// ApprovalFunc is called when a package needs interactive approval.
	// Returns true if approved, false if denied.
	// If nil and TrustAll is false, packages needing approval are skipped.
	ApprovalFunc func(name, command, source string) bool
}
```

- [ ] **Step 3: Implement package handling in apply()**

Replace the package skip block in `Syncer.apply()` with:

```go
			if sk.IsPackage {
				s.applyPackage(ctx, sk, st, &summary)
				continue
			}
```

Then add the `applyPackage` method:

```go
// defaultPackageTimeout is used when the catalog entry has no timeout.
const defaultPackageTimeout = 5 * time.Minute

func (s *Syncer) applyPackage(ctx context.Context, sk SkillStatus, st *state.State, summary *SyncCompleteMsg) {
	installCmd := sk.Entry.Install
	updateCmd := sk.Entry.Update
	newHash := CommandHash(installCmd, updateCmd)

	switch sk.Status {
	case StatusMissing:
		// Check approval.
		approved := s.checkApproval(sk, st, newHash, installCmd, updateCmd)
		if !approved {
			s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "approval_required"})
			summary.Skipped++
			return
		}

		s.emit(PackageInstallingMsg{Name: sk.Name})

		timeout := time.Duration(sk.Entry.Timeout) * time.Second
		if timeout == 0 {
			timeout = defaultPackageTimeout
		}

		_, stderr, err := s.Executor.Execute(ctx, installCmd, timeout)
		if err != nil {
			s.emit(PackageErrorMsg{Name: sk.Name, Err: err, Stderr: stderr})
			summary.Failed++
			return
		}

		// Fetch latest SHA for state tracking.
		latestSHA := ""
		src, parseErr := manifest.ParseSource(sk.Entry.Source)
		if parseErr == nil {
			sha, shaErr := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
			if shaErr == nil {
				latestSHA = sha
			}
		}

		st.RecordInstall(sk.Name, state.InstalledSkill{
			Version:    src.Ref,
			CommitSHA:  latestSHA,
			Source:     sk.Entry.Source,
			Type:       "package",
			InstallCmd: installCmd,
			UpdateCmd:  updateCmd,
			CmdHash:    newHash,
			Approval:   "approved",
			ApprovedAt: time.Now().UTC(),
		})
		if err := st.Save(); err != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
		}

		s.emit(PackageInstalledMsg{Name: sk.Name})
		summary.Installed++

	case StatusOutdated:
		if updateCmd == "" {
			s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "no update command"})
			summary.Skipped++
			return
		}

		// Check if commands changed — re-approve if hash mismatch.
		installed := st.Installed[sk.Name]
		if installed.CmdHash != "" && installed.CmdHash != newHash {
			s.emit(PackageHashMismatchMsg{
				Name:       sk.Name,
				OldCommand: installed.InstallCmd,
				NewCommand: installCmd,
				Source:     sk.Entry.Source,
			})
			approved := s.checkApproval(sk, st, newHash, installCmd, updateCmd)
			if !approved {
				s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "approval_required"})
				summary.Skipped++
				return
			}
		}

		s.emit(PackageUpdateMsg{Name: sk.Name})

		timeout := time.Duration(sk.Entry.Timeout) * time.Second
		if timeout == 0 {
			timeout = defaultPackageTimeout
		}

		_, stderr, err := s.Executor.Execute(ctx, updateCmd, timeout)
		if err != nil {
			s.emit(PackageErrorMsg{Name: sk.Name, Err: err, Stderr: stderr})
			summary.Failed++
			return
		}

		// Update state with new SHA.
		latestSHA := ""
		src, parseErr := manifest.ParseSource(sk.Entry.Source)
		if parseErr == nil {
			sha, shaErr := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
			if shaErr == nil {
				latestSHA = sha
			}
		}

		existing := st.Installed[sk.Name]
		existing.CommitSHA = latestSHA
		existing.InstallCmd = installCmd
		existing.UpdateCmd = updateCmd
		existing.CmdHash = newHash
		st.Installed[sk.Name] = existing
		if err := st.Save(); err != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
		}

		s.emit(PackageUpdatedMsg{Name: sk.Name})
		summary.Updated++
	}
}

// checkApproval determines if a package command is approved to run.
// Returns true if: TrustAll is set, state already has matching CmdHash with approval,
// or ApprovalFunc returns true.
func (s *Syncer) checkApproval(sk SkillStatus, st *state.State, newHash, installCmd, updateCmd string) bool {
	// Trust-all mode: auto-approve everything.
	if s.TrustAll {
		return true
	}

	// Check if already approved with matching hash.
	if installed, ok := st.Installed[sk.Name]; ok {
		if installed.Approval == "approved" && installed.CmdHash == newHash {
			return true
		}
	}

	// Interactive approval.
	if s.ApprovalFunc != nil {
		s.emit(PackageInstallPromptMsg{
			Name:    sk.Name,
			Command: installCmd,
			Source:  sk.Entry.Source,
		})
		approved := s.ApprovalFunc(sk.Name, installCmd, sk.Entry.Source)
		if approved {
			s.emit(PackageApprovedMsg{Name: sk.Name})
			// Record approval in state.
			existing := st.Installed[sk.Name]
			existing.CmdHash = newHash
			existing.Approval = "approved"
			existing.ApprovedAt = time.Now().UTC()
			existing.InstallCmd = installCmd
			existing.UpdateCmd = updateCmd
			st.Installed[sk.Name] = existing
			return true
		}
		s.emit(PackageDeniedMsg{Name: sk.Name})
		return false
	}

	// No approval mechanism — skip.
	return false
}
```

- [ ] **Step 4: Verify tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/sync/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/sync/syncer.go internal/sync/syncer_test.go internal/sync/events.go
git commit -m "[agent] Implement package install/update flow in syncer

Step 3 of plan4-package-polish: replaces 'not yet implemented' skip with real TOFU approval, command execution, and state recording"
```

---

### Task 4: Wire package events in workflow and add --trust-all flag

**Files:**
- Modify: `internal/workflow/bag.go`
- Modify: `internal/workflow/sync.go`
- Modify: `cmd/sync.go`

- [ ] **Step 1: Add TrustAllFlag to Bag**

In `internal/workflow/bag.go`, add to the `Bag` struct:

```go
	TrustAllFlag bool // --trust-all: approve all package commands without prompting
```

- [ ] **Step 2: Wire package events in StepSyncSkills**

In `internal/workflow/sync.go`, modify `StepSyncSkills` to set Executor and TrustAll on the syncer, and handle package events in the Emit callback:

```go
func StepSyncSkills(ctx context.Context, b *Bag) error {
	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Targets:  b.Targets,
		Executor: &sync.ShellExecutor{},
		TrustAll: b.TrustAllFlag,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus
				b.Formatter.OnSkillResolved(m.Name, m.SkillStatus)
			case sync.SkillSkippedMsg:
				b.Formatter.OnSkillSkipped(m.Name, resolved[m.Name])
			case sync.SkillDownloadingMsg:
				b.Formatter.OnSkillDownloading(m.Name)
			case sync.SkillInstalledMsg:
				b.Formatter.OnSkillInstalled(m.Name, m.Version, m.Updated)
			case sync.SkillErrorMsg:
				b.Formatter.OnSkillError(m.Name, m.Err)
			case sync.SyncCompleteMsg:
				b.Formatter.OnSyncComplete(m)

			// Package events
			case sync.PackageInstallPromptMsg:
				b.Formatter.OnPackageInstallPrompt(m.Name, m.Command, m.Source)
			case sync.PackageApprovedMsg:
				b.Formatter.OnPackageApproved(m.Name)
			case sync.PackageDeniedMsg:
				b.Formatter.OnPackageDenied(m.Name)
			case sync.PackageSkippedMsg:
				b.Formatter.OnPackageSkipped(m.Name, m.Reason)
			case sync.PackageInstallingMsg:
				b.Formatter.OnPackageInstalling(m.Name)
			case sync.PackageInstalledMsg:
				b.Formatter.OnPackageInstalled(m.Name)
			case sync.PackageUpdateMsg:
				b.Formatter.OnPackageUpdating(m.Name)
			case sync.PackageUpdatedMsg:
				b.Formatter.OnPackageUpdated(m.Name)
			case sync.PackageErrorMsg:
				b.Formatter.OnPackageError(m.Name, m.Err, m.Stderr)
			case sync.PackageHashMismatchMsg:
				b.Formatter.OnPackageHashMismatch(m.Name, m.OldCommand, m.NewCommand, m.Source)
			}
		},
	}

	for _, teamRepo := range b.Repos {
		clear(resolved)
		b.Formatter.OnRegistryStart(teamRepo)

		if err := syncer.Run(ctx, teamRepo, b.State); err != nil {
			return err
		}

		for name := range resolved {
			b.State.AddRegistry(name, teamRepo)
		}
		if err := b.State.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 3: Add --trust-all flag to sync command**

In `cmd/sync.go`:

```go
func init() {
	syncCmd.Flags().Bool("json", false, "Output machine-readable JSON (for CI/agents)")
	syncCmd.Flags().String("registry", "", "Sync only this registry (owner/repo or repo name)")
	syncCmd.Flags().Bool("trust-all", false, "Approve all package install commands without prompting")
	syncCmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
	syncCmd.Flags().MarkHidden("all")
}

func runSync(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	repoFlag, _ := cmd.Flags().GetString("registry")
	trustAllFlag, _ := cmd.Flags().GetBool("trust-all")

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		RepoFlag:         repoFlag,
		TrustAllFlag:     trustAllFlag,
		FilterRegistries: filterRegistries,
	}
	return workflow.Run(cmd.Context(), workflow.SyncSteps(), bag)
}
```

- [ ] **Step 4: Verify build and tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/bag.go internal/workflow/sync.go cmd/sync.go
git commit -m "[agent] Wire package events in workflow and add --trust-all flag

Step 4 of plan4-package-polish: connects executor, trust-all, and package event routing through the formatter"
```

---

### Task 5: Non-interactive package handling

**Files:**
- Modify: `internal/workflow/sync.go`

This task ensures packages work correctly in CI/non-TTY environments.

- [ ] **Step 1: Add interactive approval function for TTY mode**

In `internal/workflow/sync.go`, modify `StepSyncSkills` to set `ApprovalFunc` when running in a TTY:

```go
import (
	"os"

	"github.com/mattn/go-isatty"
	"charm.land/huh/v2"
)

// Inside StepSyncSkills, after creating the syncer:
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if isTTY && !b.TrustAllFlag {
		syncer.ApprovalFunc = func(name, command, source string) bool {
			var approved bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Package %q wants to run:", name)).
				Description(command).
				Affirmative("Approve").
				Negative("Deny").
				Value(&approved).
				Run()
			if err != nil {
				return false
			}
			return approved
		}
	}
```

- [ ] **Step 2: Verify non-TTY skips packages gracefully**

The existing logic already handles this: when `ApprovalFunc` is nil and `TrustAll` is false, `checkApproval` returns false and the package is skipped with reason "approval_required". The JSON formatter includes the skip reason.

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./internal/sync/ -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/workflow/sync.go
git commit -m "[agent] Add interactive TOFU approval prompt for TTY mode

Step 5 of plan4-package-polish: huh.Confirm prompt for package approval, graceful skip in non-TTY"
```

---

### Task 6: scribe upgrade -- self-update

**Files:**
- Create: `internal/upgrade/upgrade.go`
- Create: `internal/upgrade/upgrade_test.go`
- Create: `cmd/upgrade.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Write failing tests for upgrade logic**

```go
package upgrade_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/upgrade"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer patch", "0.5.0", "0.5.1", true},
		{"newer minor", "0.5.0", "0.6.0", true},
		{"newer major", "0.5.0", "1.0.0", true},
		{"same version", "0.5.0", "0.5.0", false},
		{"older version", "0.6.0", "0.5.0", false},
		{"with v prefix", "v0.5.0", "v0.6.0", true},
		{"dev version", "dev", "0.5.0", true},
		{"dev is always outdated", "dev", "0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upgrade.IsNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestDetectMethod_GoBin(t *testing.T) {
	method := upgrade.DetectMethodFromPath("/Users/test/go/bin/scribe")
	if method != upgrade.MethodGo {
		t.Errorf("got %v, want MethodGo", method)
	}
}

func TestDetectMethod_Homebrew(t *testing.T) {
	method := upgrade.DetectMethodFromPath("/opt/homebrew/bin/scribe")
	if method != upgrade.MethodHomebrew {
		t.Errorf("got %v, want MethodHomebrew", method)
	}
}

func TestDetectMethod_Binary(t *testing.T) {
	method := upgrade.DetectMethodFromPath("/usr/local/bin/scribe")
	if method != upgrade.MethodBinary {
		t.Errorf("got %v, want MethodBinary", method)
	}
}
```

- [ ] **Step 2: Implement upgrade package**

```go
package upgrade

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"

	gh "github.com/google/go-github/v69/github"
)

// InstallMethod identifies how scribe was installed.
type InstallMethod int

const (
	MethodBinary   InstallMethod = iota // downloaded binary
	MethodHomebrew                      // brew install
	MethodGo                            // go install
)

func (m InstallMethod) String() string {
	switch m {
	case MethodHomebrew:
		return "homebrew"
	case MethodGo:
		return "go install"
	default:
		return "binary"
	}
}

// Release holds info about a GitHub release.
type Release struct {
	Version string
	URL     string // HTML URL for the release page
}

// Upgrader checks for and applies updates.
type Upgrader struct {
	Owner   string
	Repo    string
	Current string // current version (injected from cmd.Version)
}

// New creates an Upgrader for the Scribe repository.
func New(currentVersion string) *Upgrader {
	return &Upgrader{
		Owner:   "Naoray",
		Repo:    "scribe",
		Current: currentVersion,
	}
}

// Check fetches the latest release and compares against current.
func (u *Upgrader) Check(ctx context.Context) (*Release, bool, error) {
	client := gh.NewClient(nil)
	rel, _, err := client.Repositories.GetLatestRelease(ctx, u.Owner, u.Repo)
	if err != nil {
		return nil, false, fmt.Errorf("check latest release: %w", err)
	}

	latest := rel.GetTagName()
	release := &Release{
		Version: latest,
		URL:     rel.GetHTMLURL(),
	}

	return release, IsNewer(u.Current, latest), nil
}

// Apply performs the upgrade using the detected install method.
func (u *Upgrader) Apply(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	method := DetectMethodFromPath(exe)

	switch method {
	case MethodHomebrew:
		cmd := exec.CommandContext(ctx, "brew", "upgrade", "Naoray/tap/scribe")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	case MethodGo:
		cmd := exec.CommandContext(ctx, "go", "install", "github.com/Naoray/scribe/cmd/scribe@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "GOBIN="+gobin(exe))
		return cmd.Run()

	default:
		return fmt.Errorf("automatic binary replacement not supported yet — download from https://github.com/Naoray/scribe/releases/latest\n\nDetected OS: %s/%s\nBinary: %s", runtime.GOOS, runtime.GOARCH, exe)
	}
}

// IsNewer reports whether latest is newer than current.
func IsNewer(current, latest string) bool {
	if current == "dev" {
		return true
	}
	c := ensureV(current)
	l := ensureV(latest)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return current != latest
	}
	return semver.Compare(c, l) < 0
}

// DetectMethodFromPath guesses how scribe was installed from the binary path.
func DetectMethodFromPath(path string) InstallMethod {
	if strings.Contains(path, "homebrew") || strings.Contains(path, "Cellar") {
		return MethodHomebrew
	}
	if strings.Contains(path, "/go/bin/") || strings.Contains(path, "/gobin/") {
		return MethodGo
	}
	return MethodBinary
}

func ensureV(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// gobin extracts the GOBIN directory from the executable path.
func gobin(exe string) string {
	idx := strings.LastIndex(exe, "/")
	if idx >= 0 {
		return exe[:idx]
	}
	return ""
}
```

- [ ] **Step 3: Implement the upgrade command**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/upgrade"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade scribe to the latest version",
	RunE:  runUpgrade,
}

func init() {
	upgradeCmd.Flags().Bool("check", false, "Check for updates without installing")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	checkOnly, _ := cmd.Flags().GetBool("check")

	upgrader := upgrade.New(Version)

	release, newer, err := upgrader.Check(cmd.Context())
	if err != nil {
		return err
	}

	if !newer {
		fmt.Fprintf(cmd.OutOrStdout(), "scribe %s is already the latest version\n", Version)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "scribe %s is available (current: %s)\n", release.Version, Version)

	if checkOnly {
		fmt.Fprintf(cmd.OutOrStdout(), "Release: %s\n", release.URL)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "upgrading...\n")
	if err := upgrader.Apply(cmd.Context()); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "upgraded to %s\n", release.Version)
	return nil
}
```

- [ ] **Step 4: Register upgrade command in root.go**

In `cmd/root.go`, add to `init()`:

```go
	rootCmd.AddCommand(upgradeCmd)
```

- [ ] **Step 5: Verify build and tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./internal/upgrade/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/upgrade/upgrade.go internal/upgrade/upgrade_test.go cmd/upgrade.go cmd/root.go
git commit -m "[agent] Add scribe upgrade command with install method detection

Step 6 of plan4-package-polish: supports brew, go install, and binary detection; version check via GitHub Releases API"
```

---

### Task 7: Agent skill

**Files:**
- Create: `skills/scribe-agent/SKILL.md`

- [ ] **Step 1: Create the skill file**

```markdown
---
name: scribe
description: Team skill sync CLI for AI coding agents
version: "1.0"
author: Naoray
---

# Scribe CLI

Scribe keeps AI coding agent skills in sync across a team. One GitHub repo holds the manifest; teammates run `scribe sync` to stay current.

## When to use

- User mentions keeping skills in sync, sharing skills with teammates, or managing AI agent configuration
- User asks about installing or updating skills for Claude Code or Cursor
- User wants to set up a team skills registry
- User is debugging skill sync issues

## Commands

### Connect to a team registry

```bash
scribe connect <owner/repo>
```

Connects to a team skills registry and runs an initial sync. The registry is a GitHub repo with a `scribe.yaml` manifest.

### Sync skills

```bash
scribe sync                          # sync all connected registries
scribe sync --registry <owner/repo>  # sync one registry only
scribe sync --trust-all              # auto-approve package install commands
scribe sync --json                   # machine-readable output for CI
```

### List installed skills

```bash
scribe list                          # interactive TUI with detail pane
scribe list --json                   # machine-readable JSON
```

Statuses: current (matches team version), outdated (team has newer), missing (not installed), extra (installed but not in team manifest).

### Add a skill to the registry

```bash
scribe add                           # interactive picker
scribe add <skill-name>              # by name
scribe add --json                    # machine-readable output
```

Discovers local skills (from `.claude/skills/`, `.ai/skills/`, `.cursor/rules/`), lets you pick one, and pushes it to the team registry.

### Create a new registry

```bash
scribe create registry
```

Interactive wizard that scaffolds a new team skills repo on GitHub with a `scribe.yaml` manifest.

### List connected registries

```bash
scribe registry list
```

### Self-update

```bash
scribe upgrade                       # check and install latest version
scribe upgrade --check               # check only, don't install
```

## Manifest format (scribe.yaml)

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: my-team
  description: My team's skill stack

catalog:
  - name: gstack
    source: "github:garrytan/gstack@v0.12.9.0"
    author: garrytan

  - name: deploy
    source: "github:MyOrg/team-skills@main"
    path: krishan/deploy
    author: krishan

  - name: superpowers
    source: "github:obra/superpowers@main"
    type: package
    install: "claude /plugin install superpowers@claude-plugins-official"
    author: obra

targets:
  default:
    - claude
    - cursor
```

## Data locations

- `~/.scribe/state.json` -- installed skills and sync state
- `~/.scribe/skills/` -- canonical skill store (symlinked by targets)
- `~/.scribe/config.toml` -- user preferences (token, team repos)

## Troubleshooting

- **"not connected"**: Run `scribe connect <owner/repo>` first
- **Permission denied on private repo**: Run `gh auth login` or set `GITHUB_TOKEN`
- **Package approval required**: Use `--trust-all` flag or approve interactively
```

- [ ] **Step 2: Commit**

```bash
git add skills/scribe-agent/SKILL.md
git commit -m "[agent] Add Scribe agent skill for AI coding agents

Step 7 of plan4-package-polish: teaches agents how to use all Scribe commands"
```

---

### Task 8: README updates

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the manifest example from TOML to YAML**

Replace the `scribe.toml` example in the "Manual setup" section with `scribe.yaml`:

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
  description: Artistfy dev team skill stack

catalog:
  - name: gstack
    source: "github:garrytan/gstack@v0.12.9.0"
    author: garrytan

  - name: deploy
    source: "github:ArtistfyHQ/team-skills@main"
    path: krishan/deploy
    author: krishan
```

- [ ] **Step 2: Update the commands table**

Replace the commands table with:

```markdown
| Command | What it does |
|---|---|
| `scribe connect <owner/repo>` | Connect to a team skills repo and run an initial sync |
| `scribe sync` | Install missing skills, update outdated ones |
| `scribe sync --trust-all` | Auto-approve package install commands |
| `scribe list` | Show all skills with interactive detail view |
| `scribe add [name]` | Add a skill to the team registry (interactive picker or by name) |
| `scribe create registry` | Scaffold a new team skills registry on GitHub |
| `scribe registry list` | List connected registries |
| `scribe upgrade` | Check for and install the latest version |
```

- [ ] **Step 3: Add upgrade section after the Install/Updating section**

```markdown
### Self-update

Scribe can update itself:

```bash
scribe upgrade              # check and install latest version
scribe upgrade --check      # just check, show available version
```

Detects your install method (Homebrew, `go install`, or binary) and uses the appropriate update mechanism.
```

- [ ] **Step 4: Add packages section to the manifest docs**

After the catalog skill entries section, add:

```markdown
### Packages

Packages run shell commands instead of downloading skill files. Useful for tools that have their own installers:

```yaml
catalog:
  - name: superpowers
    source: "github:obra/superpowers@main"
    type: package
    install: "claude /plugin install superpowers@claude-plugins-official"
    update: "claude /plugin update superpowers"
    author: obra
    timeout: 300  # seconds (default: 300)
```

Package commands require one-time approval (TOFU model). Use `--trust-all` to skip prompts in CI.
```

- [ ] **Step 5: Verify README renders correctly**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && head -200 README.md
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "[agent] Update README with YAML manifest, packages, upgrade, and new commands

Step 8 of plan4-package-polish: documentation reflects current feature set"
```

---

## Dependency Graph

```
Task 1 (executor) ─────────────────┐
                                    ├── Task 3 (syncer install flow) ── Task 4 (workflow wiring) ── Task 5 (non-interactive)
Task 2 (events + formatter) ───────┘
Task 6 (upgrade) ── standalone
Task 7 (agent skill) ── standalone
Task 8 (README) ── after Tasks 4-7 are done
```

**Parallelizable:** Tasks 1+2 can run in parallel. Task 6 is independent of Tasks 1-5. Task 7 is independent of all others. Task 8 should run last.

## Verification

After all tasks are complete:

```bash
cd /Users/krishankonig/Workspace/bets/scribe
go build ./...
go test ./...
go run ./cmd/scribe --help
go run ./cmd/scribe upgrade --check
```
