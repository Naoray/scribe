# Per-Skill Tool Management

**Date:** 2026-04-11
**Status:** Design
**Author:** brainstorming session

## Problem

Scribe currently installs every skill into every globally-enabled tool. Users need finer control: install skill X into Claude only, keep skill Y on both Claude and Cursor, skip Codex entirely for a subset. The global `scribe tools enable/disable` toggle is too coarse — it is all-or-nothing per tool, across every skill.

The infrastructure already exists: `state.InstalledSkill.Tools []string` stores which tools each installed skill lives on. What is missing is any way for the user to edit that list.

## Goals

- Let the user toggle, per skill, which tools receive it.
- Let the user bulk-edit one tool's skill loadout.
- Make the state the source of truth so `scribe sync` respects choices on subsequent runs.
- Provide a non-interactive path for CI and agent scripting.
- Preserve the existing global `tools enable/disable` as the default for newly installed skills.

## Non-Goals

- Registry-side assignment (e.g., a manifest declaring "this skill is Claude-only"). Out of scope; may layer on later.
- Project-scoped tool assignment. Assignment is machine-global, matching how `~/.scribe/state.json` already works.
- Snippets/CLAUDE.md sync. Tracked separately.

## Key Decisions

### 1. State is the source of truth

`state.InstalledSkill.Tools` is already the per-skill tool list. All assignment UI mutates this list and runs matching install/uninstall side effects. No new schema field, no new file. Migration is unnecessary because `Tools` is already populated for existing installs.

### 2. Global enable/disable becomes "default for new installs"

`scribe tools enable/disable` continues to gate which tools a freshly installed skill initially lands on. Once installed, the per-skill `Tools` list is authoritative — toggling `tools disable cursor` does not strip Cursor from already-installed skills. This is the minimum-surprise behavior and matches how the user chose explicit assignments per skill.

### 3. Side effects are synchronous in the per-skill TUI

Toggling a tool on in the list TUI immediately calls `tool.Install(skillName, canonicalDir)` and updates state. Toggling off calls `tool.Uninstall(skillName)` and updates state. No staged "save" step. Rationale: one-skill edits are cheap (one symlink), errors are cheaper to surface immediately, and the convenience north star rewards zero-ceremony edits.

### 4. Side effects are batched in the bulk TUI

`scribe tools edit <name>` presents every installed skill with a checkbox. Toggles mutate a pending in-memory diff. `s` applies the full diff in one wave. `esc` discards. Rationale: bulk edits benefit from preview + undo; a "space to toggle, immediately sync" pattern on 50 skills is noisy and accident-prone.

### 5. Undetected tools are visible but non-interactive

Tools whose `Detect()` returns false render in the toggle list as grey + italic, with `(not found)` annotation. Space and enter are no-ops. Reason: hiding them makes the feature feel inconsistent when a user reinstalls Cursor and expects it to appear.

### 6. Globally disabled tools stay togglable

A tool disabled via `tools disable cursor` still appears in the toggle list in grey with `(disabled)` annotation, and the user may toggle it on. This per-skill opt-in overrides the global disable for that skill only. Rationale: global disable should not be a hard block on per-skill choice.

## User Experience

### A. Per-skill toggle in list TUI

Right pane of the split view gains a third section between Detail and Actions:

```
┌─ commit ─────────────────┐
│ source: Naoray/scribe    │
│ rev:    12               │
│ hash:   abc1234          │
├─ Tools ──────────────────┤
│ [x] claude               │
│ [x] cursor               │
│ [ ] gemini   (disabled)  │
│ [ ] codex    (not found) │
├─ Actions ────────────────┤
│   remove                 │
└──────────────────────────┘
```

- Tab cycles focus: list → tools → actions → list
- Shift+Tab reverses
- In tools section: `space`/`enter` toggles the current row, `↑↓` moves
- Each toggle immediately runs install or uninstall, updates state, and re-renders with fresh counts
- Status bar shows transient confirmation: `✓ commit installed on gemini`
- Failures show in status bar and do not mutate state: `✗ gemini install failed: <err>`

### B. Bulk TUI: `scribe tools edit <name>`

```
$ scribe tools edit cursor

cursor · 12 skills installed · 3 pending changes

[x] commit
[x] plan-design-review
[ ] audit-drift             (pending: remove)
[x] recap                   (pending: add)
[x] ship
...

space toggle · s save · esc cancel · q quit
```

- Shows every skill in `state.Installed`, checked if `cursor` is in its `Tools` list
- `space` toggles the pending state; marker shows `(pending: add)` or `(pending: remove)`
- `s` runs the diff: a sequence of `Install`/`Uninstall` calls, each wrapped in an emitted event
- After save, prints a one-line summary (`✓ 3 changes applied to cursor`) and exits back to the shell
- `esc` or `q` with pending changes prompts: `Discard 3 pending changes? (y/n)`

### C. Scripting: `scribe skill tools <name>`

Non-interactive flat command for agents and CI.

```
scribe skill tools commit                   # print current tool list
scribe skill tools commit --add gemini      # idempotent add
scribe skill tools commit --remove cursor   # idempotent remove
scribe skill tools commit --set claude,codex  # exact set (installs/uninstalls diff)
scribe skill tools commit --json            # machine output: {"tools":["claude","codex"]}
```

- `--add`, `--remove`, and `--set` are mutually exclusive
- Errors on unknown skill name with actionable message
- Runs the same install/uninstall side effects as the TUIs
- Exits non-zero if any side effect fails, with all successful changes preserved

## Architecture

### Core package changes

A new UI-agnostic helper package `internal/tools/assign` exposes pure functions the TUIs and the CLI share:

```go
package assign

// Plan returns the tool names to install and uninstall to move from
// current to desired. Pure; no side effects.
func Plan(current, desired []string) (install, uninstall []string)

// Apply runs install/uninstall side effects for one skill. The caller
// resolves the tool-name slices from Plan into concrete Tool instances
// before invoking Apply.
func Apply(skillName, canonicalDir string, install, uninstall []tools.Tool) []Result

type Result struct {
    Tool string
    Op   Op     // OpInstall | OpUninstall
    Err  error
}
```

Keeping this in its own subpackage avoids bloating `internal/tools/tool.go` and gives the TUIs a narrow seam to mock in tests.

The `Syncer` in `internal/sync/` is updated in one place: when a skill already appears in `state.Installed` with a non-empty `Tools` list, sync uses that list verbatim instead of computing the intersection of enabled + detected tools.

### cmd/ changes

**`cmd/list_tui.go`** — extend the split-view model:

- Add `focusTools` to the `detailFocus` enum, between `focusList` and `focusActions`
- `updateDetail` routes tab and arrow keys through the new section
- New `toolToggleMsg` event carries assign.Result slices back to the model
- Right pane render gains a `renderToolsPane` helper mirroring `renderActions`

**`cmd/tools.go`** — add `newToolsEditCommand`:

```go
func newToolsEditCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "edit <name>",
        Short: "Bulk-edit which skills are installed on a tool",
        Args:  cobra.ExactArgs(1),
        RunE:  runToolsEdit,
    }
    return cmd
}
```

`runToolsEdit` builds a new `toolsEditModel` (new file `cmd/tools_edit_tui.go`) and runs it via `tea.NewProgram`. The model owns a `map[string]pendingState` keyed by skill name.

**`cmd/skill.go`** (new file) — parent `skill` command plus `skill tools` subcommand. Non-TUI, text-only, handles `--add`, `--remove`, `--set`, `--json`.

### State and side-effect ordering

For both TUIs and the CLI path, a single operation on one skill follows this sequence:

1. Compute `install, uninstall := assign.Plan(current, desired)`
2. For each tool in `uninstall`: call `tool.Uninstall(skillName)`. On error, collect and continue.
3. For each tool in `install`: call `tool.Install(skillName, canonicalDir)`. On error, collect and continue.
4. Merge successful results into `state.Installed[skillName].Tools`
5. Save state atomically

Partial failures do not block successful changes. The user sees aggregate success + per-tool errors in the status bar or stderr.

## Error Handling

| Situation | Per-skill TUI | Bulk TUI | CLI |
|---|---|---|---|
| Unknown skill name | Cannot happen (picked from list) | Cannot happen | Exit 1 with "unknown skill" |
| Tool not detected | Toggle is a no-op | Save skips that tool, reports | Exit 1 before any side effect |
| Install symlink fails | Status bar, state unchanged | Reported in save summary, continues | Stderr, exit 1 after other ops |
| State save fails | Status bar, partial rollback impossible — log it | Same | Same |
| Canonical skill dir missing | Refuse toggle, status bar explains | Refuse save | Exit 1 |

## Testing

- `internal/tools/assign`: table-driven tests for `Plan` (current/desired/install/uninstall cases)
- `internal/tools/assign`: `Apply` tested with fake `Tool` implementations (success, one failure mid-batch, all fail)
- `cmd/list_tui_test.go`: exercise the new focus state, tab cycling, and that a toggle emits the expected message
- `cmd/tools_edit_tui_test.go` (new): toggle → save → state roundtrip using `t.Setenv("HOME", t.TempDir())`
- `cmd/skill_test.go` (new): flag validation, mutual exclusion, JSON output shape
- `internal/sync/syncer_test.go`: add a case where a skill already in state with `Tools=[claude]` is not re-added to cursor on the next sync

## Rollout

Single PR. No feature flag. The existing global `tools enable/disable` keeps working unchanged. The list TUI gains a new pane, which is only visible in the split view (already keyboard-gated behind Enter). The `scribe tools edit` and `scribe skill tools` commands are additive.

## Open Questions

None. All forks resolved during brainstorming.
