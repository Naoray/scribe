# Storage Model Redesign & List TUI Overhaul

**Date:** 2026-04-10
**Status:** Draft
**Builds on:** 2026-04-06-mvp-design.md

## Summary

Redesign the skill storage model from registry-namespaced directories to flat single-copy storage with multi-source tracking, local versioning, and modification protection. Fix 8 list TUI issues uncovered during real usage.

## Motivation

The current storage model creates `~/.scribe/skills/<registry-slug>/<skill-name>/` with symlinks at `~/.claude/skills/<registry-slug>/<skill-name>`. Claude Code only discovers skills one level deep in `~/.claude/skills/`, so nested skills are invisible — users don't get them in autocomplete. Additionally, the same skill appears twice in `scribe list` (qualified + bare name via legacy symlinks).

Users also modify skills locally (e.g., customizing `/recap`) and expect those modifications to survive `scribe sync`. The current SHA-based version comparison is not human-readable and provides no way to restore previous versions.

---

## 1. Flat Storage Model

### Current (broken)

```
~/.scribe/skills/Artistfy-hq/cleanup/SKILL.md    # registry-namespaced
~/.claude/skills/Artistfy-hq/cleanup              # nested symlink — Claude can't find this
```

### New

```
~/.scribe/skills/cleanup/
  SKILL.md                    # current working copy
  versions/
    v1.md                     # snapshot before v2 was synced
    v2.md                     # snapshot before v3 was synced
~/.claude/skills/cleanup      # flat symlink — Claude discovers this
```

### Rules

- One canonical copy per skill name in `~/.scribe/skills/<name>/`
- Symlinks to tool directories use bare names: `~/.claude/skills/<name>` → `~/.scribe/skills/<name>/`
- State tracks which registries provide each skill (multi-source)
- No registry-slug directories on disk

### Migration

On first run after upgrade, migrate existing namespaced directories:
1. For each `~/.scribe/skills/<slug>/<name>/`, move to `~/.scribe/skills/<name>/`
2. If bare-name target already exists: compare content hashes — if identical, just delete the slug copy; if different, keep the newer one (by state timestamp)
3. Update symlinks in `~/.claude/skills/` and `~/.cursor/` to point to new flat paths
4. Update state.json keys (strip slug prefix for skills that moved)
5. Remove empty slug directories
6. Package-type entries (with `install_cmd`/`update_cmd`) have no local files to move — only their state keys are migrated from qualified to bare names

---

## 2. Multi-Source State Tracking

### State schema change

```json
{
  "installed": {
    "cleanup": {
      "local_version": 3,
      "installed_hash": "a1b2c3d4",
      "sources": [
        {
          "registry": "Artistfy/hq",
          "ref": "main",
          "last_sha": "abc123",
          "last_synced": "2026-04-10T10:00:00Z"
        },
        {
          "registry": "sandorian/tools",
          "ref": "v1.2.0",
          "last_sha": "def456",
          "last_synced": "2026-04-09T08:00:00Z"
        }
      ],
      "tools": ["claude", "cursor"],
      "paths": ["~/.claude/skills/cleanup", "~/.cursor/rules/cleanup"],
      "installed_at": "2026-04-05T12:00:00Z",
      "type": "",
      "install_cmd": "",
      "update_cmd": ""
    }
  }
}
```

### Key changes from current schema

| Field | Old | New |
|-------|-----|-----|
| Key | `Artistfy-hq/cleanup` (qualified) | `cleanup` (bare) |
| `version` | Git ref string | Removed — replaced by `local_version` |
| `commit_sha` | Single SHA | Removed — moved into `sources[].last_sha` |
| `source` | Single source string | Removed — replaced by `sources[]` array |
| `local_version` | N/A | Monotonic integer, bumped on each sync that changes content |
| `installed_hash` | N/A | Content hash at time of last sync/install — used to detect local modifications |
| `sources` | N/A | Array of registries that provide this skill |

### Backward compatibility

`state.Load()` already has migration logic (`parseAndMigrate`). Add a new migration step:
- If key contains `/` and matches pattern `<slug>/<name>`: convert to bare-name key, populate `sources` from old `source` field, set `local_version: 1`, compute `installed_hash`

---

## 3. Local Versioning

### Version snapshots

Before `scribe sync` overwrites a skill's files:
1. Copy current `SKILL.md` to `versions/v{N}.md` (where N = current `local_version`)
2. Increment `local_version` to N+1
3. Write new content as `SKILL.md`
4. Update `installed_hash` to hash of new content

### Directory layout

```
~/.scribe/skills/cleanup/
  SKILL.md              # current (v3)
  scripts/deploy.sh     # multi-file skills: only SKILL.md is versioned
  versions/
    v1.md               # first synced version
    v2.md               # second synced version
```

Only `SKILL.md` is versioned — auxiliary files (scripts, references) are not snapshotted. This keeps storage bounded. If a user needs full-directory snapshots, git is the right tool.

### Restore

`scribe restore cleanup v2` — copies `versions/v2.md` back to `SKILL.md`. Sets `installed_hash` to new content hash (so it's treated as "locally modified" and won't be overwritten on next sync).

### Display

`scribe list` shows version as `v3` (clean) or `v3*` (locally modified).

---

## 4. Local Modification Protection

### Detection

At sync time, for each skill that has a newer upstream version:
1. Compute current content hash of `SKILL.md` on disk
2. Compare to `installed_hash` in state
3. If they differ → skill was locally modified

### Behavior

| Scenario | Action |
|----------|--------|
| No local mods, upstream unchanged | Skip (current) |
| No local mods, upstream newer | Update (snapshot old version, write new) |
| Local mods, upstream unchanged | Skip (show "modified" status) |
| Local mods, upstream newer | **Skip with warning** — "cleanup has local modifications, skipping. Use `scribe sync --force` to override." |
| `--force` flag | Update anyway (still snapshots old version first) |

### New status: `StatusModified`

Add to `sync.Status`:
```go
StatusModified  Status = 4  // installed, locally modified
```
Display: icon `✎`, label `modified`, color blue `#3B82F6`.

---

## 5. Collision Handling

When two registries provide a skill with the same name:

### During sync

- All registries are checked. If the skill already exists locally, the source is added to `sources[]`
- Content comparison: if both registries have identical content → no conflict, both tracked as sources
- If content differs → use the version from whichever registry was synced most recently (by `last_synced` timestamp, not SHA)
- Emit warning: "cleanup exists in both Artistfy/hq and sandorian/tools with different content — using Artistfy/hq version (synced more recently)"

### In `scribe list`

Show skill once. Detail pane shows all registries under "Sources" field:
```
Sources   Artistfy/hq (main), sandorian/tools (v1.2.0)
```

### Name collision with genuinely different skills

If `Artistfy/hq` has a `cleanup` that cleans code and `sandorian/tools` has a `cleanup` that cleans temp files — same name, different purpose. This is a genuine conflict. The warning during sync surfaces it. User resolves by:
1. Renaming one skill in their registry (`cleanup` → `cleanup-files`)
2. Or disconnecting one registry

Scribe doesn't try to solve namespace collision automatically — it warns and uses the latest.

---

## 6. `ClaudeTool.Install` Change

### Current

```go
func (t ClaudeTool) Install(skillName, canonicalDir string) ([]string, error) {
    link := filepath.Join(skillsDir, skillName)  // creates nested path for qualified names
```

### New

```go
func (t ClaudeTool) Install(skillName, canonicalDir string) ([]string, error) {
    // Extract bare name — skillName is already bare in the new model
    link := filepath.Join(skillsDir, skillName)
```

Since the storage model is now flat, `skillName` passed to `Install` is already bare. The change happens in the syncer: `WriteToStore` writes to `~/.scribe/skills/<name>/` (no slug prefix), and the syncer passes bare `sk.Name` instead of `qualifiedName` to both `WriteToStore` and `Tool.Install`. State keys also use bare names.

`WriteToStore` signature changes: drop the `registrySlug` parameter. The skill goes directly into `~/.scribe/skills/<name>/`.

---

## 7. List TUI Fixes

### 7.1 Actions fix

`listRow` gains a new field:

```go
type listRow struct {
    // ... existing fields ...
    Entry     *manifest.Entry  // from SkillStatus.Entry, nil for local-only
    LatestSHA string           // for triggering update
}
```

`actionsForRow` logic:

| Action | Enabled when |
|--------|-------------|
| update | `row.HasStatus && row.Status == StatusOutdated && row.Entry != nil` |
| remove | `row.Local != nil && row.Local.LocalPath != ""` |
| copy path | `row.Local != nil && row.Local.LocalPath != ""` |
| open in editor | `row.Local != nil && row.Local.LocalPath != ""` |
| add to category | Always disabled (coming soon) |

### 7.2 Update action (single-skill sync)

When user selects "update" on an outdated skill:

1. Show "Updating..." status in action area
2. Create a `tea.Cmd` that:
   - Creates a `Syncer` with the bag's client, provider, and tools
   - Calls `syncer.RunWithDiff(ctx, registry, []SkillStatus{selectedSkill}, state)` — single-entry diff
   - Returns `updateDoneMsg{name, err}`
3. On success: update the row's status to `StatusCurrent`, show "Updated!" for 1 second
4. On error: show error message in status area

New messages:
```go
type updateStartMsg struct{ name string }
type updateDoneMsg  struct{ name string; err error }
```

### 7.3 Skill excerpt in detail pane

Read first ~8 lines of SKILL.md body (after frontmatter) and display as dimmed text in the bottom-right quadrant of the detail pane.

Add `Excerpt string` to `listRow`, populated during `buildRows` from a new `readSkillExcerpt(localPath string) string` function.

Layout change for detail pane (right side):
```
┌─────────────────────────┐
│ cleanup                 │  ← skill name
│ Clean up stale code...  │  ← description
│─────────────────────────│
│ Status   current        │  ← metadata
│ Version  v3             │
│ Sources  Artistfy/hq    │
│ Path     ~/.scribe/...  │
│─────────────────────────│
│ ▸ update                │  ← actions
│   remove                │
│   open in editor        │
│   copy path             │
│─────────────────────────│
│ ## Instructions         │  ← excerpt (dimmed)
│ Run this skill when...  │
│ ...                     │
└─────────────────────────┘
```

### 7.4 Search visibility

Always show search indicator below the header:

- Empty state: `/ search...` (dimmed placeholder)
- Active: `/ {query}` with query highlighted
- Shows in both full and split view

Update help text: `↑↓ navigate · /search · enter detail · q quit`

The `/` prefix echoes the slash-command convention users already know.

### 7.5 Alt screen removal

Remove `v.AltScreen = true` from `View()`. The TUI renders inline below the command that was typed.

Height calculation: use `min(terminalHeight, contentNeeded)` so small skill lists don't waste vertical space. The TUI should only take as much space as it needs.

### 7.6 Cursor overflow fix

**Problem:** In full-width view, 1 item is focusable but hidden above the visible area. In split view, ~3 items are hidden behind the header.

**Root cause:** `contentHeight()` returns a height that doesn't match what's actually visible. The header takes 2 lines, but group headers within the content area also consume lines — and `ensureCursorVisible` doesn't account for them.

**Fix:**
1. Audit `contentHeight()` — currently subtracts `headerHeight=2 + footerHeight=3`. With the new search bar always visible, footer is 3 lines (blank + summary + help) and header is 3 lines (title + divider + search). Adjust constants.
2. In `ensureCursorVisible`, count the group headers between `offset` and `cursor` as consumed lines, reducing the effective visible window.

### 7.7 Editor configurable

Add `editor` field to config:

```yaml
editor: cursor    # or "code", "vim", "nvim", etc.
```

Resolution order: `config.editor` → `$VISUAL` → `$EDITOR` → `vi`

Action label: **"Open in Editor"** (not "open in $EDITOR").

New command: `scribe config set editor cursor` — writes the editor preference to `~/.scribe/config.yaml`.

---

## 8. Discovery Changes

### `discovery.OnDisk` — flat scan

Since skills are now flat in `~/.scribe/skills/`, the scan simplifies:
- `~/.scribe/skills/<name>/` — every directory with SKILL.md is a skill (no registry-slug nesting)
- `~/.claude/skills/<name>/` — symlinks pointing back to scribe store (deduplicate by resolved path)
- Dedup by resolved physical path (`filepath.EvalSymlinks`), not by name

### Detecting local modifications

`OnDisk` can compute current content hash and compare against `installed_hash` in state. Add `Modified bool` field to `discovery.Skill`.

---

## 9. Scope & Sequencing

This spec covers two concerns that are coupled through the dedup fix:

**Phase 1: Storage model + state migration** (must go first)
- Flat storage directories
- State schema migration (qualified → bare keys, add `sources`, `local_version`, `installed_hash`)
- `ClaudeTool.Install` bare-name symlinks
- Discovery scan simplification
- Migration of existing namespaced directories

**Phase 2: List TUI fixes** (depends on Phase 1 for dedup and actions)
- All 7 TUI fixes (actions, update, excerpt, search, alt screen, cursor, editor)
- Local modification protection during sync
- Version snapshot on update

**Phase 3: New commands** (can follow independently)
- `scribe restore <skill> <version>`
- `scribe config set editor <name>`

---

## Out of scope

- Publishing updates back to multiple registries (`scribe registry push`) — post-MVP
- Full-directory version snapshots (only SKILL.md is versioned)
- Automatic conflict resolution for genuinely different skills with same name
- Registry priority ordering in config (use sync-order for now, add explicit priority later if needed)
