# Sync As System Reconcile

**Date:** 2026-04-11
**Status:** Draft
**Builds on:** 2026-04-10-storage-and-list-tui-design.md

## Summary

Reposition `scribe sync` from "sync connected registries" to "keep the machine in sync." `sync` should reconcile registry-backed skills, adopted local skills, and tool-facing installs into one consistent local system state.

This closes the current gap where Scribe manages a skill in `~/.scribe/skills/<name>/` but does not heal missing or drifted tool installs unless the user explicitly runs `scribe adopt` again. It also fixes Codex autocomplete drift by making `~/.codex/skills/<name>` part of normal sync reconciliation rather than a best-effort side effect of the original install.

## Goals

1. Make `scribe sync` the one command users run to restore a healthy machine state
2. Heal missing per-tool skill installs for both registry and adopted skills
3. Detect duplicate or drifted unmanaged copies in tool directories during normal sync
4. Preserve user data by refusing to auto-overwrite divergent unmanaged content
5. Ensure Codex-visible installed skills actually appear under `~/.codex/skills/`

## Non-Goals

- Auto-merging divergent tool-local skill copies into the canonical store
- Three-way merge across multiple unmanaged tool copies
- Background or daemon-based reconciliation outside explicit `scribe sync`
- Reworking package install behavior in this pass

---

## 1. Product Positioning

`scribe sync` becomes the machine-health command.

Today, users need to understand several separate concepts:
- registry sync updates managed skills
- adopt claims unmanaged skills
- restore rewinds canonical content
- missing tool symlinks may require manual repair

That is the wrong mental model. The correct product behavior is: if Scribe believes a skill should exist on this machine, `scribe sync` should make that true everywhere Scribe manages, unless doing so would destroy user-authored divergent content. In that case, `sync` should stop short, preserve the content, and clearly report the conflict.

This aligns with the repo north star in `CLAUDE.md`:
- convenience first
- one command that fixes the machine
- minimal output, but high-signal next actions when intervention is required

## 2. Canonical State Model

The canonical source of truth remains:

1. `~/.scribe/skills/<name>/` for skill content
2. `state.Installed[name]` for ownership, origin, tools, hashes, and upstream metadata

Tool-facing paths such as `~/.claude/skills/<name>` and `~/.codex/skills/<name>` are derived state. They must be treated as reconcilable projections of the canonical store, not as independent authorities.

Implications:

- Missing tool links are repairable drift, not a user problem
- Same-content unmanaged duplicates are normalization opportunities, not conflicts
- Different-content unmanaged duplicates are conflicts because they compete with the canonical projection

## 3. New Sync Responsibility

`scribe sync` should run three phases:

1. **Adoption scan**
   Detect unmanaged skills that are not yet in Scribe state and offer or auto-adopt them according to `adoption.mode`
2. **System reconcile**
   For every installed skill already tracked in state, ensure tool-facing installs match the expected projection from state
3. **Registry reconcile**
   Pull upstream registry changes into the canonical store using the existing sync/update logic

This is a change in emphasis, not in user-facing command count. The user still runs `scribe sync`, but it now means "make my system correct."

### Ordering

The preferred order is:

1. resolve active tools
2. adoption scan/import for unmanaged skills
3. local system reconcile for all state-managed skills
4. upstream registry sync/update
5. final local system reconcile for any newly installed or tool-changed skills

The second reconcile pass matters because registry sync may install new skills, change effective tools, or refresh canonical content. Ending with reconcile ensures the machine is correct after all mutations, not just midway through the run.

## 4. Reconcile Semantics

Introduce a reconcile engine driven from `state.Installed`.

For each installed skill:

1. Resolve its effective target tools from `InstalledSkill.Tools`, `ToolsMode`, config tool enablement, and tool availability
2. Compute the expected canonical dir: `~/.scribe/skills/<name>/`
3. For each expected tool path:
   - if missing: install the symlink or link projection
   - if already points to canonical content: no-op
   - if it is a same-hash real copy: replace with the canonical link
   - if it is different content: emit a reconcile conflict and leave it untouched
4. For each recorded-but-no-longer-expected tool path:
   - uninstall it unless doing so would remove divergent unmanaged content not created by Scribe

The default posture is:
- heal silently when safe
- never overwrite divergent content silently

### Same-Hash Normalization

If a tool path contains a real directory or file whose effective skill hash matches the canonical store, it is not a meaningful fork. Reconcile should remove it and reinstall the proper symlink.

This covers:
- a user copying a skill directory into `~/.codex/skills`
- a broken symlink replaced by a real directory with the same content
- tools that temporarily materialized a real copy during manual edits and then drifted back to canonical content

### Divergent Conflict

If a tool path for a Scribe-managed skill exists with different content from the canonical store, reconcile should emit a conflict and preserve the path unchanged.

Default sync must not choose winners automatically. The canonical store may be right, or the unmanaged copy may be a user’s unrecorded customization. Overwriting it would violate the local-first trust model.

## 5. Conflict Model

There are now two conflict classes during `sync`:

### A. Adoption conflicts

An unmanaged skill name collides with an existing state entry during the adoption scan. This already exists today and remains unchanged in spirit:
- same hash: treat as re-link/normalize
- different hash: report conflict

### B. Reconcile conflicts

A skill already tracked in state encounters divergent content in an expected tool path. This is new.

The conflict payload should include:
- skill name
- tool name
- expected canonical path
- found path
- canonical hash
- found hash
- whether the found content is a symlink, file, or directory

TTY output should stay compact:

```text
conflict: recap in codex differs from managed copy
run `scribe adopt recap` to inspect/resolve
```

`--verbose` and `--json` can include hashes and paths.

## 6. Codex Installation And Autocomplete

Codex skill autocomplete should be treated as a direct outcome of reconciliation, not a separate install concern.

Codex’s installed-skill path is `~/.codex/skills/<name>`. If a skill is expected to be installed to Codex, `sync` must guarantee that path exists and resolves to the canonical store.

This explicitly does **not** treat `~/.codex/superpowers/skills/` as an installed-skill location. That path can contain source material or sidecar assets, but it is not what Codex indexes for skill discovery. Skills only count as installed for Codex if they reconcile into `~/.codex/skills/`.

Result:
- if a superpowers skill is state-managed and Codex is an effective tool target, `sync` will recreate the missing `~/.codex/skills/<name>` link
- if a different real directory appears at that path, `sync` will report a conflict instead of silently hiding the issue

## 7. Relationship Between `adopt` And `sync`

`scribe adopt` remains useful, but its role narrows:

- `sync` is the automatic system reconciler
- `adopt` is the explicit intake and conflict-resolution command for unmanaged skills

That means:
- `sync` should still run the adoption scan
- `adopt` should remain the best place to inspect, preview, and resolve unmanaged-skill conflicts intentionally
- drift detection for already-managed skills must no longer depend on the user remembering to run `adopt`

In other words: `adopt` becomes a focused maintenance command, not a prerequisite for system health.

## 8. State And API Changes

This design does not require a new ownership model, but it does require explicit reconcile data structures and events.

### New reconcile result types

Add a small reconcile package or sync submodule with concepts like:

```go
type ActionKind string

const (
    ActionInstalled   ActionKind = "installed"
    ActionRelinked    ActionKind = "relinked"
    ActionRemoved     ActionKind = "removed"
    ActionConflict    ActionKind = "conflict"
    ActionUnchanged   ActionKind = "unchanged"
)

type SkillConflict struct {
    Name          string
    Tool          string
    ExpectedPath  string
    FoundPath     string
    CanonicalHash string
    FoundHash     string
    FoundType     string
}
```

These should be emitted as workflow events so the existing formatter layer can keep UI concerns out of core logic.

### State updates

When reconcile heals installs, `state.Installed[name].Paths` and `Tools` should be refreshed to reflect the actual installed projections. This keeps later uninstall and display behavior correct.

No new persisted field is required for v1 of reconcile if the effective tool set can still be derived from existing `Tools`, `ToolsMode`, and config.

## 9. Algorithm Sketch

### Reconcile expected tools

For each `InstalledSkill`:

1. Resolve effective tools using existing tool-resolution logic
2. Build `expectedTools[name]`
3. Skip package skills for now or leave them to package-specific logic

### Inspect current tool paths

For each expected tool:

1. Ask the tool for `SkillPath(name)`
2. Inspect the path:
   - nonexistent
   - symlink to canonical
   - symlink elsewhere
   - file
   - directory
3. Resolve effective content hash where possible:
   - use `SKILL.md` hash for directories
   - if path resolves directly to `SKILL.md`, hash that file

### Apply safe repairs

- nonexistent -> `tool.Install`
- symlink elsewhere but same canonical hash -> remove and `tool.Install`
- file/dir same hash -> remove and `tool.Install`
- different hash -> conflict event, no write

### Remove stale installs

For tool paths recorded in state but not in the effective tool set:
- if path is the expected canonical projection, uninstall it
- if path now contains divergent content, leave it and emit a warning/conflict rather than deleting user material

## 10. Output Design

Default sync output should stay terse.

Good default summary:

```text
syncing system...
  repaired 3 tool installs
  1 conflict skipped
syncing registries...
  17 skills up to date
```

If nothing needed repair, omit the line entirely. If only Codex links were recreated, do not call out Codex specifically unless `--verbose` is on. The user cares that the system is healthy, not which internal phase happened.

`--json` should include:
- repaired installs count
- relinked installs count
- removed stale installs count
- reconcile conflicts array

## 11. Testing

Add table-driven tests covering:

1. missing Codex skill path for a managed skill -> relinked on sync
2. same-hash real directory in a tool path -> replaced with symlink
3. different-hash real directory in a tool path -> conflict, no overwrite
4. adopted local skill with `OriginLocal` still reconciles during sync
5. disabled or unavailable tool is not force-installed
6. stale tool path is removed when no longer expected
7. stale tool path with divergent content is preserved and reported

Workflow tests should verify ordering:
- adoption scan before registry sync
- system reconcile after adoption
- final reconcile after registry install/update

## 12. Migration And Rollout

This should ship without state migration.

Existing users benefit automatically on next `scribe sync`:
- missing `~/.codex/skills/*` links are recreated
- same-hash duplicates are normalized
- divergent copies are surfaced as conflicts instead of silently ignored

No new command is required. Documentation should update `scribe sync` to mean:

> Keep connected registries, local skills, and installed tool projections in sync.

## 13. Open Questions Deferred

These are intentionally out of scope for this pass:

1. conflict-resolution flags such as `scribe sync --prefer-managed`
2. auto-merge between canonical and divergent unmanaged tool copies
3. a dedicated `scribe doctor` or `scribe repair` command
4. package reconcile behavior for tools that do not use per-skill symlinks

The design should leave room for a future explicit conflict-resolution workflow, but default sync behavior should already satisfy the north star: one command that keeps the machine healthy without surprising data loss.
