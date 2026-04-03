# List Detail View & Version Infrastructure

**Date:** 2026-04-03
**Branch:** `feat/list-detail-view` (based on `feat/list-local-disk-discovery`)

## Summary

Evolve `scribe list --local` from a single-column skill browser into a split-pane layout with inline descriptions, a reactive detail preview, an action menu, and version tracking infrastructure.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Detail view layout | Split pane (list left, detail right) | No context loss — user sees neighbors while inspecting a skill |
| Action menu placement | Right pane (replaces metadata on enter) | Consistent with split-pane choice; left pane dims to signal focus shift |
| Left pane format | `name — truncated description` | Scannable at a glance without cursor movement |
| Version in left pane | Name only; colored `↑` arrow when outdated (future) | Keep left pane clean; version always visible in right pane |
| Version source | Frontmatter → state.json → content hash | Every skill gets a comparable identifier |
| `Managed` field | Dropped from `Skill` struct; kept as derived in JSON output | JSON backwards compat: `_, ok := state.Installed[name]; ok` — state presence is the sole indicator |
| Update action | Shown greyed out, "source unknown" | Placeholder until categories-by-source exists |
| Narrow terminal | Fall back to single-column below 80 cols | Split pane unusable at narrow widths |

## 1. Discovery: `readSkillMetadata`

### Changes to `internal/discovery/discovery.go`

**Rename** `readSkillDescription` → `readSkillMetadata`. Parse additional frontmatter fields:

```yaml
---
name: browse
version: 1.1.0
description: Fast headless browser for QA testing...
---
```

Returns a `SkillMeta` struct, consumed by `OnDisk` to populate `Skill` fields:

```go
type SkillMeta struct {
    Name        string // from frontmatter `name:` field
    Description string // from frontmatter `description:` or first paragraph
    Version     string // from frontmatter `version:` field, empty if absent
}

// In OnDisk:
meta := readSkillMetadata(skillDir)
sk := Skill{
    Name:        entry.Name(),       // filesystem name is canonical
    Description: meta.Description,
    Version:     meta.Version,       // may be empty — resolved later
    // ...
}
```

The filesystem directory name remains the canonical skill name. `meta.Name` is only used for display if it differs (not currently needed but parsed for completeness).

**`discovery.Skill` struct changes:**

- Drop `Managed bool` from struct (keep as derived in JSON — see section 4)
- Add `ContentHash string` — always computed once in `OnDisk()`, cached on the struct
- `Version` resolution order:
  1. SKILL.md frontmatter `version:` (e.g. `1.1.0`)
  2. state.json `Version` (stamped at install time)
  3. Content hash prefixed with `#` (e.g. `#a3f2c1b8`)

Since content hash is always computed, every skill always has a version. The `–` display is unreachable in normal operation.

**Content hash computation** (`internal/discovery/hash.go`):

```
SHA256( sorted-relative-paths + file-contents )
```

- Walk the skill directory recursively
- **Follow symlinks** via `filepath.EvalSymlinks` before reading — hash the resolved content, not the link target path. Two skills pointing to the same source will have the same hash (correct for content comparison).
- Sort files by relative path for determinism
- Concatenate `relativePath + contents` for each file
- SHA256 the result, take first 8 hex chars
- **Exclude**: `.git/` (at any depth), `.DS_Store` files, `node_modules/` directories. Do NOT exclude all dotfile directories — `.github/` and similar may contain legitimate skill content.
- **Symlink walk**: call `filepath.EvalSymlinks` on each file path before reading. If `EvalSymlinks` returns an error (e.g., broken symlink, circular link), skip the file and continue.
- **Normalize line endings**: convert CRLF → LF before hashing content for cross-platform determinism.
- **Deliberate design choice**: two skills whose symlinks resolve to the same source directory will produce the same hash. This is correct for content comparison (they are the same skill). Document this in the code so nobody "fixes" it later.
- **Compute once** in `OnDisk()` during skill enumeration, store on `Skill.ContentHash`. Never recompute in `View()` or on cursor movement.

### Why content hash matters

Most SKILL.md files don't have a `version:` field today. The content hash gives every skill a comparable fingerprint without requiring skill author cooperation. Future `scribe update` compares installed hash vs source hash. Future `scribe tag` writes an explicit `version:` into frontmatter for skills with known sources.

## 2. List TUI: Split Pane Layout

### Phase: `listPhaseSkills` (rendering change, no new phase)

Replace single-column `viewSkills` with a two-column layout using `lipgloss.JoinHorizontal`.

**Narrow terminal fallback:** When `m.width < 80`, render the original single-column layout instead of the split pane. This is a simple conditional in `viewSkills()` — no separate code path, just `if m.width < 80 { return m.viewSkillsSingleColumn() }`. The single-column view uses the existing format but with inline descriptions (`name — desc` on one line).

**Left pane:**

```
  ascii — ASCII diagram generator
▸ browse — Fast headless browser for QA...
  canary — Post-deploy canary monitoring
  careful — Safety guardrails for destructi...
  ↓ 24 more below
```

- `name` + ` — ` + description, truncated to fit pane width via `runewidth.Truncate`
- Cursor indicator `▸` on current skill
- Scrollable — each item is 1 line (not 2), so `ensureCursorVisible` scroll math uses `contentHeight` directly (not `maxContentLines() / 2`). Drop the old `maxContentLines()` helper; use `contentHeight` computed in `viewSkills()` for both scroll window and pane rendering.
- Future: orange `↑` after name when outdated (not implemented now)

**Right pane:**

```
browse
Fast headless browser for QA testing and site
dogfooding. Navigate pages, interact with...
──────────────────────────────
Version   1.1.0
Package   gstack
Source    garrytan/gstack
Targets   claude
Path      ~/.claude/skills/browse
```

- Updates reactively as cursor moves (no enter needed)
- Description rendered with `lipgloss.NewStyle().Width(rightWidth)` for word wrapping
- If description + metadata exceeds available height, truncate description with `...` (the action menu's "open in $EDITOR" serves as the full-content escape hatch)
- Key-value metadata with aligned labels
- Version shows frontmatter version, state version, or `#hash`
- Fields with empty values are omitted (e.g., no Source line if source unknown)

**Fixed-height pane rendering:** Both panes must be rendered to exactly the same height before `JoinHorizontal`. Pad shorter panes with empty lines:

```go
contentHeight := m.height - headerHeight - footerHeight
leftRendered  := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight).Render(leftContent)
rightRendered := lipgloss.NewStyle().Width(rightWidth).Height(contentHeight).Render(rightContent)
divider       := lipgloss.NewStyle().Height(contentHeight).Render(strings.TrimRight(strings.Repeat("│\n", contentHeight), "\n"))
body          := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, divider, rightRendered)
```

**Width calculation:**

```go
leftWidth  = min(m.width * 45 / 100, m.width - 40, 60)  // ensure right pane gets ≥40 chars, cap at 60
rightWidth = m.width - leftWidth - 3                     // 3 for divider + padding
```

Triple-`min` ensures: right pane always has room for metadata, left pane doesn't waste space on ultra-wide terminals. On a 120-col terminal: left=54, right=63. On an 80-col terminal: left=36, right=41. On a 200-col terminal: left=60, right=137.

**Search behavior:** Type-to-search remains active only in `listPhaseSkills`. The search query renders in the left pane header area (below the horizontal divider, above the skill list — costs 1 line from `contentHeight` when active). The right pane updates to show detail for the first matching skill. Search is disabled in `listPhaseActions`.

**Layout height budget:**

```
headerHeight = 2  // title line + horizontal divider
searchHeight = 0 or 1  // only when search is active
footerHeight = 2  // blank line + help text
contentHeight = m.height - headerHeight - searchHeight - footerHeight
```

`contentHeight` is the single source of truth for both scroll window and pane rendering. The old `maxContentLines()` helper is removed.

## 3. Action Menu Phase

### New phase: `listPhaseActions`

Pressing enter in `listPhaseSkills` transitions to `listPhaseActions`:

- Left pane renders identically but with **dimmed styling** (lipgloss foreground color override) to signal focus is on the right
- Left pane cursor is frozen — up/down navigates the action menu instead
- Right pane shows:
  - Skill name (bold) + short metadata line (package + version)
  - Divider
  - Action list with cursor

**Actions:**

| Action | Behavior | Availability |
|--------|----------|-------------|
| `update` | Check source for newer version | Greyed + "source unknown" until categories-by-source exists |
| `remove` | Delete skill from disk + state | Always (disabled for state-only ghost skills — see below) |
| `add to category` | Assign/change skill category | Always available |
| `copy path` | Copy skill's `LocalPath` to clipboard | Disabled for state-only ghost skills |
| `open in $EDITOR` | Open SKILL.md in external editor | Disabled for state-only ghost skills |

**State-only ghost skills** (exist in state.json but not on disk, i.e. `LocalPath == ""`): disable disk-dependent actions (copy path, open editor). Remove action becomes "Remove from state" (state cleanup only, no disk deletion).

**Key bindings in action phase:**

| Key | Action |
|-----|--------|
| `up/k`, `down/j` | Navigate action list |
| `enter` | Execute selected action (no-op on greyed/disabled items) |
| `esc` | Return to `listPhaseSkills` (right pane reverts to metadata) |
| `q`, `ctrl+c` | Quit |

**Action execution:**

- `copy path`: `tea.SetClipboard(skill.LocalPath)` — handle `tea.ClipboardMsg` for success/failure (may fail on SSH without OSC 52, headless terminals). Show confirmation or error in right pane. Auto-return to skills phase after 1s via `tea.Tick`. Track a `pendingTickID int` that increments on every new tick; when the tick message arrives, discard it if its ID doesn't match the current `pendingTickID`. This handles cancellation on esc, phase transitions, and quit — not just esc.
- `open in $EDITOR`: Resolve editor via `$VISUAL` → `$EDITOR` → `vi` fallback chain (Unix convention: `$VISUAL` is for full-screen editors). `tea.Exec(exec.Command(editor, skillMDPath), callback)` — on return, always re-read skill metadata to pick up any changes (even if editor exited non-zero, the user may have saved before the error). If editor exit code is non-zero, show a warning in the right pane but don't block.
- `remove`: Enter a **confirm micro-substate** (`listSubstateConfirm`) within the action phase. Show "Remove browse? (y/n)" in right pane. Only `y` and `n` keys are active; all other keys are no-ops. On confirm:
  - **Safety check**: Validate that `LocalPath` is contained within `~/.scribe/skills/` or `~/.claude/skills/` before any deletion. Reject paths outside these directories.
  - If skill `LocalPath` is a symlink: delete the symlink only (`os.Remove`), not the target
  - If skill is a regular directory: `os.RemoveAll(LocalPath)`
  - Delete remaining paths in `InstalledSkill.Paths` (deduplicated against `LocalPath` to avoid double work)
  - Call `state.Save()` **immediately** after removing from in-memory state. If Save fails, show error in right pane and do not proceed with disk deletion. Order: save state → delete from disk (prevents orphaned state on interrupt).
  - On cancel: return to action menu
- `update`: No-op for now, show "source unknown" message inline
- `add to category`: Future — shows in menu, not wired yet

**Signal handling:** Handle both `tea.KeyPressMsg` for `"ctrl+c"` and `tea.InterruptMsg` in `Update()` for proper SIGINT behavior in raw mode.

### Phase constants

```go
const (
    listPhaseGroups  listPhase = iota  // keep as zero value (safe default)
    listPhaseSkills
    listPhaseActions
)
```

`listPhaseGroups` stays as iota 0 so the zero-value default is the safe starting phase.

## 4. Version Display Rules

| Source | Display in right pane | Display in left pane |
|--------|----------------------|---------------------|
| Frontmatter `version: 1.1.0` | `1.1.0` | (none, unless outdated → orange `↑`) |
| State.json version | `v1.0.0` or `main@a3f2c1b` | (none) |
| Content hash only | `#a3f2c1b8` | (none) |

Resolution: frontmatter wins over state.json wins over content hash. Content hash is always available as fallback, so every skill has a displayable version.

### JSON output (`--json`)

The `--json` output includes:

```json
{
  "name": "browse",
  "description": "Fast headless browser...",
  "version": "1.1.0",
  "content_hash": "a3f2c1b8",
  "package": "gstack",
  "source": "garrytan/gstack",
  "path": "~/.claude/skills/browse",
  "targets": ["claude"],
  "managed": true
}
```

- `path` — keeps existing JSON field name (current `printLocalJSON` emits `path`, not `local_path`; changing it would break consumers)
- `managed` — derived field: `_, ok := state.Installed[name]; ok`. State presence is the sole indicator. This preserves backwards compatibility.
- `content_hash` — always present (new field)
- `version` — follows the resolution chain (frontmatter → state → `#hash`). **Behavior change from current**: previously empty for untracked skills, now always populated (hash fallback). Call this out in release notes.

`readSkillMetadata` continues to use **line-by-line frontmatter parsing** (same approach as current `readSkillDescription`), not a YAML library. This avoids YAML type coercion issues where unquoted `1.1.0` might be parsed as a float.

## 5. Future: `scribe tag` and `scribe update`

**Not built in this iteration.** The version infrastructure (content hash + frontmatter version) enables:

- `scribe tag <skill> [version]` — writes `version:` into SKILL.md frontmatter. Only when source is known (from categories-by-source config), or user provides version explicitly.
- `scribe update [skill]` — compares installed content hash vs source content hash. Requires categories-by-source mapping to know where to fetch from.
- Outdated indicator — orange `↑` in left pane when installed hash differs from source hash.

## 6. Future: Registry Loading Spinner

**Descoped from this iteration.** The registry-based `scribe list` (without `--local`) uses a different code path (`printMultiListTable` / workflow steps) that never enters the Bubble Tea TUI. Adding a loading spinner requires reworking the registry list into either:

- A minimal `tea.Program` wrapping a spinner + async fetch, or
- Converting the full registry list to a TUI

This is a workflow/output-mode change, not a TUI phase addition. It deserves its own spec and implementation.

## Files to Create/Modify

| File | Change |
|------|--------|
| `internal/discovery/discovery.go` | Rename `readSkillDescription` → `readSkillMetadata`, parse `version:`, drop `Managed` from struct |
| `internal/discovery/hash.go` | New — content hash computation (symlink-following, dotfile-excluding) |
| `cmd/list_tui.go` | Split-pane layout, fixed-height rendering, action menu phase, confirm substate, dimmed left pane, inline descriptions, narrow terminal fallback, `InterruptMsg` handling |
| `cmd/list.go` | Pass `cmd.Context()` to `tea.NewProgram` via `tea.WithContext` |

## Out of Scope

- `scribe tag` command
- `scribe update` command
- Outdated arrow indicator in left pane
- `add to category` action implementation (shows in menu, not wired)
- Categories-by-source-repo feature
- Registry loading spinner (descoped — needs its own spec)
