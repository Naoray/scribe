# Nono-Inspired Patterns for Scribe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt three high-leverage patterns from nono-cli that accelerate Scribe's agentic-first pivot: Claude Code hook integration (agent bridge), embedded registry catalog (zero-network first-run), and three-state config layering.

**Architecture:** Each pattern is independent and ships as its own commit/PR. The hook integration is highest leverage — it makes agents aware of Scribe automatically. Embedded catalogs eliminate network dependency for discovery. Config layering enables project-level overrides without breaking the single-file model.

**Tech Stack:** Go 1.26.1, Cobra, `encoding/json`, `os/exec`, `embed` stdlib package.

---

## File Map

```
internal/
  hooks/
    hooks.go              # NEW — hook installer (Claude Code settings.json writer)
    hooks_test.go         # NEW — hook installer tests
    scripts/
      scribe-hook.sh      # NEW — embedded hook script for Claude Code
  tools/
    claude.go             # MODIFY — add CanonicalTarget() method
  config/
    config.go             # MODIFY — add Hooks field, project-level layering
    config_test.go        # MODIFY — tests for new config fields
    layered.go            # NEW — three-state merge logic
    layered_test.go       # NEW — merge logic tests
  embed/
    embed.go              # NEW — embedded catalog registry data
    catalog.json          # NEW — default embedded catalog
cmd/
  hooks.go                # NEW — `scribe hooks install/uninstall/status` command
  hooks_test.go           # NEW — command tests
```

---

## Task 1: Claude Code Hook Script

The hook script is the bridge between Scribe and Claude Code. When a tool use fails inside Claude Code, this hook fires and tells the agent about Scribe's capabilities — what skills are installed, what's available, and how to sync.

**Files:**
- Create: `internal/hooks/scripts/scribe-hook.sh`

- [ ] **Step 1: Write the hook script**

This script runs as a `PostToolUseFailure` hook in Claude Code. It checks if scribe is installed, gathers skill state, and returns structured JSON that Claude Code injects into the agent's context.

```bash
#!/usr/bin/env bash
# scribe-hook.sh — Claude Code PostToolUseFailure hook
# Surfaces Scribe skill inventory to the agent when a tool use fails.
# Output: JSON with hookSpecificOutput.additionalContext for Claude Code.

set -euo pipefail

# Only act if scribe is available
if ! command -v scribe &>/dev/null; then
  exit 0
fi

# Gather installed skill list (fast, local-only)
SKILLS=$(scribe list --json 2>/dev/null || echo '[]')

# Count skills by status
INSTALLED=$(echo "$SKILLS" | jq 'length // 0' 2>/dev/null || echo 0)

# Check if any registries are connected
HAS_REGISTRY=$(scribe registry list --json 2>/dev/null | jq 'length > 0' 2>/dev/null || echo false)

# Build context message
CONTEXT="Scribe skill manager is available on this machine. "
CONTEXT+="$INSTALLED skills currently installed. "

if [ "$HAS_REGISTRY" = "true" ]; then
  CONTEXT+="Team registries connected — run 'scribe sync' to update skills. "
  CONTEXT+="Run 'scribe add <query>' to search and install skills from registries. "
else
  CONTEXT+="No registries connected — run 'scribe registry connect <owner/repo>' to add one. "
fi

CONTEXT+="Run 'scribe list' to see installed skills. "
CONTEXT+="Run 'scribe status' to check for available updates."

# Output structured JSON for Claude Code hook system
jq -n --arg ctx "$CONTEXT" '{
  hookSpecificOutput: {
    additionalContext: $ctx
  }
}'
```

- [ ] **Step 2: Verify script is valid bash**

Run: `bash -n internal/hooks/scripts/scribe-hook.sh`
Expected: No output (valid syntax)

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/scripts/scribe-hook.sh
git commit -m "feat: add Claude Code hook script for agent bridge

Surfaces Scribe skill inventory to agents via PostToolUseFailure hook.
Returns structured JSON with hookSpecificOutput.additionalContext."
```

---

## Task 2: Hook Installer Package

The installer reads Claude Code's `~/.claude/settings.json`, adds the hook entry, and writes the script to `~/.claude/hooks/`. Follows nono's pattern of embedding the script in the binary.

**Files:**
- Create: `internal/hooks/hooks.go`
- Create: `internal/hooks/hooks_test.go`

- [ ] **Step 1: Write the failing test for hook installation**

```go
package hooks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/hooks"
)

func TestInstallClaudeHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .claude dir (simulates Claude Code installed)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := hooks.InstallClaude()
	if err != nil {
		t.Fatalf("InstallClaude() error: %v", err)
	}

	if result.Status != hooks.Installed {
		t.Errorf("status = %v, want Installed", result.Status)
	}

	// Verify hook script written
	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("hook script not found: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("hook script not executable")
	}

	// Verify settings.json updated
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	if !contains(data, "scribe-hook.sh") {
		t.Error("settings.json does not reference scribe-hook.sh")
	}
}

func TestInstallClaudeHook_AlreadyInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Install once
	if _, err := hooks.InstallClaude(); err != nil {
		t.Fatal(err)
	}

	// Install again — should report AlreadyInstalled
	result, err := hooks.InstallClaude()
	if err != nil {
		t.Fatalf("second InstallClaude() error: %v", err)
	}
	if result.Status != hooks.AlreadyInstalled {
		t.Errorf("status = %v, want AlreadyInstalled", result.Status)
	}
}

func TestInstallClaudeHook_NoClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No .claude dir — should return error
	_, err := hooks.InstallClaude()
	if err == nil {
		t.Fatal("expected error when .claude dir missing")
	}
}

func TestUninstallClaudeHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Install then uninstall
	if _, err := hooks.InstallClaude(); err != nil {
		t.Fatal(err)
	}

	if err := hooks.UninstallClaude(); err != nil {
		t.Fatalf("UninstallClaude() error: %v", err)
	}

	// Script should be gone
	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("hook script still exists after uninstall")
	}

	// settings.json should not reference scribe
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		if contains(data, "scribe-hook.sh") {
			t.Error("settings.json still references scribe-hook.sh after uninstall")
		}
	}
}

func contains(data []byte, s string) bool {
	return len(data) > 0 && len(s) > 0 && string(data) != "" && indexOf(data, s) >= 0
}

func indexOf(data []byte, s string) int {
	for i := 0; i <= len(data)-len(s); i++ {
		if string(data[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/hooks/ -v -run TestInstall`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement the hook installer**

```go
package hooks

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed scripts/scribe-hook.sh
var hookScript []byte

// InstallStatus describes the outcome of a hook installation.
type InstallStatus int

const (
	Installed        InstallStatus = iota // freshly installed
	AlreadyInstalled                      // hook already present, no changes
	Updated                              // hook script updated to newer version
)

func (s InstallStatus) String() string {
	switch s {
	case Installed:
		return "installed"
	case AlreadyInstalled:
		return "already_installed"
	case Updated:
		return "updated"
	default:
		return "unknown"
	}
}

// InstallResult reports what happened during installation.
type InstallResult struct {
	Status     InstallStatus
	ScriptPath string
}

const (
	hookFileName = "scribe-hook.sh"
	hookEvent    = "PostToolUseFailure"
)

// InstallClaude installs the Scribe hook into Claude Code's settings.
// Creates ~/.claude/hooks/scribe-hook.sh and registers it in ~/.claude/settings.json.
func InstallClaude() (InstallResult, error) {
	claudeDir, err := claudeDir()
	if err != nil {
		return InstallResult{}, err
	}

	if _, err := os.Stat(claudeDir); err != nil {
		return InstallResult{}, fmt.Errorf("claude code not detected (~/.claude missing): %w", err)
	}

	// Write hook script
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create hooks dir: %w", err)
	}

	scriptPath := filepath.Join(hooksDir, hookFileName)
	existing, readErr := os.ReadFile(scriptPath)
	scriptExists := readErr == nil

	// Check if script content matches (for AlreadyInstalled vs Updated)
	if scriptExists && string(existing) == string(hookScript) {
		// Script identical — check settings.json too
		if settingsHasHook(claudeDir) {
			return InstallResult{Status: AlreadyInstalled, ScriptPath: scriptPath}, nil
		}
	}

	if err := os.WriteFile(scriptPath, hookScript, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("write hook script: %w", err)
	}

	// Update settings.json
	if err := addHookToSettings(claudeDir); err != nil {
		return InstallResult{}, fmt.Errorf("update settings.json: %w", err)
	}

	status := Installed
	if scriptExists {
		status = Updated
	}
	return InstallResult{Status: status, ScriptPath: scriptPath}, nil
}

// UninstallClaude removes the Scribe hook from Claude Code.
func UninstallClaude() error {
	claudeDir, err := claudeDir()
	if err != nil {
		return err
	}

	// Remove script
	scriptPath := filepath.Join(claudeDir, "hooks", hookFileName)
	if err := os.Remove(scriptPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove hook script: %w", err)
	}

	// Remove from settings.json
	return removeHookFromSettings(claudeDir)
}

// Status checks whether the Scribe hook is installed in Claude Code.
func Status() (installed bool, scriptPath string, err error) {
	claudeDir, err := claudeDir()
	if err != nil {
		return false, "", err
	}

	scriptPath = filepath.Join(claudeDir, "hooks", hookFileName)
	if _, err := os.Stat(scriptPath); err != nil {
		return false, "", nil
	}

	return settingsHasHook(claudeDir), scriptPath, nil
}

func claudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// --- settings.json helpers ---

// settingsHasHook checks if settings.json already contains our hook.
func settingsHasHook(claudeDir string) bool {
	settings, err := readSettings(claudeDir)
	if err != nil {
		return false
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	entries, ok := hooksMap[hookEvent].([]any)
	if !ok {
		return false
	}

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hookMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok {
				if filepath.Base(cmd) == hookFileName || cmd == "$HOME/.claude/hooks/"+hookFileName {
					return true
				}
			}
		}
	}
	return false
}

// addHookToSettings adds the Scribe hook entry to settings.json.
// Preserves all existing settings and hooks — only adds our entry.
func addHookToSettings(claudeDir string) error {
	settings, err := readSettings(claudeDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	// Get or create hooks map
	hooksMap, _ := settings["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	// Get or create event entries
	entries, _ := hooksMap[hookEvent].([]any)

	// Check if we already have a scribe entry
	found := false
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hookMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hookMap["command"].(string); filepath.Base(cmd) == hookFileName {
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		newEntry := map[string]any{
			"matcher": ".*",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "$HOME/.claude/hooks/" + hookFileName,
				},
			},
		}
		entries = append(entries, newEntry)
	}

	hooksMap[hookEvent] = entries
	settings["hooks"] = hooksMap

	return writeSettings(claudeDir, settings)
}

// removeHookFromSettings removes the Scribe hook entry from settings.json.
func removeHookFromSettings(claudeDir string) error {
	settings, err := readSettings(claudeDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	entries, ok := hooksMap[hookEvent].([]any)
	if !ok {
		return nil
	}

	// Filter out scribe entries
	var filtered []any
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			filtered = append(filtered, entry)
			continue
		}

		var filteredHooks []any
		for _, h := range hooksList {
			hookMap, ok := h.(map[string]any)
			if !ok {
				filteredHooks = append(filteredHooks, h)
				continue
			}
			cmd, _ := hookMap["command"].(string)
			if filepath.Base(cmd) != hookFileName {
				filteredHooks = append(filteredHooks, h)
			}
		}

		if len(filteredHooks) > 0 {
			entryMap["hooks"] = filteredHooks
			filtered = append(filtered, entryMap)
		}
	}

	if len(filtered) == 0 {
		delete(hooksMap, hookEvent)
	} else {
		hooksMap[hookEvent] = filtered
	}

	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	}

	return writeSettings(claudeDir, settings)
}

func readSettings(claudeDir string) (map[string]any, error) {
	path := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse settings.json: %w", err)
	}
	return settings, nil
}

func writeSettings(claudeDir string, settings map[string]any) error {
	path := filepath.Join(claudeDir, "settings.json")
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings.json: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save settings.json: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/hooks/ -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hooks/hooks.go internal/hooks/hooks_test.go
git commit -m "feat: add Claude Code hook installer

Installs scribe-hook.sh to ~/.claude/hooks/ and registers it in
settings.json as a PostToolUseFailure handler. Preserves existing
settings. Supports install/uninstall/status operations.

Pattern borrowed from nono-cli's hook bridge architecture."
```

---

## Task 3: `scribe hooks` Command

Cobra command exposing hook management to users and agents.

**Files:**
- Create: `cmd/hooks.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHooksInstallCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .claude dir
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"hooks", "install", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hooks install claude failed: %v", err)
	}
}

func TestHooksStatusCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"hooks", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hooks status failed: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -v -run TestHooks`
Expected: FAIL — command not registered

- [ ] **Step 3: Implement the hooks command**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/hooks"
)

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage agent integration hooks",
	Long:  "Install, uninstall, and check status of hooks that bridge Scribe with AI agents.",
}

var hooksInstallCmd = &cobra.Command{
	Use:       "install <target>",
	Short:     "Install hook for an AI agent",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"claude"},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		switch target {
		case "claude":
			result, err := hooks.InstallClaude()
			if err != nil {
				return err
			}
			switch result.Status {
			case hooks.Installed:
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Hook installed at %s\n", result.ScriptPath)
			case hooks.AlreadyInstalled:
				fmt.Fprintln(cmd.OutOrStdout(), "✓ Hook already installed")
			case hooks.Updated:
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Hook updated at %s\n", result.ScriptPath)
			}
			return nil
		default:
			return fmt.Errorf("unsupported target: %s (available: claude)", target)
		}
	},
}

var hooksUninstallCmd = &cobra.Command{
	Use:       "uninstall <target>",
	Short:     "Remove hook for an AI agent",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"claude"},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		switch target {
		case "claude":
			if err := hooks.UninstallClaude(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Hook removed")
			return nil
		default:
			return fmt.Errorf("unsupported target: %s (available: claude)", target)
		}
	},
}

var hooksStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hook installation status",
	RunE: func(cmd *cobra.Command, args []string) error {
		installed, scriptPath, err := hooks.Status()
		if err != nil {
			return err
		}
		if installed {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Claude Code hook installed at %s\n", scriptPath)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "○ Claude Code hook not installed")
			fmt.Fprintln(cmd.OutOrStdout(), "  Run: scribe hooks install claude")
		}
		return nil
	},
}

func init() {
	hooksCmd.AddCommand(hooksInstallCmd)
	hooksCmd.AddCommand(hooksUninstallCmd)
	hooksCmd.AddCommand(hooksStatusCmd)
}
```

- [ ] **Step 4: Register command in root.go**

In `cmd/root.go`, add `hooksCmd` to the `rootCmd.AddCommand(...)` call in `init()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -v -run TestHooks`
Expected: PASS

- [ ] **Step 6: Manual smoke test**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go run ./cmd/scribe hooks --help`
Expected: Shows install/uninstall/status subcommands

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go run ./cmd/scribe hooks status`
Expected: Shows current hook status

- [ ] **Step 7: Commit**

```bash
git add cmd/hooks.go cmd/root.go
git commit -m "feat: add 'scribe hooks' command

Exposes hook management: install, uninstall, status.
Currently supports Claude Code target."
```

---

## Task 4: Auto-Install Hook on First Sync

When Scribe detects Claude Code is installed and the hook isn't present, automatically install it during `scribe sync`. This is the zero-friction path — users don't need to know about `scribe hooks install`.

**Files:**
- Modify: `internal/workflow/sync.go` — add StepInstallHooks
- Modify: `internal/workflow/bag.go` — add HookResults field
- Create: `internal/workflow/sync_hooks_test.go`

- [ ] **Step 1: Write the failing test**

```go
package workflow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestStepInstallHooks_InstallsWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .claude dir
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	bag := &workflow.Bag{}
	if err := workflow.StepInstallHooks(context.Background(), bag); err != nil {
		t.Fatalf("StepInstallHooks() error: %v", err)
	}

	// Verify hook was installed
	scriptPath := filepath.Join(home, ".claude", "hooks", "scribe-hook.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("hook script not installed: %v", err)
	}
}

func TestStepInstallHooks_SkipsWhenNoClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No .claude dir
	bag := &workflow.Bag{}
	if err := workflow.StepInstallHooks(context.Background(), bag); err != nil {
		t.Fatalf("StepInstallHooks() should not error when Claude missing: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/workflow/ -v -run TestStepInstallHooks`
Expected: FAIL — StepInstallHooks not defined

- [ ] **Step 3: Add HookResults to Bag**

In `internal/workflow/bag.go`, add to the Bag struct:

```go
// Hook installation results (populated by StepInstallHooks)
HookResults []string
```

- [ ] **Step 4: Implement StepInstallHooks**

Add to `internal/workflow/sync.go`:

```go
// StepInstallHooks auto-installs agent hooks when the target tool is
// detected but the hook is missing. Runs silently — hook installation
// failures are non-fatal (logged but do not block sync).
func StepInstallHooks(ctx context.Context, b *Bag) error {
	installed, _, err := hooks.Status()
	if err != nil {
		// Claude Code not installed or can't determine — skip silently
		return nil
	}
	if installed {
		return nil
	}

	result, err := hooks.InstallClaude()
	if err != nil {
		// Non-fatal — don't block sync for a hook failure
		return nil
	}

	if result.Status == hooks.Installed {
		b.HookResults = append(b.HookResults, "claude")
	}
	return nil
}
```

- [ ] **Step 5: Wire StepInstallHooks into SyncSteps**

In `SyncSteps()`, add the hook step after ResolveTools:

```go
func SyncSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"CheckConnected", StepCheckConnected},
		{"FilterRegistries", StepFilterRegistries},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"InstallHooks", StepInstallHooks},
		{"Adopt", StepAdopt},
		{"SyncSkills", StepSyncSkills},
	}
}
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/workflow/ -v -run TestStepInstallHooks`
Expected: PASS

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./... -count=1`
Expected: All tests PASS (no regressions)

- [ ] **Step 7: Commit**

```bash
git add internal/workflow/sync.go internal/workflow/bag.go internal/workflow/sync_hooks_test.go
git commit -m "feat: auto-install Claude Code hook during sync

Silently installs the agent bridge hook on first sync when Claude Code
is detected. Non-fatal — hook failures never block sync."
```

---

## Task 5: Embedded Default Catalog

Compile a default catalog into the binary so `scribe add` can show available skills without network access on first run. Follows nono's `include_str!()` pattern using Go's `embed` package.

**Files:**
- Create: `internal/embed/embed.go`
- Create: `internal/embed/catalog.json`
- Create: `internal/embed/embed_test.go`
- Modify: `internal/provider/provider.go` — add fallback to embedded catalog

- [ ] **Step 1: Define the embedded catalog JSON**

```json
{
  "format_version": 1,
  "updated_at": "2026-04-14T00:00:00Z",
  "registries": [
    {
      "repo": "Naoray/scribe",
      "description": "Official Scribe skill registry",
      "builtin": true,
      "catalog": []
    }
  ]
}
```

This starts empty — populated with actual skills as they're published. The structure allows offline browsing of the registry index.

- [ ] **Step 2: Write the failing test**

```go
package embed_test

import (
	"testing"

	scribeEmbed "github.com/Naoray/scribe/internal/embed"
)

func TestEmbeddedCatalog_Loads(t *testing.T) {
	catalog, err := scribeEmbed.LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error: %v", err)
	}

	if catalog.FormatVersion != 1 {
		t.Errorf("format_version = %d, want 1", catalog.FormatVersion)
	}

	if len(catalog.Registries) == 0 {
		t.Error("expected at least one registry")
	}

	if catalog.Registries[0].Repo != "Naoray/scribe" {
		t.Errorf("first registry repo = %q, want Naoray/scribe", catalog.Registries[0].Repo)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/embed/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 4: Implement the embed package**

```go
package embed

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed catalog.json
var catalogData []byte

// Catalog is the embedded registry index compiled into the binary.
type Catalog struct {
	FormatVersion int               `json:"format_version"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Registries    []CatalogRegistry `json:"registries"`
}

// CatalogRegistry describes a registry and its known skills.
type CatalogRegistry struct {
	Repo        string         `json:"repo"`
	Description string         `json:"description"`
	Builtin     bool           `json:"builtin"`
	Catalog     []CatalogEntry `json:"catalog"`
}

// CatalogEntry is a skill known at build time.
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Ref         string `json:"ref"`
}

// LoadCatalog parses and returns the embedded catalog.
func LoadCatalog() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(catalogData, &c); err != nil {
		return nil, fmt.Errorf("parse embedded catalog: %w", err)
	}
	return &c, nil
}
```

- [ ] **Step 5: Run tests**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/embed/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/embed/embed.go internal/embed/catalog.json internal/embed/embed_test.go
git commit -m "feat: add embedded catalog for zero-network first-run

Compiles a default registry catalog into the binary using go:embed.
Enables offline skill browsing without network access.

Pattern inspired by nono-cli's include_str!() approach."
```

---

## Task 6: Three-State Config Layering

Implement `InheritableValue[T]` semantics for config fields. This enables project-level `.scribe/config.yaml` that can override, clear, or inherit global settings. Follows nono's `Inherit | Clear | Set(T)` pattern.

**Files:**
- Create: `internal/config/layered.go`
- Create: `internal/config/layered_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestMergeConfigs_ProjectOverridesGlobal(t *testing.T) {
	global := &config.Config{
		Editor: "vim",
		Adoption: config.AdoptionConfig{Mode: "auto"},
		Registries: []config.RegistryConfig{
			{Repo: "team/skills", Enabled: true},
		},
	}

	project := &config.Config{
		Editor: "code",
	}

	merged := config.Merge(global, project)

	if merged.Editor != "code" {
		t.Errorf("Editor = %q, want 'code'", merged.Editor)
	}
	// Inherited from global
	if merged.AdoptionMode() != "auto" {
		t.Errorf("AdoptionMode = %q, want 'auto'", merged.AdoptionMode())
	}
	// Registries inherited
	if len(merged.Registries) != 1 {
		t.Errorf("Registries len = %d, want 1", len(merged.Registries))
	}
}

func TestMergeConfigs_ProjectAddsRegistry(t *testing.T) {
	global := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "team/skills", Enabled: true},
		},
	}

	project := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "project/extras", Enabled: true},
		},
	}

	merged := config.Merge(global, project)

	if len(merged.Registries) != 2 {
		t.Fatalf("Registries len = %d, want 2", len(merged.Registries))
	}
}

func TestMergeConfigs_NilProjectReturnsGlobal(t *testing.T) {
	global := &config.Config{Editor: "vim"}

	merged := config.Merge(global, nil)

	if merged.Editor != "vim" {
		t.Errorf("Editor = %q, want 'vim'", merged.Editor)
	}
}

func TestLoadWithProject_FindsProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write global config
	globalDir := filepath.Join(home, ".scribe")
	os.MkdirAll(globalDir, 0o755)
	os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte("editor: vim\n"), 0o644)

	// Write project config
	projectDir := t.TempDir()
	scribeDir := filepath.Join(projectDir, ".scribe")
	os.MkdirAll(scribeDir, 0o755)
	os.WriteFile(filepath.Join(scribeDir, "config.yaml"), []byte("editor: code\n"), 0o644)

	cfg, err := config.LoadWithProject(projectDir)
	if err != nil {
		t.Fatalf("LoadWithProject() error: %v", err)
	}

	if cfg.Editor != "code" {
		t.Errorf("Editor = %q, want 'code' (project override)", cfg.Editor)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/config/ -v -run TestMerge`
Expected: FAIL — Merge function not defined

- [ ] **Step 3: Implement config merging**

```go
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Merge combines a global config with a project-level override.
// Project values take precedence over global. Registries are merged
// (union by repo name). Nil project returns a copy of global.
func Merge(global, project *Config) *Config {
	if project == nil {
		cp := *global
		return &cp
	}

	merged := *global

	// Scalar overrides: project wins if non-zero
	if project.Editor != "" {
		merged.Editor = project.Editor
	}
	if project.Token != "" {
		merged.Token = project.Token
	}
	if project.Adoption.Mode != "" {
		merged.Adoption.Mode = project.Adoption.Mode
	}

	// Adoption paths: append project paths
	if len(project.Adoption.Paths) > 0 {
		merged.Adoption.Paths = append(merged.Adoption.Paths, project.Adoption.Paths...)
	}

	// Registries: merge by repo name (project overrides same-name, appends new)
	if len(project.Registries) > 0 {
		repoIndex := make(map[string]int, len(merged.Registries))
		for i, r := range merged.Registries {
			repoIndex[strings.ToLower(r.Repo)] = i
		}

		for _, pr := range project.Registries {
			key := strings.ToLower(pr.Repo)
			if idx, exists := repoIndex[key]; exists {
				merged.Registries[idx] = pr // project overrides
			} else {
				merged.Registries = append(merged.Registries, pr)
			}
		}
	}

	// Tools: merge by name (same logic as registries)
	if len(project.Tools) > 0 {
		toolIndex := make(map[string]int, len(merged.Tools))
		for i, t := range merged.Tools {
			toolIndex[strings.ToLower(t.Name)] = i
		}

		for _, pt := range project.Tools {
			key := strings.ToLower(pt.Name)
			if idx, exists := toolIndex[key]; exists {
				merged.Tools[idx] = pt
			} else {
				merged.Tools = append(merged.Tools, pt)
			}
		}
	}

	return &merged
}

// LoadWithProject loads global config from ~/.scribe/config.yaml then
// merges with a project-level .scribe/config.yaml if found. Walks up
// from projectDir looking for .scribe/config.yaml.
func LoadWithProject(projectDir string) (*Config, error) {
	global, err := Load()
	if err != nil {
		return nil, err
	}

	projectCfg, err := findProjectConfig(projectDir)
	if err != nil {
		return nil, err
	}

	return Merge(global, projectCfg), nil
}

// findProjectConfig walks up from dir looking for .scribe/config.yaml.
// Returns nil (not error) if no project config found.
func findProjectConfig(dir string) (*Config, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	for {
		candidate := filepath.Join(dir, ".scribe", "config.yaml")
		data, err := os.ReadFile(candidate)
		if err == nil {
			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached root
		}
		dir = parent
	}

	return nil, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/config/ -v -run "TestMerge|TestLoadWithProject"`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./... -count=1`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/layered.go internal/config/layered_test.go
git commit -m "feat: add project-level config layering with merge semantics

Global config (~/.scribe/config.yaml) can now be overridden by
project-level .scribe/config.yaml. Scalars: project wins if set.
Registries/tools: merged by name, project overrides same-name entries.

Three-state semantics inspired by nono-cli's InheritableValue pattern."
```

---

## Task 7: Integration Test — Full Hook Lifecycle

End-to-end test covering install → verify → sync-auto-install → uninstall.

**Files:**
- Create: `internal/hooks/integration_test.go`

- [ ] **Step 1: Write the integration test**

```go
package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/hooks"
)

func TestHookLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Phase 1: Status before install
	installed, _, err := hooks.Status()
	if err != nil {
		t.Fatalf("initial Status() error: %v", err)
	}
	if installed {
		t.Fatal("hook should not be installed initially")
	}

	// Phase 2: Install
	result, err := hooks.InstallClaude()
	if err != nil {
		t.Fatalf("InstallClaude() error: %v", err)
	}
	if result.Status != hooks.Installed {
		t.Errorf("install status = %v, want Installed", result.Status)
	}

	// Phase 3: Verify settings.json structure
	settingsData, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing hooks key")
	}

	events, ok := hooksMap["PostToolUseFailure"].([]any)
	if !ok {
		t.Fatal("settings.json missing PostToolUseFailure")
	}

	if len(events) != 1 {
		t.Errorf("PostToolUseFailure entries = %d, want 1", len(events))
	}

	// Phase 4: Idempotent re-install
	result, err = hooks.InstallClaude()
	if err != nil {
		t.Fatalf("second InstallClaude() error: %v", err)
	}
	if result.Status != hooks.AlreadyInstalled {
		t.Errorf("re-install status = %v, want AlreadyInstalled", result.Status)
	}

	// Phase 5: Verify hook script is executable
	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat hook script: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("hook script not executable")
	}

	// Phase 6: Uninstall
	if err := hooks.UninstallClaude(); err != nil {
		t.Fatalf("UninstallClaude() error: %v", err)
	}

	installed, _, err = hooks.Status()
	if err != nil {
		t.Fatalf("post-uninstall Status() error: %v", err)
	}
	if installed {
		t.Error("hook still installed after uninstall")
	}

	// Phase 7: Verify settings.json cleaned up
	settingsData, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json after uninstall: %v", err)
	}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("parse settings.json after uninstall: %v", err)
	}

	// hooks key should be removed (no entries left)
	if _, hasHooks := settings["hooks"]; hasHooks {
		hooksMap, _ = settings["hooks"].(map[string]any)
		if len(hooksMap) > 0 {
			t.Error("settings.json still has non-empty hooks after uninstall")
		}
	}
}

func TestHookPreservesExistingSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0o755)

	// Pre-existing settings with other hooks
	existing := map[string]any{
		"permissions": map[string]any{"allow": []any{"Read"}},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": ".*",
					"hooks":   []any{map[string]any{"type": "command", "command": "my-other-hook.sh"}},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o644)

	// Install scribe hook
	if _, err := hooks.InstallClaude(); err != nil {
		t.Fatalf("InstallClaude() error: %v", err)
	}

	// Verify existing settings preserved
	settingsData, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var settings map[string]any
	json.Unmarshal(settingsData, &settings)

	// permissions should still be there
	if _, ok := settings["permissions"]; !ok {
		t.Error("existing permissions key was lost")
	}

	// PreToolUse hook should still be there
	hooksMap := settings["hooks"].(map[string]any)
	if _, ok := hooksMap["PreToolUse"]; !ok {
		t.Error("existing PreToolUse hook was lost")
	}

	// PostToolUseFailure should be added
	if _, ok := hooksMap["PostToolUseFailure"]; !ok {
		t.Error("PostToolUseFailure not added")
	}

	// Uninstall — should only remove our hook, preserve others
	hooks.UninstallClaude()

	settingsData, _ = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	json.Unmarshal(settingsData, &settings)

	// PreToolUse should survive
	hooksMap = settings["hooks"].(map[string]any)
	if _, ok := hooksMap["PreToolUse"]; !ok {
		t.Error("PreToolUse hook lost after uninstall")
	}

	// permissions should survive
	if _, ok := settings["permissions"]; !ok {
		t.Error("permissions lost after uninstall")
	}
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/hooks/ -v -run TestHook`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/integration_test.go
git commit -m "test: add hook lifecycle integration tests

Covers install → idempotent re-install → uninstall → cleanup.
Verifies existing settings.json entries are preserved."
```

---

## Summary

| Task | Pattern | Source | Impact |
|------|---------|--------|--------|
| 1-4  | Claude Code hook bridge | nono `hooks.rs` + `nono-hook.sh` | Agents auto-discover Scribe |
| 5    | Embedded catalog | nono `include_str!()` / `embedded.rs` | Zero-network first-run |
| 6    | Three-state config layering | nono `InheritableValue<T>` | Project-level overrides |
| 7    | Integration tests | — | Confidence |

**Execution order matters:** Tasks 1-4 (hook system) are the highest-leverage deliverable and should ship first. Task 5 (embedded catalog) and Task 6 (config layering) are independent and can be parallelized.
