# Package Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable `scribe sync` to install and update package-type catalog entries (like superpowers) via their declared `install`/`update` shell commands, with TOFU trust gating.

**Architecture:** The sync engine (`internal/sync/`) gains a `CommandExecutor` interface for running shell commands and a TOFU trust-prompt flow integrated via the existing event system. The `Syncer.apply()` method replaces its "not yet implemented" skip with real install/update logic. The `Formatter` interface grows package-specific callbacks. The `--trust-all` flag enables CI/agent use.

**Tech Stack:** Go 1.26, existing Cobra + Charm stack, `os/exec` for command execution.

**Design spec:** `docs/superpowers/specs/2026-04-06-mvp-design.md` section 8 (Package Sync: Install & Update).

---

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Executor | `internal/sync/executor.go` | Create | CommandExecutor interface, ShellExecutor implementation |
| Executor | `internal/sync/executor_test.go` | Create | Timeout, success, failure, stderr capture tests |
| Events | `internal/sync/events.go` | Modify | Add PackageHashMismatchMsg, PackageUpdateMsg, PackageUpdatedMsg |
| Sync Engine | `internal/sync/syncer.go` | Modify | Add Executor/TrustAll/ApprovalFunc fields, replace package skip with applyPackage() |
| Sync Engine | `internal/sync/syncer_test.go` | Create | Package install/update/deny/hash-mismatch tests |
| Formatter | `internal/workflow/formatter.go` | Modify | Add package lifecycle callbacks to Formatter interface |
| Formatter | `internal/workflow/formatter_text.go` | Modify | Package output: prompt, install progress, errors |
| Formatter | `internal/workflow/formatter_json.go` | Modify | Package results in JSON output |
| Workflow | `internal/workflow/bag.go` | Modify | Add TrustAllFlag field |
| Workflow | `internal/workflow/sync.go` | Modify | Wire Executor, TrustAll, ApprovalFunc, package events in Emit callback |
| Cmd | `cmd/sync.go` | Modify | Add `--trust-all` flag |

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

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run TestShellExecutor -v`
Expected: FAIL — `ShellExecutor` not defined.

- [ ] **Step 3: Implement executor**

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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -run TestShellExecutor -v`
Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sync/executor.go internal/sync/executor_test.go
git commit -m "[agent] Add CommandExecutor interface and ShellExecutor implementation

Step 1 of package-sync: shell command execution with timeout and process group kill"
```

---

### Task 2: Extend events and formatter for packages

**Files:**
- Modify: `internal/sync/events.go`
- Modify: `internal/workflow/formatter.go`
- Modify: `internal/workflow/formatter_text.go`
- Modify: `internal/workflow/formatter_json.go`

- [ ] **Step 1: Add missing package events**

In `internal/sync/events.go`, add after the existing `PackageErrorMsg` (after line 164):

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

In `internal/workflow/formatter.go`, add to the `Formatter` interface after the existing methods (after line 23, before `Flush`):

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

In `internal/workflow/formatter_text.go`, add before the `Flush` method:

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

In `internal/workflow/formatter_json.go`, add before the `Flush` method:

```go
func (f *jsonFormatter) OnPackageInstallPrompt(name, command, source string) {}

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

- [ ] **Step 5: Verify compilation passes**

Run: `go build ./...`
Expected: PASS — all formatter methods satisfy the interface.

- [ ] **Step 6: Commit**

```bash
git add internal/sync/events.go internal/workflow/formatter.go internal/workflow/formatter_text.go internal/workflow/formatter_json.go
git commit -m "[agent] Add package lifecycle events and formatter support

Step 2 of package-sync: extend Formatter interface with package-specific callbacks"
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

	// Should have recorded in state.
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
		// No ApprovalFunc set, TrustAll false — simulates non-interactive.
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

	err := syncer.RunWithDiff(context.Background(), "test/repo", statuses, st)
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
	hash := sync.CommandHash(installCmd, updateCmd)

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

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run TestApply_Package -v`
Expected: FAIL — `Executor`, `TrustAll`, `ApprovalFunc` fields don't exist on `Syncer`.

- [ ] **Step 3: Add Executor, TrustAll, and ApprovalFunc to Syncer**

In `internal/sync/syncer.go`, modify the `Syncer` struct (lines 29-34) to add the new fields, and add the `time` import:

```go
// Syncer wires manifest, github, tools, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client   GitHubFetcher
	Provider provider.Provider // optional — if set, used for discovery and fetch
	Tools    []tools.Tool
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

- [ ] **Step 4: Replace package skip with applyPackage call**

In `internal/sync/syncer.go`, replace lines 188-191 (the package skip block):

```go
			if sk.IsPackage {
				s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "package install not yet implemented"})
				summary.Skipped++
				continue
			}
```

With:

```go
			if sk.IsPackage {
				s.applyPackage(ctx, sk, registrySlug, st, &summary)
				continue
			}
```

- [ ] **Step 5: Implement applyPackage and checkApproval methods**

Add after the `emit` method in `internal/sync/syncer.go`:

```go
// defaultPackageTimeout is used when the catalog entry has no timeout.
const defaultPackageTimeout = 5 * time.Minute

func (s *Syncer) applyPackage(ctx context.Context, sk SkillStatus, registrySlug string, st *state.State, summary *SyncCompleteMsg) {
	installCmd := sk.Entry.Install
	updateCmd := sk.Entry.Update
	newHash := CommandHash(installCmd, updateCmd)
	qualifiedName := registrySlug + "/" + sk.Name

	switch sk.Status {
	case StatusMissing:
		approved := s.checkApproval(sk, qualifiedName, st, newHash, installCmd, updateCmd)
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

		version := "unknown"
		src, parseErr := manifest.ParseSource(sk.Entry.Source)
		if parseErr == nil {
			version = src.Ref
		}

		st.RecordInstall(qualifiedName, state.InstalledSkill{
			Version:    version,
			CommitSHA:  sk.LatestSHA,
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
		installed := st.Installed[qualifiedName]
		if installed.CmdHash != "" && installed.CmdHash != newHash {
			s.emit(PackageHashMismatchMsg{
				Name:       sk.Name,
				OldCommand: installed.InstallCmd,
				NewCommand: installCmd,
				Source:     sk.Entry.Source,
			})
			approved := s.checkApproval(sk, qualifiedName, st, newHash, installCmd, updateCmd)
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

		existing := st.Installed[qualifiedName]
		existing.CommitSHA = sk.LatestSHA
		existing.InstallCmd = installCmd
		existing.UpdateCmd = updateCmd
		existing.CmdHash = newHash
		st.Installed[qualifiedName] = existing
		if err := st.Save(); err != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
		}

		s.emit(PackageUpdatedMsg{Name: sk.Name})
		summary.Updated++
	}
}

// checkApproval determines if a package command is approved to run.
func (s *Syncer) checkApproval(sk SkillStatus, qualifiedName string, st *state.State, newHash, installCmd, updateCmd string) bool {
	if s.TrustAll {
		return true
	}

	if installed, ok := st.Installed[qualifiedName]; ok {
		if installed.Approval == "approved" && installed.CmdHash == newHash {
			return true
		}
	}

	if s.ApprovalFunc != nil {
		s.emit(PackageInstallPromptMsg{
			Name:    sk.Name,
			Command: installCmd,
			Source:  sk.Entry.Source,
		})
		approved := s.ApprovalFunc(sk.Name, installCmd, sk.Entry.Source)
		if approved {
			s.emit(PackageApprovedMsg{Name: sk.Name})
			existing := st.Installed[qualifiedName]
			existing.CmdHash = newHash
			existing.Approval = "approved"
			existing.ApprovedAt = time.Now().UTC()
			existing.InstallCmd = installCmd
			existing.UpdateCmd = updateCmd
			st.Installed[qualifiedName] = existing
			return true
		}
		s.emit(PackageDeniedMsg{Name: sk.Name})
		return false
	}

	return false
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/sync/ -run TestApply_Package -v`
Expected: all 5 tests PASS.

- [ ] **Step 7: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/sync/syncer.go internal/sync/syncer_test.go
git commit -m "[agent] Implement package install/update flow in syncer

Step 3 of package-sync: replaces 'not yet implemented' skip with TOFU approval, command execution, and state recording"
```

---

### Task 4: Wire package events in workflow and add --trust-all flag

**Files:**
- Modify: `internal/workflow/bag.go`
- Modify: `internal/workflow/sync.go`
- Modify: `cmd/sync.go`

- [ ] **Step 1: Add TrustAllFlag to Bag**

In `internal/workflow/bag.go`, add after `RepoFlag` (line 21):

```go
	TrustAllFlag bool // --trust-all: approve all package commands without prompting
```

- [ ] **Step 2: Wire Executor, TrustAll, and package events in StepSyncSkills**

In `internal/workflow/sync.go`, replace the `StepSyncSkills` function entirely. The key changes vs current code:
1. Add `Executor: &sync.ShellExecutor{}` to syncer
2. Add `TrustAll: b.TrustAllFlag` to syncer
3. Add package event cases to the Emit switch

```go
func StepSyncSkills(ctx context.Context, b *Bag) error {
	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
		Tools:    b.Tools,
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

	// Set interactive approval when in TTY mode.
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

	for _, teamRepo := range b.Repos {
		clear(resolved)
		b.Formatter.OnRegistryStart(teamRepo)

		if err := syncer.Run(ctx, teamRepo, b.State); err != nil {
			return err
		}

		if err := b.State.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	return nil
}
```

Also add the required imports at the top of `internal/workflow/sync.go`:

```go
import (
	"context"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"charm.land/huh/v2"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)
```

- [ ] **Step 3: Add --trust-all flag to sync command**

In `cmd/sync.go`, add the flag and wire it to the bag:

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

- [ ] **Step 4: Verify build and full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/bag.go internal/workflow/sync.go cmd/sync.go
git commit -m "[agent] Wire package events in workflow and add --trust-all flag

Step 4 of package-sync: connects executor, trust-all, approval prompt, and package event routing"
```

---

### Task 5: End-to-end smoke test

**Files:**
- No code changes — manual verification

This task verifies the full flow works with a real (or simulated) registry.

- [ ] **Step 1: Verify the full test suite passes**

Run: `go test ./... -count=1`
Expected: all tests PASS.

- [ ] **Step 2: Build and test the binary**

Run: `go build -o /tmp/scribe ./cmd/scribe && /tmp/scribe --help`
Expected: `sync` command shows `--trust-all` flag in help.

Run: `/tmp/scribe sync --help`
Expected: shows `--trust-all  Approve all package install commands without prompting`.

- [ ] **Step 3: Commit (if any fixes were needed)**

Only commit if Steps 1-2 revealed issues that needed fixing.

---

### Task 6: Add superpowers to artistfy/hq manifest

**Context:** The artistfy/hq registry at `/Users/krishankonig/Workspace/artistfy/hq` currently uses a legacy `scribe.toml` with no catalog entries. We need to migrate it to `scribe.yaml` and add the superpowers package entry.

**Files:**
- Modify (external): `/Users/krishankonig/Workspace/artistfy/hq/scribe.toml` → replace with `scribe.yaml`

- [ ] **Step 1: Migrate artistfy/hq to scribe.yaml**

In the artistfy/hq repo, create `scribe.yaml` with the superpowers entry:

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
  description: Artistfy team skills
catalog:
  - name: superpowers
    source: "github:obra/superpowers@main"
    type: package
    install: "claude /plugin install superpowers@claude-plugins-official"
    author: obra
    description: Core skills library for Claude Code
```

- [ ] **Step 2: Remove old scribe.toml**

```bash
cd /Users/krishankonig/Workspace/artistfy/hq
git rm scribe.toml
git add scribe.yaml
```

- [ ] **Step 3: Commit and push**

```bash
git commit -m "chore: migrate to scribe.yaml, add superpowers package"
git push
```

- [ ] **Step 4: Test scribe sync picks up superpowers**

```bash
cd /Users/krishankonig/Workspace/bets/scribe
go run ./cmd/scribe sync --trust-all
```

Expected: superpowers appears as a package entry and the install command is executed (or, if `claude` CLI is not available, a clear error message about the command failing).
