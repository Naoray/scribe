# Storage Model Redesign & List TUI Overhaul

**Date:** 2026-04-10
**Status:** Draft (revised after counselor review + research)
**Builds on:** 2026-04-06-mvp-design.md

## Summary

Redesign skill storage from registry-namespaced directories to flat single-copy storage with local-first ownership, 3-way merge for upstream updates, content-hash modification detection, and version snapshots. Fix 8 list TUI issues uncovered during real usage.

## Motivation

**Claude can't discover nested skills.** The current storage model creates `~/.scribe/skills/<registry-slug>/<skill-name>/` with symlinks at `~/.claude/skills/<registry-slug>/<skill-name>`. Claude Code only scans one level deep in `~/.claude/skills/`, so nested skills are invisible — users don't get them in autocomplete. Skills also appear twice in `scribe list` (qualified + bare name via legacy symlinks).

**Local modifications get destroyed.** Users customize skills locally (e.g., tweaking `/recap`) and expect those changes to survive `scribe sync`. The current model has no modification detection or merge capability — sync silently overwrites everything.

**Versions are meaningless.** Branch refs like `main` are not versions — they're just branch names that never change from the user's perspective. SHA hashes are opaque. Users need human-readable local revision numbers.

## Core Principle: Local Machine Is Source of Truth

Scribe manages skills for the developer's machine. Registries are distribution channels, not authorities. This means:

- The developer's local copy is always authoritative
- Registry sync is a pull mechanism, never an automatic overwrite
- Local modifications are preserved by default, merged on conflict
- The maintainer of a skill is the person who created it (determined by manifest metadata), not whoever has repo write access

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
  SKILL.md                    # current working copy (may be locally modified)
  .scribe-base.md             # pristine copy from last sync (merge base)
  versions/
    v1.md                     # rollback snapshot
    v2.md
~/.claude/skills/cleanup      # flat symlink — Claude discovers this
```

### Rules

- One canonical copy per skill name in `~/.scribe/skills/<name>/`
- Symlinks to tool directories use bare names: `~/.claude/skills/<name>` → `~/.scribe/skills/<name>/`
- State tracks which registries provide each skill (multi-source)
- No registry-slug directories on disk
- Reserved names blocked as skill names: `versions`, `.git`, `.DS_Store`

### Migration

On first run after upgrade, migrate existing namespaced directories:
1. For each `~/.scribe/skills/<slug>/<name>/`, move to `~/.scribe/skills/<name>/`
2. If bare-name target already exists: compare content hashes — if identical, delete the slug copy; if different, move the slug copy to a quarantine dir (`~/.scribe/migration-conflicts/<slug>-<name>/`) for manual review, log a warning
3. Update symlinks in `~/.claude/skills/` and `~/.cursor/` to point to new flat paths
4. Remove stale registry-slug directories from tool dirs (e.g., `~/.claude/skills/Artistfy-hq/`)
5. Update state.json keys (strip slug prefix for skills that moved)
6. Remove empty slug directories from `~/.scribe/skills/`
7. Package-type entries (with `install_cmd`/`update_cmd`) have no local files to move — only their state keys are migrated from qualified to bare names
8. Add `schema_version: 2` to state.json to mark migration as complete (skip on future loads)
9. Copy current `SKILL.md` to `.scribe-base.md` for each migrated skill (establishes merge base)

---

## 2. Multi-Source State Tracking

### State schema change

```json
{
  "schema_version": 2,
  "installed": {
    "cleanup": {
      "revision": 3,
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
| `version` | Git ref string | Removed — replaced by `revision` |
| `commit_sha` | Single SHA | Removed — moved into `sources[].last_sha` |
| `source` | Single source string | Removed — replaced by `sources[]` array |
| `revision` | N/A | Monotonic integer, bumped on each sync that changes content |
| `installed_hash` | N/A | SHA-256 of SKILL.md at time of last sync/install — detects local modifications |
| `sources` | N/A | Array of registries that provide this skill |
| `schema_version` | N/A | Top-level field for idempotent migrations |

### Source lifecycle

- When a registry is disconnected (`scribe registry disconnect`): remove that registry from all skills' `sources[]`. If a skill has no remaining sources, it becomes local-only (still installed, just untracked).
- When a registry is synced and has a new skill: add source entry to existing skill or create new skill entry.
- `sources[].last_sha` is internal bookkeeping for sync comparison — never displayed to users.

### Backward compatibility

`state.Load()` already has migration logic (`parseAndMigrate`). Add a new migration step gated by `schema_version < 2`:
- If key contains `/` and matches pattern `<slug>/<name>`: convert to bare-name key, populate `sources` from old `source` field, set `revision: 1`, compute `installed_hash`

---

## 3. Merge Base & 3-Way Merge

### The `.scribe-base.md` file

Every synced skill stores a pristine copy of the last-synced SKILL.md as `.scribe-base.md` alongside the working copy. This enables 3-way merge when both local and upstream change:

```
~/.scribe/skills/cleanup/
  SKILL.md              ← working copy (may be locally modified)
  .scribe-base.md       ← pristine from last sync (merge base)
```

- On fresh install: `SKILL.md` and `.scribe-base.md` are identical
- On local modification: `SKILL.md` diverges, `.scribe-base.md` stays pristine
- On upstream update (no local mods): both files are overwritten with new content
- On upstream update (with local mods): 3-way merge triggered

### 3-Way merge via `git merge-file`

When upstream has a new version AND the skill has local modifications:

```bash
git merge-file SKILL.md .scribe-base.md /tmp/upstream-new.md
```

This produces a merged result in `SKILL.md` using Git's battle-tested merge algorithm:
- **base** = `.scribe-base.md` (what the registry gave us last time)
- **ours** = `SKILL.md` (current on disk, locally modified)
- **theirs** = new upstream content (fetched from registry)

### Merge outcomes

| Result | Exit code | Action |
|--------|-----------|--------|
| Clean merge | 0 | Auto-apply. Update `.scribe-base.md` to upstream content. Show "merged cleanly" |
| Conflict | 1 | Write conflict markers to `SKILL.md`. Prompt user: **[m]erge** (keep markers, resolve manually), **[r]eplace** (discard local, use upstream), **[s]kip** (keep local, don't update) |
| Error | >1 | Show error, skip skill |

### Conflict resolution

After merge with conflicts, `SKILL.md` contains Git-style conflict markers:
```
<<<<<<< local
Your customized instructions here
=======
Updated upstream instructions here
>>>>>>> upstream
```

The skill is marked as `StatusConflicted` until resolved. User options:
- Edit manually, remove markers → next sync detects clean state
- `scribe resolve cleanup --ours` → keep local version
- `scribe resolve cleanup --theirs` → use upstream version

### Post-merge state update

After successful merge or manual resolution:
- Update `.scribe-base.md` to the new upstream content
- Update `installed_hash` to hash of the resolved `SKILL.md`
- Bump `revision`
- Snapshot pre-merge version to `versions/`

---

## 4. Local Versioning & Rollback

### Revision counter

Each skill has a `revision` number — a local monotonic counter. It means "how many times the content changed on this machine." This is the only version number users see.

- Branch refs (`main`) are not versions — they're metadata about where content came from
- SHAs are internal bookkeeping — never shown to users
- `revision` is bumped on: sync that changes content, manual restore, merge resolution

### Display format

- `rev 3` — clean, matches last sync
- `rev 3*` — locally modified (hash differs from `installed_hash`)
- `rev 3!` — conflicted (has unresolved merge markers)

NOT `v3` — avoids confusion with semver.

### Version snapshots

Before content changes (sync, merge, restore):
1. Copy current `SKILL.md` to `versions/rev-{N}.md` (where N = current `revision`)
2. Bump `revision`
3. Write new content

### Retention

Keep last 10 version snapshots per skill. On each new snapshot, delete oldest if count exceeds 10. Configurable via `config.yaml`:

```yaml
version_retention: 10    # default, 0 = unlimited
```

### Restore

`scribe restore cleanup rev-2` — copies `versions/rev-2.md` back to `SKILL.md`. Sets `installed_hash` to new content hash (treated as "locally modified" on next sync). Shows: "Restored rev 2. This skill will be preserved during future syncs unless you run `scribe sync --force`."

---

## 5. Local Modification Protection

### Detection

At sync time, for each skill:
1. Compute SHA-256 of `SKILL.md` on disk
2. Compare to `installed_hash` in state
3. If they differ → skill was locally modified

### Sync behavior matrix

| Local mods? | Upstream changed? | Action |
|-------------|-------------------|--------|
| No | No | Skip (status: current) |
| No | Yes | Fast-forward: snapshot old, overwrite, update `.scribe-base.md` |
| Yes | No | Skip (status: modified) |
| Yes | Yes | **3-way merge** via `git merge-file` (see Section 3) |

No `--force` flag that blindly overwrites all. Instead, the merge flow always gives users control:
- Clean merges auto-apply (safe)
- Conflicts prompt for resolution (user decides)

### New statuses

Add to `sync.Status`:
```go
StatusModified    Status = 4  // installed, locally modified, upstream unchanged
StatusConflicted  Status = 5  // merge produced conflicts, needs resolution
```

| Status | Icon | Label | Color |
|--------|------|-------|-------|
| `StatusModified` | `✎` | `modified` | blue `#3B82F6` |
| `StatusConflicted` | `⚡` | `conflict` | orange `#F97316` |

---

## 6. Collision Handling

When two registries provide a skill with the same name:

### Same content

Both registries tracked as sources. No conflict. Skill shown once in list.

### Different content

**Block the install with an error.** Don't silently overwrite.

```
⚠ cleanup from sandorian/tools conflicts with existing cleanup from Artistfy/hq
  Different content detected. Options:
  - Rename one in its registry (cleanup → cleanup-files)
  - Disconnect one registry (scribe registry disconnect sandorian/tools)
  - Force replace (scribe sync --replace cleanup --from sandorian/tools)
```

Non-TTY: skip with error, never auto-resolve.

### In `scribe list`

Show skill once. Detail pane shows all registries under "Sources":
```
Sources   Artistfy/hq (main), sandorian/tools (v1.2.0)
```

---

## 7. `WriteToStore` and `ClaudeTool.Install` Changes

### `WriteToStore`

Drop `registrySlug` parameter. Skill goes directly to `~/.scribe/skills/<name>/`:

```go
func WriteToStore(skillName string, files []SkillFile) (string, error) {
    // writes to ~/.scribe/skills/<skillName>/
}
```

After writing, also write `.scribe-base.md` as a copy of `SKILL.md` (establishes merge base).

### `ClaudeTool.Install`

No code change needed — `skillName` is already bare in the new model. The syncer passes bare `sk.Name` instead of `qualifiedName`.

### Syncer changes

- State keys use bare names: `st.RecordInstall(sk.Name, ...)` not `st.RecordInstall(qualifiedName, ...)`
- `WriteToStore(sk.Name, files)` not `WriteToStore(registrySlug, sk.Name, files)`
- Source tracking: append to `sources[]` instead of storing single `source` string

---

## 8. List TUI Fixes

### 8.1 Actions fix

`listRow` gains new fields:

```go
type listRow struct {
    // ... existing fields ...
    Entry     *manifest.Entry  // from SkillStatus.Entry, nil for local-only
    LatestSHA string           // for triggering update
}
```

`actionsForRow` replaces current hardcoded-disabled logic:

| Action | Enabled when |
|--------|-------------|
| update | `row.HasStatus && row.Status == StatusOutdated && row.Entry != nil` |
| remove | `row.Local != nil && row.Local.LocalPath != ""` |
| copy path | `row.Local != nil && row.Local.LocalPath != ""` |
| open in editor | `row.Local != nil && row.Local.LocalPath != ""` |
| add to category | Always disabled (coming soon) |

### 8.2 Update action (single-skill sync)

When user selects "update" on an outdated skill:

1. Show "Updating..." status in action area
2. Create a `tea.Cmd` that:
   - Creates a `Syncer` with the bag's client, provider, and tools
   - Calls `syncer.RunWithDiff(ctx, registry, []SkillStatus{selectedSkill}, state)` — single-entry diff
   - Handles merge flow (3-way merge if local mods exist)
   - Returns `updateDoneMsg{name, err, merged, conflicted}`
3. On success (clean): update row status to `StatusCurrent`, show "Updated!" for 1 second
4. On success (merged): show "Updated! (merged with local changes)"
5. On conflict: show "Merge conflict — resolve in editor", update status to `StatusConflicted`
6. On error: show error message in status area

New messages:
```go
type updateStartMsg    struct{ name string }
type updateDoneMsg     struct{ name string; err error; merged bool; conflicted bool }
```

### 8.3 Skill excerpt in detail pane

Read first ~8 lines of SKILL.md body (after frontmatter) and display as dimmed text in the bottom section of the detail pane.

Add `Excerpt string` to `listRow`, populated during `buildRows` (cached, not re-read on every render).

Layout change for detail pane (right side):
```
┌─────────────────────────┐
│ cleanup                 │  ← skill name
│ Clean up stale code...  │  ← description
│─────────────────────────│
│ Status   current        │  ← metadata
│ Version  rev 3          │
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

### 8.4 Search visibility

Always show search indicator below the header:

- Empty state: `/ search...` (dimmed placeholder)
- Active: `/ {query}` with query highlighted
- Shows in both full and split view

Update help text: `↑↓ navigate · /search · enter detail · q quit`

The `/` prefix echoes the slash-command convention users already know.

### 8.5 Alt screen removal

Remove `v.AltScreen = true` from `View()`. The TUI renders inline below the command that was typed. Terminal history is preserved — user can scroll up to see what they typed before.

Height calculation: use `min(terminalHeight, contentNeeded)` so small skill lists don't waste vertical space. The TUI should only take as much space as it needs.

Note: TUI output stays in the terminal scroll buffer after exit. This is intentional — `scribe list` behaves like `ls`, not like `vim`.

### 8.6 Cursor overflow fix

**Problem:** In full-width view, 1 item is focusable but hidden above the visible area. In split view, ~3 items are hidden behind the header.

**Root cause:** `contentHeight()` subtracts fixed constants that don't match actual chrome size.

**Fix:**
1. Recalculate `contentHeight()` — with the new always-visible search bar, header is 3 lines (title + divider + search), footer is 3 lines (blank + summary + help). Total chrome = 6 lines.
2. In `ensureCursorVisible`, count group headers between `offset` and the visible window end as consumed lines. Algorithm: `visibleDataRows = contentHeight - groupHeadersInViewport(offset, contentHeight)`. Cursor must stay within `visibleDataRows`.

### 8.7 Editor configurable

Add `editor` field to config:

```yaml
editor: cursor    # or "code", "vim", "nvim", etc.
```

Resolution order: `config.editor` → `$VISUAL` → `$EDITOR` → `vi`

Action label: **"Open in Editor"** (not "open in $EDITOR").

New command: `scribe config set editor cursor` — writes the editor preference to `~/.scribe/config.yaml`. This introduces the `scribe config` subcommand surface.

---

## 9. Discovery Changes

### `discovery.OnDisk` — flat scan

Since skills are now flat in `~/.scribe/skills/`, the scan simplifies:
- `~/.scribe/skills/<name>/` — every directory with SKILL.md is a skill (no registry-slug nesting)
- `~/.claude/skills/<name>/` — symlinks pointing back to scribe store (deduplicate by resolved path via `filepath.EvalSymlinks`)
- Skip reserved names: `versions`, `.git`, `.DS_Store`

### Detecting local modifications

`OnDisk` computes current content hash of SKILL.md and compares against `installed_hash` in state. Add fields to `discovery.Skill`:

```go
type Skill struct {
    // ... existing fields ...
    Modified   bool   // SKILL.md hash differs from installed_hash
    Conflicted bool   // SKILL.md contains unresolved merge markers
    Revision   int    // from state
}
```

---

## 10. Scope & Sequencing

**Phase 1: Storage model + merge engine** (must go first)
- Flat storage directories
- `.scribe-base.md` merge base files
- State schema migration (qualified → bare keys, add `sources`, `revision`, `installed_hash`, `schema_version`)
- `WriteToStore` signature change (drop `registrySlug`)
- `ClaudeTool.Install` bare-name symlinks (cleanup of old nested dirs)
- Discovery scan simplification + modification detection
- Migration of existing namespaced directories (with quarantine for conflicts)
- Local modification protection during sync (hash comparison)
- 3-way merge via `git merge-file`
- Version snapshot on update + retention cap
- Collision blocking (different content, same name)

**Phase 2: List TUI fixes** (depends on Phase 1 for dedup, actions, and status)
- All 7 TUI fixes (actions, update, excerpt, search, alt screen, cursor, editor)
- `StatusModified` and `StatusConflicted` display
- Single-skill update with merge support
- `scribe config set editor` command

**Phase 3: Resolution & restore commands** (can follow independently)
- `scribe restore <skill> <revision>`
- `scribe resolve <skill> [--ours|--theirs]`

---

## 11. Contradictions with MVP Spec (to reconcile)

1. **`scribe remove` uses qualified names in MVP spec.** With bare keys, remove takes bare names: `scribe remove cleanup`. To disconnect one source without removing the skill: `scribe registry disconnect <repo>` (removes that registry from all skills' sources).

2. **`WriteToStore` signature.** MVP spec says `WriteToStore(registrySlug, skillName, files)`. This spec drops `registrySlug`. MVP spec must be updated.

3. **Canonical store path.** MVP spec says `~/.scribe/skills/<registry-slug>/<name>/`. This spec says `~/.scribe/skills/<name>/`. MVP spec must be updated.

---

## Out of scope

- Publishing updates back to registries (`scribe registry push`) — post-MVP
- Full-directory version snapshots (only SKILL.md is versioned for now)
- Project-level skill scoping (`.scribe/` in project dir to limit which skills are active) — future, useful for reducing token waste
- Registry priority ordering in config — not needed since collisions block rather than auto-resolve
- `rerere`-style conflict resolution replay — future optimization
- Maintainer push flow — post-MVP
