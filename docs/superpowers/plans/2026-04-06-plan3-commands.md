# Command Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure Scribe's CLI commands to match the MVP command taxonomy: top-level commands for daily skill management (`list`, `add`, `remove`, `tools`), registry subcommands for publishing and administration (`registry connect`, `registry add`, `registry migrate`).

**Architecture:** The current `cmd/add.go` handles both "install to machine" and "share to registry" -- this plan splits those into `scribe add` (consumer) and `scribe registry add` (publisher). `scribe connect` and `scribe migrate` move under the `registry` subcommand. A new `scribe remove` command handles skill uninstallation. A new `scribe tools` command manages tool enable/disable. The list command flips to machine-first default. All changes are in the `cmd/` layer; core packages (`internal/`) are not modified except where noted.

**Tech Stack:** Go 1.26, Cobra, Charm (Bubble Tea v2, Huh v2, Lip Gloss v2), existing `internal/` packages.

**Spec:** `docs/superpowers/specs/2026-04-06-mvp-design.md` (Sections 1, 2, 9, 10)

**Depends on:** Plan 1 (tools rename: `internal/targets/` -> `internal/tools/`, `Target` -> `Tool`, `InstalledSkill.Targets` -> `InstalledSkill.Tools`) and Plan 2 (provider, registry connect with type inference). This plan assumes those are complete -- references to `tools.Tool`, `config.ToolConfig`, `config.RegistryEntry`, `provider.Provider`, etc. assume they exist.

---

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Tools cmd | `cmd/tools.go` | Create | `scribe tools`, `scribe tools enable`, `scribe tools disable` |
| Tools cmd | `cmd/tools_test.go` | Create | Unit tests for tools subcommands |
| Remove cmd | `cmd/remove.go` | Create | `scribe remove <name>` with confirmation, uninstall, state cleanup |
| Remove cmd | `cmd/remove_test.go` | Create | Unit tests for remove logic |
| List cmd | `cmd/list.go` | Modify | Machine-first default, `--remote` flag replaces `--local` |
| List TUI | `cmd/list_tui.go` | Modify | Group by registry, author + tools columns, updated detail panel |
| Add cmd | `cmd/add.go` | Rewrite | Consumer-only: install to machine, search registries + skills.sh |
| Add TUI | `cmd/add_tui.go` | Modify | Browse available skills from registries, search integration |
| Registry add | `cmd/registry_add.go` | Create | Publisher action: share local skills to a registry (moved from add.go) |
| Registry add TUI | `cmd/registry_add_tui.go` | Create | Interactive picker for sharing skills (moved from add_tui.go) |
| Registry connect | `cmd/registry_connect.go` | Create | `scribe registry connect` (moved from connect.go) |
| Registry migrate | `cmd/registry_migrate.go` | Create | `scribe registry migrate` (moved from migrate.go) |
| Registry parent | `cmd/registry.go` | Modify | Register new subcommands: connect, add, migrate, enable, disable |
| Connect | `cmd/connect.go` | Delete | Replaced by `cmd/registry_connect.go` |
| Migrate | `cmd/migrate.go` | Delete | Replaced by `cmd/registry_migrate.go` |
| Root | `cmd/root.go` | Modify | New command tree: add removeCmd, toolsCmd; drop connectCmd, migrateCmd |
| README | `README.md` | Modify | Updated command taxonomy and examples |

---

### Task 1: scribe tools command

**Files:**
- Create: `cmd/tools_test.go`
- Create: `cmd/tools.go`

This task assumes Plan 1 has created `config.ToolConfig` with fields `Name string`, `Enabled bool` and that `config.Config` has a `Tools []ToolConfig` field. It also assumes `tools.AllTools()` returns all known tools and `tools.Tool` has a `Detect() bool` method.

- [x] **Step 1: Write the failing test for tools list output**

```go
// cmd/tools_test.go
package cmd

import (
	"bytes"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestFormatToolsList(t *testing.T) {
	tools := []config.ToolConfig{
		{Name: "claude", Enabled: true},
		{Name: "cursor", Enabled: true},
		{Name: "windsurf", Enabled: false},
	}

	var buf bytes.Buffer
	formatToolsList(&buf, tools)

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("claude")) {
		t.Errorf("expected claude in output, got:\n%s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("enabled")) {
		t.Errorf("expected 'enabled' in output, got:\n%s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("disabled")) {
		t.Errorf("expected 'disabled' for windsurf, got:\n%s", out)
	}
}

func TestFormatToolsListJSON(t *testing.T) {
	tools := []config.ToolConfig{
		{Name: "claude", Enabled: true},
		{Name: "cursor", Enabled: false},
	}

	var buf bytes.Buffer
	if err := formatToolsListJSON(&buf, tools); err != nil {
		t.Fatalf("formatToolsListJSON: %v", err)
	}

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(`"name":"claude"`)) && !bytes.Contains(buf.Bytes(), []byte(`"name": "claude"`)) {
		t.Errorf("expected claude JSON entry, got:\n%s", out)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -run TestFormatTools -v
```

Expected: FAIL -- `formatToolsList` and `formatToolsListJSON` not defined.

- [x] **Step 3: Implement the tools command**

```go
// cmd/tools.go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List and manage AI tools",
	Long: `Show detected AI tools and their enabled/disabled status.

Examples:
  scribe tools                # list tools
  scribe tools enable cursor  # enable a tool
  scribe tools disable cursor # disable a tool`,
	Args: cobra.NoArgs,
	RunE: runToolsList,
}

var toolsEnableCmd = &cobra.Command{
	Use:   "enable <tool>",
	Short: "Enable a tool for skill installation",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolsEnable,
}

var toolsDisableCmd = &cobra.Command{
	Use:   "disable <tool>",
	Short: "Disable a tool (skills won't install to it)",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolsDisable,
}

func init() {
	toolsCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	toolsCmd.AddCommand(toolsEnableCmd)
	toolsCmd.AddCommand(toolsDisableCmd)
}

func runToolsList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if useJSON {
		return formatToolsListJSON(os.Stdout, cfg.Tools)
	}

	formatToolsList(os.Stdout, cfg.Tools)
	return nil
}

func runToolsEnable(cmd *cobra.Command, args []string) error {
	return setToolEnabled(args[0], true)
}

func runToolsDisable(cmd *cobra.Command, args []string) error {
	return setToolEnabled(args[0], false)
}

func setToolEnabled(name string, enabled bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	found := false
	for i, t := range cfg.Tools {
		if strings.EqualFold(t.Name, name) {
			cfg.Tools[i].Enabled = enabled
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown tool %q -- known tools: %s", name, knownToolNames(cfg.Tools))
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	verb := "enabled"
	if !enabled {
		verb = "disabled"
	}
	fmt.Printf("%s %s\n", name, verb)
	return nil
}

func knownToolNames(tools []config.ToolConfig) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func formatToolsList(w io.Writer, tools []config.ToolConfig) {
	if len(tools) == 0 {
		fmt.Fprintln(w, "No tools configured. Run `scribe sync` to auto-detect tools.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tSTATUS")
	for _, t := range tools {
		status := "enabled"
		if !t.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(tw, "%s\t%s\n", t.Name, status)
	}
	tw.Flush()
}

func formatToolsListJSON(w io.Writer, tools []config.ToolConfig) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tools)
}
```

- [x] **Step 4: Run the test to verify it passes**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -run TestFormatTools -v
```

Expected: PASS.

- [x] **Step 5: Verify the full build compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [x] **Step 6: Commit**

```bash
git add cmd/tools.go cmd/tools_test.go
git commit -m "[agent] Add scribe tools command with enable/disable subcommands

Step 1 of plan3-commands"
```

---

### Task 2: scribe remove command

**Files:**
- Create: `cmd/remove_test.go`
- Create: `cmd/remove.go`

This task assumes Plan 1 has added `Tool.Uninstall(skillName string) error` to the tool interface and that `InstalledSkill.Tools` (renamed from `Targets`) lists which tools have the skill installed.

- [x] **Step 1: Write the failing test for remove logic**

```go
// cmd/remove_test.go
package cmd

import (
	"testing"
)

func TestResolveRemoveTarget(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		installed  []string // keys in state
		wantKey    string
		wantErr    bool
	}{
		{
			name:      "exact namespaced match",
			input:     "Artistfy-hq/recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			wantKey:   "Artistfy-hq/recap",
		},
		{
			name:      "bare name unique match",
			input:     "deploy",
			installed: []string{"Artistfy-hq/deploy", "Artistfy-hq/recap"},
			wantKey:   "Artistfy-hq/deploy",
		},
		{
			name:      "bare name ambiguous",
			input:     "recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			wantErr:   true,
		},
		{
			name:      "not found",
			input:     "nonexistent",
			installed: []string{"Artistfy-hq/recap"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRemoveTarget(tt.input, tt.installed)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got key=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantKey {
				t.Errorf("got %q, want %q", got, tt.wantKey)
			}
		})
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -run TestResolveRemoveTarget -v
```

Expected: FAIL -- `resolveRemoveTarget` not defined.

- [x] **Step 3: Implement the remove command**

```go
// cmd/remove.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

var removeCmd = &cobra.Command{
	Use:   "remove <skill>",
	Short: "Remove a skill from this machine",
	Long: `Remove a skill from the local machine. Deletes files from the
canonical store, removes symlinks from all tools, and clears state.

If the skill name is ambiguous (exists in multiple registries), pass
the full namespaced name (e.g. Artistfy-hq/recap) or use interactive
mode in a terminal.

Examples:
  scribe remove recap
  scribe remove Artistfy-hq/recap
  scribe remove recap --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	removeCmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func runRemove(cmd *cobra.Command, args []string) error {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())

	st, err := state.Load()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Build list of installed skill keys.
	installedKeys := make([]string, 0, len(st.Installed))
	for k := range st.Installed {
		installedKeys = append(installedKeys, k)
	}

	// Resolve which skill to remove.
	skillKey, err := resolveRemoveTarget(args[0], installedKeys)
	if err != nil {
		// If ambiguous and TTY, show picker.
		if isTTY && strings.Contains(err.Error(), "ambiguous") {
			skillKey, err = pickRemoveTarget(args[0], installedKeys)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	installed := st.Installed[skillKey]

	// Check if managed by a registry.
	managedBy := findManagingRegistries(skillKey, cfg)

	// Confirmation.
	if !yesFlag && isTTY && !useJSON {
		msg := fmt.Sprintf("Remove %q from this machine?", skillKey)
		if len(managedBy) > 0 {
			msg += fmt.Sprintf("\n  (managed by %s -- will re-install on next sync)", strings.Join(managedBy, ", "))
		}
		var confirm bool
		if err := huh.NewConfirm().Title(msg).Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Uninstall from all tools that have this skill.
	var uninstallErrors []string
	skillName := extractSkillName(skillKey)
	allTools := tools.AllTools()
	for _, toolName := range installed.Tools {
		for _, t := range allTools {
			if t.Name() == toolName {
				if err := t.Uninstall(skillName); err != nil {
					uninstallErrors = append(uninstallErrors, fmt.Sprintf("%s: %v", toolName, err))
				}
				break
			}
		}
	}

	// Remove from canonical store.
	storeDir, err := tools.StoreDir()
	if err == nil {
		canonicalDir := filepath.Join(storeDir, skillKey)
		if err := os.RemoveAll(canonicalDir); err != nil && !os.IsNotExist(err) {
			uninstallErrors = append(uninstallErrors, fmt.Sprintf("store cleanup: %v", err))
		}
	}

	// Remove recorded paths that may exist outside store.
	for _, p := range installed.Paths {
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			os.Remove(p)
		}
	}

	// Remove from state.
	st.Remove(skillKey)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"removed":    skillKey,
			"managed_by": managedBy,
			"errors":     uninstallErrors,
		})
	}

	fmt.Printf("Removed %s\n", skillKey)
	if len(managedBy) > 0 {
		fmt.Printf("  Note: managed by %s -- will re-install on next sync\n", strings.Join(managedBy, ", "))
	}
	for _, e := range uninstallErrors {
		fmt.Fprintf(os.Stderr, "  Warning: %s\n", e)
	}

	return nil
}

// resolveRemoveTarget matches user input against installed skill keys.
// Accepts "registry-slug/name" for exact match or bare "name" for unique match.
func resolveRemoveTarget(input string, installedKeys []string) (string, error) {
	// Exact match first (case-insensitive).
	for _, k := range installedKeys {
		if strings.EqualFold(k, input) {
			return k, nil
		}
	}

	// Bare name match: find all keys ending with /input.
	var matches []string
	for _, k := range installedKeys {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], input) {
			matches = append(matches, k)
		}
		// Also match keys without slash (legacy unnamespaced).
		if !strings.Contains(k, "/") && strings.EqualFold(k, input) {
			matches = append(matches, k)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("skill %q is not installed", input)
	default:
		return "", fmt.Errorf("ambiguous skill %q -- matches: %s\nSpecify the full name, e.g.: scribe remove %s",
			input, strings.Join(matches, ", "), matches[0])
	}
}

// pickRemoveTarget shows an interactive picker for ambiguous skill names.
func pickRemoveTarget(input string, installedKeys []string) (string, error) {
	var matches []string
	for _, k := range installedKeys {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], input) {
			matches = append(matches, k)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("skill %q is not installed", input)
	}

	opts := make([]huh.Option[string], len(matches))
	for i, m := range matches {
		opts[i] = huh.NewOption(m, m)
	}

	var selected string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Multiple skills named %q -- which one?", input)).
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}

// findManagingRegistries checks if the skill is still in any connected registry.
func findManagingRegistries(skillKey string, cfg *config.Config) []string {
	var managing []string
	// The skill key format is "registry-slug/name". If the registry-slug
	// matches a connected registry (after slugifying), the skill is managed.
	parts := strings.SplitN(skillKey, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	slug := parts[0]
	for _, reg := range cfg.Registries {
		regSlug := strings.ReplaceAll(reg.Repo, "/", "-")
		if strings.EqualFold(regSlug, slug) {
			managing = append(managing, reg.Repo)
		}
	}
	return managing
}

// extractSkillName returns the bare skill name from a "registry/name" key.
func extractSkillName(key string) string {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return key
}
```

- [x] **Step 4: Run the test to verify it passes**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ -run TestResolveRemoveTarget -v
```

Expected: PASS.

- [x] **Step 5: Verify the full build compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [x] **Step 6: Commit**

```bash
git add cmd/remove.go cmd/remove_test.go
git commit -m "[agent] Add scribe remove command with namespaced resolution

Step 2 of plan3-commands"
```

---

### Task 3: Rework scribe list -- machine-first default

**Files:**
- Modify: `cmd/list.go`
- Modify: `internal/workflow/list.go`

The list command currently defaults to showing remote registry diff. This task flips it to machine-first: `scribe list` shows everything on disk grouped by registry, and `--remote` flag restores the old behavior.

- [x] **Step 1: Update the list command flags**

Replace the flag setup and `runList` function in `cmd/list.go`:

```go
// cmd/list.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/workflow"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all skills on this machine",
	Long: `Show all skills installed on this machine, grouped by source registry.

Default view shows installed skills with name, version, author, status, and tools.
Use --remote to see skills available in connected registries that are not installed.

Examples:
  scribe list                   # show all installed skills
  scribe list --remote          # show available but not installed
  scribe list --registry org/r  # filter to one registry
  scribe list --json            # machine-readable output`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	listCmd.Flags().Bool("remote", false, "Show available skills from registries (not installed)")
	listCmd.Flags().String("registry", "", "Show only this registry (owner/repo or repo name)")
	listCmd.Flags().String("group", "", "Jump directly to a group (e.g. registry slug)")
}

func runList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	remoteFlag, _ := cmd.Flags().GetBool("remote")
	repoFlag, _ := cmd.Flags().GetString("registry")
	groupFlag, _ := cmd.Flags().GetString("group")

	isTTY := isatty.IsTerminal(os.Stdout.Fd())

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		RemoteFlag:       remoteFlag,
		RepoFlag:         repoFlag,
		FilterRegistries: filterRegistries,
	}

	// Wire up TUI for local list when running in a terminal.
	if isTTY && !jsonFlag {
		bag.ListTUI = func(skills []discovery.Skill) error {
			m := newListModel(skills, groupFlag, bag.State)
			p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
			_, err := p.Run()
			if errors.Is(err, tea.ErrInterrupted) {
				os.Exit(130)
			}
			if err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		}
	}

	return workflow.Run(cmd.Context(), workflow.ListSteps(), bag)
}
```

- [x] **Step 2: Add RemoteFlag to the workflow Bag**

In `internal/workflow/bag.go`, add the `RemoteFlag` field alongside the existing `LocalFlag`:

```go
// Add after the LocalFlag field:
RemoteFlag bool   // --remote: show available skills from registries
```

- [x] **Step 3: Update StepBranchLocalOrRemote to flip the default**

In `internal/workflow/list.go`, update `StepBranchLocalOrRemote` so that the default (no flags) shows local, and `--remote` shows the registry diff:

```go
// StepBranchLocalOrRemote handles both the local-only view and the
// remote diff view, keeping the workflow linear.
func StepBranchLocalOrRemote(ctx context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	// Remote view: explicit --remote flag with registries connected.
	if b.RemoteFlag && len(b.Config.TeamRepos) > 0 {
		// Reuse shared steps for migration and filtering.
		if err := StepMigrateRegistries(ctx, b); err != nil {
			return err
		}
		if err := StepFilterRegistries(ctx, b); err != nil {
			return err
		}

		syncer := &sync.Syncer{Client: sync.WrapGitHubClient(b.Client), Targets: []targets.Target{}}
		multiRegistry := len(b.Repos) > 1

		if useJSON {
			return printMultiListJSON(ctx, w, b.Repos, syncer, b.State)
		}
		return printMultiListTable(ctx, w, b.Repos, syncer, b.State, multiRegistry)
	}

	// Default: machine-first local view.
	return listLocal(w, b.State, useJSON, b.ListTUI)
}
```

- [x] **Step 4: Run the full test suite to check nothing is broken**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./cmd/ ./internal/workflow/ -v -count=1
```

Expected: PASS.

- [x] **Step 5: Verify the build compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [x] **Step 6: Commit**

```bash
git add cmd/list.go internal/workflow/bag.go internal/workflow/list.go
git commit -m "[agent] Flip list default to machine-first, add --remote flag

Step 3 of plan3-commands"
```

---

### Task 4: Update list TUI -- group by registry

**Files:**
- Modify: `cmd/list_tui.go`

The current TUI groups by package name. This task changes grouping to registry slug, adds author and tools columns to the skill list, and updates the detail panel to show registry, author, version, status, tools, and paths.

- [x] **Step 1: Update the group building logic in newListModel**

Replace the `newListModel` function in `cmd/list_tui.go` to group by registry instead of package:

```go
func newListModel(skills []discovery.Skill, groupFlag string, st *state.State) listModel {
	// Build group list by registry source.
	counts := map[string]int{}
	for _, sk := range skills {
		g := skillRegistryGroup(sk, st)
		counts[g]++
	}

	groups := []listGroupItem{
		{name: "all", key: "", count: len(skills)},
	}

	// "Local (unmanaged)" first, then registries alphabetically.
	if n, ok := counts["Local (unmanaged)"]; ok {
		groups = append(groups, listGroupItem{name: "Local (unmanaged)", key: "Local (unmanaged)", count: n})
	}
	var registries []string
	for k := range counts {
		if k != "Local (unmanaged)" {
			registries = append(registries, k)
		}
	}
	sort.Strings(registries)
	for _, k := range registries {
		groups = append(groups, listGroupItem{name: k, key: k, count: counts[k]})
	}

	m := listModel{
		phase:  listPhaseGroups,
		groups: groups,
		skills: skills,
		state:  st,
	}

	// If --group flag is set, skip to skills phase.
	if groupFlag != "" {
		m.groupKey = groupFlag
		m.filtered = m.filterSkills()
		m.phase = listPhaseSkills
	}

	return m
}

// skillRegistryGroup returns the display group name for a skill.
// Uses state registries to determine the source registry.
func skillRegistryGroup(sk discovery.Skill, st *state.State) string {
	if installed, ok := st.Installed[sk.Name]; ok {
		if len(installed.Registries) > 0 {
			return installed.Registries[0]
		}
	}
	if sk.Source != "" {
		return sk.Source
	}
	return "Local (unmanaged)"
}
```

- [x] **Step 2: Update formatSkillLine to show author and tools**

Replace `formatSkillLine` in `cmd/list_tui.go`:

```go
func (m listModel) formatSkillLine(sk discovery.Skill, isCursor bool, maxWidth int) string {
	prefix := "  "
	nameStyle := ltNameStyle
	if isCursor {
		prefix = ltCursorStyle.Render("^") + " "
		nameStyle = ltCursorStyle
	}

	// Build columns: name, version, author, tools.
	name := sk.Name
	ver := sk.Version
	if ver == "" {
		ver = "-"
	}

	// Author from state if available.
	author := ""
	if installed, ok := m.state.Installed[sk.Name]; ok {
		author = installed.Author
	}
	if author == "" {
		author = "-"
	}

	// Truncate name to fit.
	nameWidth := maxWidth / 3
	if nameWidth > 25 {
		nameWidth = 25
	}
	name = runewidth.Truncate(name, nameWidth, "...")

	verWidth := 12
	ver = runewidth.Truncate(ver, verWidth, "...")

	authorWidth := 10
	author = runewidth.Truncate(author, authorWidth, "...")

	line := fmt.Sprintf("%s%-*s  %-*s  %-*s",
		prefix,
		nameWidth, nameStyle.Render(name),
		verWidth, ltDimStyle.Render(ver),
		authorWidth, ltDimStyle.Render(author),
	)

	return line
}
```

- [x] **Step 3: Update renderDetail to show registry, author, tools**

Replace `renderDetail` in `cmd/list_tui.go`:

```go
func (m listModel) renderDetail(sk discovery.Skill, width int) string {
	var b strings.Builder

	b.WriteString(ltCursorStyle.Render(sk.Name) + "\n")

	if sk.Description != "" {
		descStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("#aaaaaa"))
		b.WriteString(descStyle.Render(sk.Description) + "\n")
	}

	b.WriteString(ltDivStyle.Render(strings.Repeat("-", width-2)) + "\n")

	type kv struct{ key, val string }
	var pairs []kv

	// Registry.
	if installed, ok := m.state.Installed[sk.Name]; ok {
		if len(installed.Registries) > 0 {
			pairs = append(pairs, kv{"Registry", strings.Join(installed.Registries, ", ")})
		}
		if installed.Author != "" {
			pairs = append(pairs, kv{"Author", installed.Author})
		}
	}

	if sk.Version != "" {
		pairs = append(pairs, kv{"Version", sk.Version})
	}
	if sk.ContentHash != "" {
		pairs = append(pairs, kv{"Hash", sk.ContentHash})
	}
	if sk.Source != "" {
		pairs = append(pairs, kv{"Source", sk.Source})
	}

	// Tools.
	if installed, ok := m.state.Installed[sk.Name]; ok {
		toolNames := installed.Tools
		if len(toolNames) == 0 {
			toolNames = installed.Targets // backward compat during migration
		}
		if len(toolNames) > 0 {
			pairs = append(pairs, kv{"Tools", strings.Join(toolNames, ", ")})
		}
	} else if len(sk.Targets) > 0 {
		pairs = append(pairs, kv{"Tools", strings.Join(sk.Targets, ", ")})
	}

	if sk.LocalPath != "" {
		path := sk.LocalPath
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
			path = "~" + strings.TrimPrefix(path, home)
		}
		pairs = append(pairs, kv{"Path", path})
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(10)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))

	for _, p := range pairs {
		b.WriteString(keyStyle.Render(p.key) + valStyle.Render(p.val) + "\n")
	}

	return b.String()
}
```

- [x] **Step 4: Update filterSkills to group by registry instead of package**

Replace `filterSkills` in `cmd/list_tui.go`:

```go
func (m listModel) filterSkills() []discovery.Skill {
	var result []discovery.Skill
	lower := strings.ToLower(m.search)

	for _, sk := range m.skills {
		// Group filter -- now matches registry group.
		if m.groupKey != "" {
			g := skillRegistryGroup(sk, m.state)
			if g != m.groupKey {
				continue
			}
		}
		// Search filter.
		if m.search != "" {
			if !strings.Contains(strings.ToLower(sk.Name), lower) &&
				!strings.Contains(strings.ToLower(sk.Description), lower) {
				continue
			}
		}
		result = append(result, sk)
	}

	if m.groupKey == "" && m.search == "" {
		return m.skills
	}
	return result
}
```

- [x] **Step 5: Verify the build compiles and existing tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./cmd/ -v -count=1
```

Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add cmd/list_tui.go
git commit -m "[agent] Update list TUI: group by registry, show author and tools

Step 4 of plan3-commands"
```

---

### Task 5: Move share/publish to scribe registry add

**Files:**
- Create: `cmd/registry_add.go`
- Create: `cmd/registry_add_tui.go`
- Modify: `cmd/registry.go`

This task extracts the "share skill to registry" flow from the current `cmd/add.go` into `scribe registry add`. The code is largely moved, not rewritten.

- [x] **Step 1: Create registry_add.go with the publisher flow**

Move `runAdd`, `runAddByName`, `runAddInteractive`, `sortCandidates`, `resolveTargetRegistry`, `filterAlreadyInTarget`, `wireAddEmit`, `finishAdd`, `autoSync`, and `fetchRegistryManifest` from `cmd/add.go` into `cmd/registry_add.go`. Update the command definition:

```go
// cmd/registry_add.go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

type registryAddResult struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Source   string `json:"source,omitempty"`
	Uploaded bool   `json:"uploaded"`
	Error    string `json:"error,omitempty"`
}

var registryAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Share a skill to a team registry",
	Long: `Share a local skill to a team registry on GitHub.

If the skill has a known source (synced from another registry), adds a
source reference. If it's a local-only skill, uploads the files to the
registry.

With no arguments in a terminal, shows an interactive browser to select
skills. In non-TTY mode, the skill name is required.

Examples:
  scribe registry add cleanup
  scribe registry add gstack --registry ArtistfyHQ/team-skills
  scribe registry add --yes cleanup`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryAdd,
}

func init() {
	registryAddCmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	registryAddCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	registryAddCmd.Flags().String("registry", "", "Target registry (owner/repo)")
}
```

Then copy the full body of `runAdd` (renamed to `runRegistryAdd`), along with all helper functions (`runAddByName` renamed to `runRegistryAddByName`, `runAddInteractive` renamed to `runRegistryAddInteractive`, etc.) from the current `cmd/add.go`. The function signatures stay the same -- only the names change to avoid collisions with the new consumer `add` command.

The following functions move verbatim (with renamed entry points):
- `runRegistryAdd` (was `runAdd`)
- `runRegistryAddByName` (was `runAddByName`)
- `runRegistryAddInteractive` (was `runAddInteractive`)
- `sortCandidates`
- `resolveTargetRegistry`
- `filterAlreadyInTarget`
- `wireRegistryAddEmit` (was `wireAddEmit`, uses `registryAddResult`)
- `finishRegistryAdd` (was `finishAdd`)
- `autoSync`
- `fetchRegistryManifest`

- [x] **Step 2: Create registry_add_tui.go**

Move the entire content of `cmd/add_tui.go` into `cmd/registry_add_tui.go`. Rename the model type from `addModel` to `registryAddModel` and the constructor from `newAddModel` to `newRegistryAddModel`. Update references in `runRegistryAddInteractive` accordingly.

The `registryAddModel` type, `registryAddItem`, and all methods (`Init`, `Update`, `View`, `filteredItems`, `selectedCount`, `selectedCandidates`, `ensureCursorVisible`, `maxContentLines`, `skillGroup`) are moved verbatim with the type rename.

- [x] **Step 3: Register registry add subcommand**

In `cmd/registry.go`, add the subcommand registration in `init()`:

```go
func init() {
	registryCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
}
```

- [x] **Step 4: Verify the build compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [x] **Step 5: Run existing tests to check nothing broke**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./... -count=1
```

Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add cmd/registry_add.go cmd/registry_add_tui.go cmd/registry.go
git commit -m "[agent] Move share-to-registry flow to scribe registry add

Step 5 of plan3-commands"
```

---

### Task 6: Rewrite scribe add -- consumer install command

**Files:**
- Rewrite: `cmd/add.go`
- Rewrite: `cmd/add_tui.go`

The consumer `scribe add` becomes the "install to machine" command. It searches connected registries and optionally skills.sh, then installs selected skills.

- [x] **Step 1: Rewrite cmd/add.go for consumer install**

```go
// cmd/add.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// directInstallPattern matches owner/repo:skillname format.
var directInstallPattern = regexp.MustCompile(`^\w[\w.-]*/[\w.-]+:\S+$`)

var addCmd = &cobra.Command{
	Use:   "add [query or owner/repo:skill]",
	Short: "Find and install skills from registries",
	Long: `Browse and install skills from connected registries.

With no arguments in a terminal, shows an interactive TUI to browse
all available skills from connected registries.

With a search query, searches connected registries and optionally
skills.sh for matching skills.

With owner/repo:skill format, installs directly from that registry
(auto-connects if needed).

Examples:
  scribe add                          # interactive TUI
  scribe add react                    # search "react"
  scribe add antfu/skills:nuxt        # direct install
  scribe add antfu/skills:nuxt --yes  # non-interactive
  scribe add react --json             # search results as JSON`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConsumerAdd,
}

func init() {
	addCmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	addCmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

// addSearchResult represents a skill available for installation.
type addSearchResult struct {
	Name        string `json:"name"`
	Registry    string `json:"registry"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source,omitempty"`
	Installed   bool   `json:"installed"`
}

func runConsumerAdd(cmd *cobra.Command, args []string) error {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	client := gh.NewClient(cmd.Context(), cfg.Token)

	// No args + non-TTY = error.
	if len(args) == 0 && !isTTY {
		return fmt.Errorf("skill name or search query required when not running interactively")
	}

	// Direct install: owner/repo:skill format.
	if len(args) == 1 && directInstallPattern.MatchString(args[0]) {
		return runDirectInstall(cmd, args[0], cfg, st, client, useJSON, isTTY, yesFlag)
	}

	// Search mode (with query) or browse mode (no query, TTY).
	query := ""
	if len(args) == 1 {
		query = args[0]
	}

	results, err := searchRegistries(cmd, query, cfg, st, client)
	if err != nil {
		return err
	}

	// Search skills.sh as a fallback if we have a query and npx is available.
	if query != "" {
		skillsShResults := searchSkillsSh(query)
		results = mergeSearchResults(results, skillsShResults)
	}

	if useJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"results": results})
	}

	if len(results) == 0 {
		if query != "" {
			fmt.Printf("No skills matching %q found in connected registries.\n", query)
		} else {
			fmt.Println("No skills available. Connect a registry first: scribe registry connect <owner/repo>")
		}
		return nil
	}

	if !isTTY {
		// Non-TTY, non-JSON: print a simple list.
		for _, r := range results {
			status := ""
			if r.Installed {
				status = " (installed)"
			}
			fmt.Printf("  %s/%s  %s%s\n", r.Registry, r.Name, r.Version, status)
		}
		return nil
	}

	// Interactive: pick and install.
	return runConsumerAddInteractive(cmd, results, cfg, st, client, yesFlag)
}

func runDirectInstall(cmd *cobra.Command, ref string, cfg *config.Config, st *state.State, client *gh.Client, useJSON, isTTY, yesFlag bool) error {
	// Parse owner/repo:skill.
	colonIdx := strings.LastIndex(ref, ":")
	repoSlug := ref[:colonIdx]
	skillName := ref[colonIdx+1:]

	// Auto-connect if not connected.
	connected := false
	for _, reg := range cfg.Registries {
		if strings.EqualFold(reg.Repo, repoSlug) {
			connected = true
			break
		}
	}
	if !connected {
		if !useJSON {
			fmt.Printf("Auto-connecting to %s...\n", repoSlug)
		}
		// Add to config as a new registry.
		cfg.Registries = append(cfg.Registries, config.RegistryEntry{
			Repo:    repoSlug,
			Enabled: true,
		})
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	// Sync just this registry to install the skill.
	tgts := tools.EnabledTools(cfg)
	syncer := &sync.Syncer{
		Client:  sync.WrapGitHubClient(client),
		Targets: tgts,
		Emit: func(msg any) {
			if useJSON {
				return
			}
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				fmt.Printf("  Installed %s %s\n", m.Name, m.Version)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  Error: %s: %v\n", m.Name, m.Err)
			}
		},
	}

	if err := syncer.Run(cmd.Context(), repoSlug, st); err != nil {
		return err
	}

	// Check if the specific skill was installed.
	if _, ok := st.Installed[skillName]; !ok {
		return fmt.Errorf("skill %q not found in %s", skillName, repoSlug)
	}

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"installed": skillName,
			"registry":  repoSlug,
		})
	}
	return nil
}

// searchRegistries searches connected registries for available skills.
func searchRegistries(cmd *cobra.Command, query string, cfg *config.Config, st *state.State, client *gh.Client) ([]addSearchResult, error) {
	var results []addSearchResult

	for _, reg := range cfg.Registries {
		if !reg.Enabled {
			continue
		}

		owner, repo, err := parseOwnerRepo(reg.Repo)
		if err != nil {
			continue
		}

		m, err := fetchRegistryManifest(cmd.Context(), client, owner, repo)
		if err != nil {
			continue // skip unreachable registries
		}

		for _, entry := range m.Catalog {
			// Filter by query if provided.
			if query != "" {
				lower := strings.ToLower(query)
				if !strings.Contains(strings.ToLower(entry.Name), lower) &&
					!strings.Contains(strings.ToLower(entry.Description), lower) {
					continue
				}
			}

			_, installed := st.Installed[entry.Name]
			results = append(results, addSearchResult{
				Name:        entry.Name,
				Registry:    reg.Repo,
				Author:      entry.Author,
				Description: entry.Description,
				Version:     sourceVersion(entry.Source),
				Source:      entry.Source,
				Installed:   installed,
			})
		}
	}

	return results, nil
}

// parseOwnerRepo splits "owner/repo" into parts.
func parseOwnerRepo(slug string) (string, string, error) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo: %s", slug)
	}
	return parts[0], parts[1], nil
}

// sourceVersion extracts the version/ref from a source string like "github:owner/repo@v1.0.0".
func sourceVersion(source string) string {
	if idx := strings.LastIndex(source, "@"); idx >= 0 {
		return source[idx+1:]
	}
	return ""
}

// searchSkillsSh shells out to `npx skills find` for broader discovery.
// Returns empty slice if npx is not available.
func searchSkillsSh(query string) []addSearchResult {
	_, err := exec.LookPath("npx")
	if err != nil {
		return nil
	}

	out, err := exec.Command("npx", "skills", "find", query).Output()
	if err != nil {
		return nil
	}

	return parseSkillsShOutput(string(out))
}

// parseSkillsShOutput parses `npx skills find` output.
// Expected format: lines like "owner/repo@skill-name  description  N installs"
func parseSkillsShOutput(output string) []addSearchResult {
	var results []addSearchResult
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Try to parse "owner/repo@skill-name" at the start.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		ref := fields[0]
		atIdx := strings.Index(ref, "@")
		if atIdx < 0 {
			continue
		}
		repoSlug := ref[:atIdx]
		skillName := ref[atIdx+1:]

		desc := ""
		if len(fields) > 1 {
			desc = strings.Join(fields[1:], " ")
		}

		results = append(results, addSearchResult{
			Name:        skillName,
			Registry:    repoSlug,
			Description: desc,
			Source:      "skills.sh",
		})
	}
	return results
}

// mergeSearchResults combines registry and skills.sh results, deduplicating by name+registry.
func mergeSearchResults(registry, skillsSh []addSearchResult) []addSearchResult {
	seen := map[string]bool{}
	for _, r := range registry {
		seen[r.Registry+"/"+r.Name] = true
	}
	merged := append([]addSearchResult{}, registry...)
	for _, r := range skillsSh {
		key := r.Registry + "/" + r.Name
		if !seen[key] {
			merged = append(merged, r)
		}
	}
	return merged
}

func runConsumerAddInteractive(cmd *cobra.Command, results []addSearchResult, cfg *config.Config, st *state.State, client *gh.Client, yesFlag bool) error {
	// Filter out already-installed skills.
	var available []addSearchResult
	for _, r := range results {
		if !r.Installed {
			available = append(available, r)
		}
	}

	if len(available) == 0 {
		fmt.Println("All available skills are already installed.")
		return nil
	}

	// Use huh select for now; a full TUI can be added later.
	opts := make([]huh.Option[int], len(available))
	for i, r := range available {
		label := fmt.Sprintf("%s (%s)", r.Name, r.Registry)
		if r.Description != "" {
			label += " - " + r.Description
		}
		opts[i] = huh.NewOption(label, i)
	}

	var selected int
	if err := huh.NewSelect[int]().
		Title("Select a skill to install").
		Options(opts...).
		Value(&selected).
		Run(); err != nil {
		return err
	}

	choice := available[selected]

	// Confirmation.
	if !yesFlag {
		var confirm bool
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Install %q from %s?", choice.Name, choice.Registry)).
			Value(&confirm).
			Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Install via sync.
	fmt.Printf("Installing %s from %s...\n", choice.Name, choice.Registry)

	// Auto-connect if needed (skills.sh result).
	connected := false
	for _, reg := range cfg.Registries {
		if strings.EqualFold(reg.Repo, choice.Registry) {
			connected = true
			break
		}
	}
	if !connected {
		cfg.Registries = append(cfg.Registries, config.RegistryEntry{
			Repo:    choice.Registry,
			Enabled: true,
		})
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("  Auto-connected to %s\n", choice.Registry)
	}

	tgts := tools.EnabledTools(cfg)
	syncer := &sync.Syncer{
		Client:  sync.WrapGitHubClient(client),
		Targets: tgts,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				verb := "Installed"
				if m.Updated {
					verb = "Updated"
				}
				fmt.Printf("  %s %s %s\n", verb, m.Name, m.Version)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  Error: %s: %v\n", m.Name, m.Err)
			}
		},
	}

	return syncer.Run(cmd.Context(), choice.Registry, st)
}
```

- [x] **Step 2: Simplify add_tui.go**

The current `add_tui.go` was for the publisher flow and has been moved to `registry_add_tui.go` in Task 5. Replace the file with a minimal placeholder that the consumer add can grow into:

```go
// cmd/add_tui.go
package cmd

// Consumer add TUI is handled inline in runConsumerAddInteractive
// using huh.NewSelect for the initial implementation.
// A full Bubble Tea TUI can be added as a follow-up.
```

- [x] **Step 3: Verify the build compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [x] **Step 4: Run all tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./... -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add cmd/add.go cmd/add_tui.go
git commit -m "[agent] Rewrite scribe add as consumer install command

Step 6 of plan3-commands"
```

---

### Task 7: Move connect to scribe registry connect

**Files:**
- Create: `cmd/registry_connect.go`
- Modify: `cmd/registry.go`
- Delete: `cmd/connect.go`

- [x] **Step 1: Create registry_connect.go**

Copy `cmd/connect.go` into `cmd/registry_connect.go` with the command renamed:

```go
// cmd/registry_connect.go
package cmd

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/workflow"
)

var registryConnectCmd = &cobra.Command{
	Use:   "connect [owner/repo]",
	Short: "Connect to a skill registry",
	Long: `Connect to a skill registry so Scribe can sync skills from it.

Examples:
  scribe registry connect ArtistfyHQ/team-skills
  scribe registry connect                          # interactive prompt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryConnect,
}

func runRegistryConnect(cmd *cobra.Command, args []string) error {
	repo, err := resolveRepo(args)
	if err != nil {
		return err
	}

	bag := &workflow.Bag{
		RepoArg: repo,
	}
	return workflow.Run(cmd.Context(), workflow.ConnectSteps(), bag)
}

// resolveRepo returns the owner/repo string from args or an interactive prompt.
func resolveRepo(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("no repo specified -- usage: scribe registry connect <owner/repo>")
	}

	var repo string
	err := huh.NewInput().
		Title("Skill registry repo").
		Placeholder("owner/repo").
		Validate(func(s string) error {
			_, _, err := manifest.ParseOwnerRepo(s)
			return err
		}).
		Value(&repo).
		Run()
	if err != nil {
		return "", err
	}
	return repo, nil
}
```

- [x] **Step 2: Register connect subcommand in registry.go**

In `cmd/registry.go`, add to `init()`:

```go
registryCmd.AddCommand(registryConnectCmd)
```

- [x] **Step 3: Delete cmd/connect.go**

```bash
rm /Users/krishankonig/Workspace/bets/scribe/cmd/connect.go
```

- [x] **Step 4: Update guide.go references**

In `cmd/guide.go`, the `runGuideInteractive` function calls `resolveRepo(nil)`. This function has been moved to `cmd/registry_connect.go` so it remains in the same package -- no import changes needed. Verify the guide's connect flow still references the correct function.

Check that `runGuideInteractive` uses `resolveRepo` (which now lives in `registry_connect.go` but is still in `package cmd`) and that the help text in the guide refers to `scribe registry connect` instead of `scribe connect`.

Update the guide JSON steps and display text:

In `cmd/guide.go`, update the step command string:

```go
// In runGuideJSON, change:
steps = append(steps, guideStep{
    Command:     "scribe registry connect <owner/repo>",
    Description: "Connect to your team's skill registry",
})
```

And update `displayGuideSummary` if it references `scribe connect`.

- [x] **Step 5: Verify build compiles and tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./... -count=1
```

- [x] **Step 6: Commit**

```bash
git add cmd/registry_connect.go cmd/registry.go cmd/guide.go
git rm cmd/connect.go
git commit -m "[agent] Move connect to scribe registry connect

Step 7 of plan3-commands"
```

---

### Task 8: Move migrate to scribe registry migrate

**Files:**
- Create: `cmd/registry_migrate.go`
- Modify: `cmd/registry.go`
- Delete: `cmd/migrate.go`

- [x] **Step 1: Create registry_migrate.go**

Copy the entire content of `cmd/migrate.go` into `cmd/registry_migrate.go` with the command variable renamed:

```go
// cmd/registry_migrate.go
package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
)

var registryMigrateCmd = &cobra.Command{
	Use:   "migrate [owner/repo]",
	Short: "Convert a scribe.toml registry to scribe.yaml",
	Long: `Fetches the existing scribe.toml from a registry, converts it to the
new scribe.yaml format, and pushes the change as a single commit
(deleting scribe.toml and creating scribe.yaml).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryMigrate,
}

func runRegistryMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var repo string
	if len(args) > 0 {
		repo = args[0]
	} else if len(cfg.TeamRepos) == 1 {
		repo = cfg.TeamRepos[0]
	} else {
		return fmt.Errorf("specify a registry: scribe registry migrate owner/repo")
	}

	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	client := gh.NewClient(ctx, cfg.Token)
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required -- run `gh auth login` or set GITHUB_TOKEN")
	}

	// Check if scribe.yaml already exists.
	exists, err := client.FileExists(ctx, owner, repoName, manifest.ManifestFilename, "HEAD")
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s/%s already has a %s -- nothing to migrate", owner, repoName, manifest.ManifestFilename)
	}

	// Fetch and convert.
	raw, err := client.FetchFile(ctx, owner, repoName, manifest.LegacyManifestFilename, "HEAD")
	if err != nil {
		return fmt.Errorf("fetch scribe.toml: %w", err)
	}

	converted, err := migrate.Convert(raw)
	if err != nil {
		return err
	}

	encoded, err := converted.Encode()
	if err != nil {
		return err
	}

	// Show preview.
	fmt.Printf("Converted %s/%s:\n\n%s\n", owner, repoName, string(encoded))

	if isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Print("Push this change? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Push: delete scribe.toml, create scribe.yaml.
	files := map[string]string{
		manifest.ManifestFilename:       string(encoded),
		manifest.LegacyManifestFilename: "", // empty string = delete
	}
	err = client.PushFiles(ctx, owner, repoName, files, "migrate: scribe.toml -> scribe.yaml")
	if err != nil {
		return fmt.Errorf("push migration: %w", err)
	}

	fmt.Printf("Migrated %s/%s to %s\n", owner, repoName, manifest.ManifestFilename)
	return nil
}
```

- [x] **Step 2: Register migrate subcommand in registry.go**

In `cmd/registry.go`, add to `init()`:

```go
registryCmd.AddCommand(registryMigrateCmd)
```

- [x] **Step 3: Delete cmd/migrate.go**

```bash
rm /Users/krishankonig/Workspace/bets/scribe/cmd/migrate.go
```

- [x] **Step 4: Verify build compiles and tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./... -count=1
```

- [x] **Step 5: Commit**

```bash
git add cmd/registry_migrate.go cmd/registry.go
git rm cmd/migrate.go
git commit -m "[agent] Move migrate to scribe registry migrate

Step 8 of plan3-commands"
```

---

### Task 9: Update root.go -- new command tree

**Files:**
- Modify: `cmd/root.go`
- Modify: `cmd/registry.go`

- [x] **Step 1: Update root.go with the new command tree**

```go
// cmd/root.go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "scribe",
	Short:   "Team skill sync for AI coding agents",
	Long:    "Scribe syncs AI coding agent skills across your team via a shared GitHub registry.",
	Version: Version,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Top-level: daily skill management.
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(guideCmd)

	// Registry subcommand: administration & publishing.
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(createCmd)
}
```

- [x] **Step 2: Update registry.go with all subcommands**

Ensure `cmd/registry.go` has all subcommands registered in `init()`:

```go
func init() {
	registryCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryConnectCmd)
	registryCmd.AddCommand(registryMigrateCmd)
}
```

- [x] **Step 3: Verify build compiles, run all tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./... -count=1
```

- [x] **Step 4: Verify command tree looks correct**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go run ./cmd/scribe --help
cd /Users/krishankonig/Workspace/bets/scribe && go run ./cmd/scribe registry --help
cd /Users/krishankonig/Workspace/bets/scribe && go run ./cmd/scribe tools --help
```

Expected output should show:
- Top-level: `list`, `add`, `remove`, `sync`, `tools`, `guide`, `registry`, `create`
- `registry`: `connect`, `create`, `add`, `list`, `migrate`
- `tools`: `enable`, `disable`

- [x] **Step 5: Commit**

```bash
git add cmd/root.go cmd/registry.go
git commit -m "[agent] Wire new command tree in root.go

Step 9 of plan3-commands"
```

---

### Task 10: README update

**Files:**
- Modify: `README.md`

- [x] **Step 1: Read the current README**

```bash
cat /Users/krishankonig/Workspace/bets/scribe/README.md
```

- [x] **Step 2: Update the command sections**

Find the usage examples section and update with the new command taxonomy. Replace references to `scribe connect` with `scribe registry connect`. Add `remove`, `tools` commands. Update the Quick Start flow:

```markdown
## Quick Start

```bash
# Connect to your team's registry
scribe registry connect ArtistfyHQ/team-skills

# Sync skills to your machine
scribe sync

# See what's installed
scribe list
```

## Commands

### Daily Use

| Command | Description |
|---------|-------------|
| `scribe list` | Show all skills on this machine |
| `scribe add [query]` | Find & install skills from registries |
| `scribe remove <skill>` | Remove a skill from this machine |
| `scribe sync` | Pull updates from connected registries |
| `scribe tools` | List detected AI tools, enable/disable |

### Registry Management

| Command | Description |
|---------|-------------|
| `scribe registry connect <repo>` | Connect to a skill registry |
| `scribe registry create` | Create a new registry repo |
| `scribe registry add` | Share a local skill to a registry |
| `scribe registry list` | List connected registries |
| `scribe registry migrate` | Convert scribe.toml to scribe.yaml |

### Other

| Command | Description |
|---------|-------------|
| `scribe guide` | Interactive setup guide |
| `scribe --version` | Show version |
```

- [x] **Step 3: Verify the README renders correctly**

Skim the updated README for broken links, formatting issues, or stale references.

- [x] **Step 4: Commit**

```bash
git add README.md
git commit -m "[agent] Update README with new command taxonomy

Step 10 of plan3-commands"
```

---

## Dependency Notes

This plan assumes the following from Plan 1 and Plan 2 are complete:

**From Plan 1 (tools rename):**
- `internal/targets/` renamed to `internal/tools/`
- `Target` interface renamed to `Tool`
- `Tool` interface has `Detect() bool` and `Uninstall(skillName string) error`
- `InstalledSkill.Targets` renamed to `InstalledSkill.Tools`
- `config.ToolConfig` struct with `Name string`, `Enabled bool`
- `config.Config.Tools []ToolConfig`
- `tools.AllTools()` returns all known tool implementations
- `tools.EnabledTools(cfg)` returns tools where `Enabled == true`
- `tools.StoreDir()` returns the canonical store path

**From Plan 2 (provider, registry connect):**
- `config.RegistryEntry` struct with `Repo string`, `Enabled bool`, `Type string`, `Writable bool`, `Builtin bool`
- `config.Config.Registries []RegistryEntry` (replaces `TeamRepos []string`)
- `provider.Provider` interface with `Discover()` and `Fetch()`
- `provider.GitHubProvider` implementation

**If Plan 1/2 field names differ from what's assumed here**, the implementing agent should adjust references accordingly. The logic and structure of each task remain the same.
