# Per-Skill Tool Management

**Date:** 2026-04-11
**Status:** Design (revision 2)
**Author:** brainstorming session + counselors review round 1

## Problem

Scribe currently installs every skill into every globally-enabled tool. Users need finer control: install skill X into Claude only, keep skill Y on both Claude and Cursor, skip Codex entirely for a subset. The global `scribe tools enable/disable` toggle is too coarse — it is all-or-nothing per tool, across every skill.

The infrastructure partly exists: `state.InstalledSkill.Tools []string` already stores which tools each installed skill lives on, but the field is populated automatically by sync and does not record user intent. It cannot distinguish "the user chose these tools" from "these were the globally enabled tools at install time."

## Goals

- Let the user toggle, per skill, which tools receive it.
- Let the user bulk-edit one tool's skill loadout.
- Record user intent explicitly so `scribe sync` respects deliberate choices without freezing skills that were never touched.
- Provide a non-interactive path for CI and agent scripting.
- Preserve the existing global `tools enable/disable` as the default for skills the user has not explicitly assigned.
- Keep `tools enable` useful: enabling a new tool globally should backfill it onto existing skills that are still using defaults.

## Non-Goals

- Registry-side assignment (e.g., a manifest declaring "this skill is Claude-only"). Out of scope; may layer on later.
- Project-scoped tool assignment. Assignment is machine-global, matching how `~/.scribe/state.json` already works.
- Per-skill control for **Cursor**. Cursor installs symlinks into `<cwd>/.cursor/rules/`, which is project-scoped, not machine-global. Machine-wide toggles for Cursor would mean "install to whatever directory scribe happens to run in," which is not durable. v1 leaves Cursor under the existing global `tools enable/disable` behavior only. Per-project Cursor assignment is a separate spec.
- Per-skill control for **package** sources. `applyPackage` in the syncer does not populate `InstalledSkill.Tools` today and package content is tool-specific by construction. v1 excludes packages from per-skill assignment. The UI greys them out with the annotation `(package)`.
- Snippets/CLAUDE.md sync. Tracked separately.

## Key Decisions

### 1. Explicit assignment mode on `InstalledSkill`

Add a new field:

```go
// ToolsMode controls how sync reconciles the Tools list.
type ToolsMode string

const (
    ToolsModeInherit ToolsMode = "" // default: recompute from globally enabled tools
    ToolsModePinned  ToolsMode = "pinned" // user explicitly chose; sync respects verbatim
)

type InstalledSkill struct {
    // ... existing fields
    Tools     []string  `json:"tools"`
    Paths     []string  `json:"paths"`
    ToolsMode ToolsMode `json:"tools_mode,omitempty"`
}
```

- `inherit` (default, zero value) — sync recomputes the tool set from the globally enabled, detected tools on every run. Legacy state entries without the field load as inherit.
- `pinned` — user explicitly touched this skill via the per-skill TUI, the bulk `tools loadout` TUI, or the `skill edit --tools` CLI. Sync uses `Tools` verbatim. Global `tools enable/disable` does not affect pinned skills.

Rationale: overloading "non-empty `Tools`" to mean "pinned" would instantly freeze every existing install, because today sync populates `Tools` for every skill. An explicit flag is the only way to preserve meaningful defaults.

### 2. Reconcile on global `tools enable/disable`

When the user runs `scribe tools enable gemini`, scribe iterates `state.Installed`, and for every skill with `ToolsMode == ToolsModeInherit`:
- If gemini is detected and not already in `Tools`, install gemini for that skill and append.
- If the skill is a package source, skip.

`scribe tools disable gemini` does the symmetric uninstall for inherit-mode skills.

Pinned skills are never touched by reconcile. This matches the user's explicit intent.

Reconcile runs inline at the end of `tools enable/disable` and prints a one-line summary per skill affected. The command stays non-destructive for pinned skills.

### 3. State is the source of truth; operations are install-first

Reordered from the original spec. For a single skill's `(current, desired)` diff:

1. Compute `install, uninstall := assign.Plan(current, desired)`.
2. For each tool in `install`: run `tool.Install(skillName, canonicalDir)`. On success, merge into an in-memory "applied" set. On failure, record the error and continue.
3. For each tool in `uninstall`: run `tool.Uninstall(skillName)`. On success, drop from the applied set. On failure, record the error and continue.
4. Compute the new state: start from the current `Tools` slice, apply only the successful install/uninstall results.
5. Save state atomically with both `Tools` and `Paths` updated (install results carry the new paths; uninstall results carry the paths to remove).

Rationale:
- Install-first means a failed add → remove swap leaves the skill on the **old** tool, not nothing. Availability > consistency for this workflow.
- Per-tool success gates per-tool state merge. A failed install never lands in `Tools`; a failed uninstall stays in `Tools`. State always reflects what is actually on disk, not what the user asked for.
- Partial failures do not block other successful changes.

### 4. Self-healing sync

Even outside the assignment flow, the syncer already does a full re-link on every run for skills it touches. Revision: when sync encounters a pinned skill whose state lists a tool that is no longer present on disk (symlink missing, target deleted), it re-runs `Install` for that tool. This makes the system robust to manual `rm -rf ~/.claude/skills/<name>` and to partial failures in the assignment flow.

### 5. Per-skill TUI uses deferred apply

Revised from the original "immediate apply" semantics. The per-skill tool toggles in the list TUI right pane are staged in an in-memory overlay. The apply happens when focus leaves the tools pane (tab away, esc, or close the detail view). This unifies the mental model with the bulk TUI (both are staged + applied on commit), eliminates N-sequential-saves on rapid toggles, and gives the user an implicit "change my mind" window.

A small footer hint makes the model visible: `space toggle · tab/esc apply · a abort`.

### 6. CLI surface rename

Final shape:

- `scribe skill edit <name> [--tools claude,codex] [--add gemini] [--remove cursor] [--pin|--inherit]` — non-interactive edit of one skill's assignment. Mutates `Tools` and optionally flips `ToolsMode`. Default is `--pin` (any explicit assignment pins the skill).
- `scribe tools loadout <tool>` — renamed from the original `tools edit`. Opens the bulk TUI for one tool. Clearer: "show me this tool's skill loadout."

The existing `scribe tools enable/disable` commands stay.

This removes the `tools` double-meaning: global commands live on `scribe tools`, per-skill edits live on `scribe skill edit`.

### 7. Canonical path helper extraction

Before implementation, extract `internal/paths.SkillStore(name string) string` returning `~/.scribe/skills/<name>`. The current syncer, list TUI, and any new assignment code consume this helper. This is a small pre-refactor that prevents every caller from duplicating the path math.

### 8. Undetected tools must still appear in the resolver

`tools.ResolveStatuses` currently starts from detected builtins + manual config. A builtin that is not installed and not in config is invisible. For the tools pane to render `[ ] codex (not found)` consistently, the resolver needs to always include every builtin, with `DetectKnown: true, Detected: false`. One-line change in `internal/tools/runtime.go`.

### 9. Visual differentiation of "not found" vs "disabled"

Two states render differently in the tools pane:

| State | Render | Togglable |
|---|---|---|
| Enabled, detected, in `Tools` | `[x] claude` | yes |
| Enabled, detected, not in `Tools` | `[ ] claude` | yes |
| Globally disabled, detected | `( ) gemini (disabled)` | yes (per-skill override) |
| Not detected | `[-] codex (not found)` | no |
| Cursor (project-scoped) | `[-] cursor (project-scoped)` | no |
| Skill is a package source | `[-] <tool> (package)` | no |

Cursor is never togglable in the per-skill or bulk TUIs (see Non-Goals). Packages are never togglable.

## User Experience

### A. Per-skill tool toggles in list TUI

Right pane of the split view gains a middle section between Detail and Actions:

```
┌─ commit ─────────────────────┐
│ source: Naoray/scribe        │
│ rev:    12                   │
│ hash:   abc1234              │
│ mode:   inherit              │
├─ Tools ──────────────────────┤
│ [x] claude                   │
│ [ ] codex                    │
│ ( ) gemini (disabled)        │
│ [-] cursor (project-scoped)  │
│ [-] aider  (not found)       │
├─ Actions ────────────────────┤
│   remove                     │
└──────────────────────────────┘

space toggle · tab apply · a abort · q quit
```

- Tab cycles focus: list → tools → actions → list.
- In tools section: `space`/`enter` toggles the current row into a pending state; `↑↓` moves; `a` aborts all pending changes for this skill.
- Pending rows render with a `*` prefix: `* [x] codex` means the row is currently unchecked on disk but will be installed on apply.
- Apply happens when focus leaves the tools pane for any reason (tab, esc, arrow out).
- The first pending toggle flips `ToolsMode` to `pinned` on apply. Confirmation appears in status bar: `✓ commit pinned to [claude codex]`.
- The `mode` field in the Detail pane reflects `inherit` or `pinned` live.

### B. Bulk TUI: `scribe tools loadout <tool>`

Unchanged from the original spec except for the rename:

```
$ scribe tools loadout codex

codex · 12 skills installed · 3 pending changes

[x] commit
[x] plan-design-review
[ ] audit-drift             (pending: remove)
[x] recap                   (pending: add)
[x] ship
...

space toggle · s save · esc cancel · q quit
```

Save applies all pending changes in one wave using the same install-first ordering. Every affected skill gets pinned. Package skills are not listed.

### C. Non-interactive: `scribe skill edit <name>`

```
scribe skill edit commit                       # print current assignment + mode
scribe skill edit commit --tools claude,codex  # exact set, pins
scribe skill edit commit --add gemini          # idempotent add, pins
scribe skill edit commit --remove codex        # idempotent remove, pins
scribe skill edit commit --inherit             # return to inherit mode; sync will recompute
scribe skill edit commit --json                # machine output
```

- `--tools`, `--add`, `--remove`, `--inherit` are mutually exclusive except that `--add` and `--remove` can be combined.
- Any mutation implicitly pins the skill unless `--inherit` is passed.
- Exits non-zero on unknown skill, partial failure, or invalid tool name.

### D. Reconcile in `scribe tools enable/disable`

```
$ scribe tools enable gemini
Enabled gemini globally.
Backfilling inherit-mode skills...
  ✓ commit          (+gemini)
  ✓ plan-design-review (+gemini)
  ⏭ my-override     (pinned, skipped)
  ✓ audit-drift     (+gemini)
3 skills updated, 1 skipped.
```

Uses the same install-first ordering and partial-failure semantics.

## Architecture

### `tools.Tool` interface change

`Uninstall` must report which paths it removed so the merge step can drop them from `state.InstalledSkill.Paths`. Current signature is `Uninstall(skillName string) error`; new signature is `Uninstall(skillName string) (removed []string, err error)`. Four existing implementations update (`ClaudeTool`, `CursorTool`, `GeminiTool`, `CodexTool`) plus the custom-tool runtime wrapper. This is strictly additive from the caller's perspective — existing `_ = tool.Uninstall(...)` sites become `_, _ = tool.Uninstall(...)`.

Rationale: `state.InstalledSkill.Paths` is a flat slice without a per-tool key. Without a return value from `Uninstall`, there is no reliable way to know which paths to drop when only one tool is removed. Re-deriving the path from convention is fragile (custom tools define arbitrary install paths).

### New package: `internal/tools/assign`

Pure functions shared by TUIs, CLI, and the reconcile path:

```go
package assign

type Mode int
const (
    ModeAdd    Mode = iota // successful install adds to state.Tools
    ModeRemove              // successful uninstall drops from state.Tools
)

type Op struct {
    Tool string
    Mode Mode
}

// Plan returns the install and uninstall Ops needed to move current → desired.
// Pure; no side effects.
func Plan(current, desired []string) (install, uninstall []Op)

// Result records the outcome of one Op.
type Result struct {
    Op    Op
    Paths []string // populated on successful install
    Err   error
}

// Apply runs Ops in install-first order, returning per-Op results. Caller
// resolves tool names to concrete tools.Tool instances before invoking.
func Apply(skillName, canonicalDir string, install, uninstall []tools.Tool) []Result

// Merge folds successful Results into a copy of the current state slice,
// updating both Tools and Paths. Failed ops leave their tool unchanged.
func Merge(installed state.InstalledSkill, results []Result) state.InstalledSkill
```

### Syncer changes

`internal/sync/syncer.go`:
- For skills with `ToolsMode == ToolsModePinned`, skip the "which tools" computation and use `state.Tools` verbatim.
- Self-heal: for each tool in `state.Tools`, verify the expected symlink exists via `os.Lstat` on every recorded path in `state.Paths` that belongs to that tool. If missing, re-run `Install`. Path-to-tool association is re-derived by matching path prefixes against each `tools.Tool.Name()`-specific root.
- `applyPackage` is unchanged. Packages always stay inherit mode with empty `Tools`.
- Cursor is excluded from both the pinned path and the self-heal path. It continues to use today's per-sync behavior (install into the current working directory's `.cursor/rules/`).

### cmd/ changes

- `cmd/list_tui.go`: add `focusTools` between `focusList` and `focusActions`. Add `pendingTools map[string]bool` and an apply-on-defocus handler. `renderToolsPane` builds rows from `tools.ResolveStatuses` + current `state.Tools` + pending overlay.
- `cmd/tools.go`: add `newToolsLoadoutCommand`. Deletes the old `edit` subcommand if any. `runToolsEnable`/`runToolsDisable` call a new `reconcileInheritSkills` helper.
- `cmd/tools_loadout_tui.go` (new): bulk TUI model, same `assign.Apply` path.
- `cmd/skill.go` (new): parent `skill` command + `edit` subcommand. Shares flag parsing and mutex rules.

### `internal/tools/runtime.go` changes

`ResolveStatuses` always emits every builtin, marking undetected ones with `Detected: false`. One-line behavior change, covered by a new test case.

## Error Handling

| Situation | Per-skill TUI | Loadout TUI | `skill edit` CLI | Reconcile |
|---|---|---|---|---|
| Unknown skill name | n/a (picked from list) | n/a | Exit 1 | n/a |
| Invalid tool name (--add foo) | n/a | n/a | Exit 1 before any op | n/a |
| Tool not detected | Toggle is no-op | Not listed | Exit 1 | Skipped silently |
| Package skill | Not eligible | Not listed | Exit 1 with "packages unsupported" | Skipped silently |
| Install symlink fails | Status bar, per-tool not added to state | Save summary, continues | Stderr, continues, exit 1 at end | Printed in summary, continues |
| Uninstall symlink fails | Same — per-tool not removed from state | Same | Same | Same |
| State save fails | Status bar; in-memory state already reflects filesystem — user can retry | Same | Stderr, exit 1 | Same |
| Cursor eligible anywhere? | No | No | `--add cursor` → exit 1 | No |

## Testing

- `internal/tools/assign`: table-driven tests for `Plan`, `Apply` (with fake tools covering success, mid-batch failure, all fail), and `Merge`.
- `internal/tools/runtime_test.go`: `ResolveStatuses` now emits undetected builtins.
- `internal/sync/syncer_test.go`:
  - Pinned skill with `Tools=[claude]` not re-added to cursor after `tools enable cursor`.
  - Inherit skill gets cursor added on `tools enable cursor` reconcile.
  - Pinned skill with missing symlink triggers self-heal.
  - Package skill never pinned, never surfaced to assignment flows.
- `cmd/list_tui_test.go`: focus cycling into tools pane, toggle staging, apply on defocus, pinning side effect.
- `cmd/tools_loadout_tui_test.go` (new): staging + save + roundtrip via `t.Setenv("HOME", t.TempDir())`.
- `cmd/skill_test.go` (new): flag mutex, `--inherit` resets mode, `--add cursor` rejected.
- `cmd/tools_test.go`: reconcile path, pinned skills skipped, summary output.

## Rollout

Single PR. No feature flag. Migration is a no-op: new `ToolsMode` field defaults to `inherit` for existing state entries, preserving current behavior on the first sync after upgrade.

The existing global `tools enable/disable` gains the reconcile behavior. Users who previously relied on "enable never touches installed skills" will see a one-time backfill — this is the intended fix for the biggest coarse-toggle pain point, and pinned skills (which don't exist yet for any current user) are still respected.

## Open Questions

None. All counselors-round-1 ship-blockers resolved.
