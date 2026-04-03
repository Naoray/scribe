# TODOS

## Multi-registry sync/list UX

**What:** When multiple registries are connected, `scribe sync` and `scribe list` should support registry selection.

**Why:** With a single registry the behavior is obvious. With multiple, the user needs control over which registry to sync from or list. Without this, commands silently operate on all registries which may not be what they want.

**Behavior:**
- If one registry connected: proceed as today (no prompt)
- If multiple registries connected + no flag: prompt user to select one (huh select)
- `--all` flag: operate on all connected registries

**Pros:** Clear UX, no surprises, consistent with how tools like `git remote` work.

**Cons:** Adds a prompt to every sync/list run for multi-registry users unless they always use `--all`.

**Context:** Decision made during eng review of `scribe connect` (2026-03-28). The `huh` library is being added for `scribe connect`'s interactive prompt, so the select UI is already available.

**Depends on:** `scribe connect` multi-registry config landed.

---

## Registry column in `scribe list`

**What:** Add a REGISTRY column to `scribe list` output showing which registry each skill came from. Highlight name conflicts (same skill name from different registries).

**Why:** With multiple registries connected, a flat skill list with no source attribution is confusing. Users need to know where each skill came from, especially when there are conflicts.

**Behavior:** Table gains REGISTRY column. Name conflicts shown with a warning indicator.

**Pros:** Transparency, easier debugging, supports `--registry` filter flag.

**Cons:** Wider table output, more complex list rendering.

**Context:** Flagged during eng review 2026-03-28. Requires `registry` field in state.json InstalledSkill (see below).

**Depends on:** Registry tracking in state.json.

---

## Registry source tracking in state.json

**What:** Add a `registry` field to `InstalledSkill` in state.json.

**Why:** With multi-registry support, re-sync needs to know which registry to check for updates for each installed skill. Without this, every sync checks all registries for every skill.

**What to add:**
```json
"gstack": {
  "registry": "vercel/skills",
  "version": "v0.12.9.0",
  ...
}
```

**Pros:** Enables precise per-registry diffs, list attribution, and update tracking.

**Cons:** State migration needed for existing installs (old entries have no registry field — treat as belonging to the first configured registry, or flag as unknown).

**Context:** Flagged during eng review 2026-03-28.

**Depends on:** `scribe connect` multi-registry config landed.

---

## Guide command: table-driven path dispatch

**What:** Replace the options-list + switch-case pair in `runGuideInteractive` with a single `[]guidePath` slice that co-locates each path's label, availability predicate, and handler.

**Why:** Currently the huh options list and the `switch chosen` must be kept in sync manually — add a path in one place, forget the other. A table-driven slice eliminates that duplication.

**When:** When a fourth guide path is actually needed. Three paths is fine with a switch.

**Context:** Identified during PR #37 review (2026-04-03). Deliberately deferred — not worth doing for three cases.

---

## `scribe create registry --existing <owner/repo>` (initialize registry in existing repo)

**What:** Allow `scribe create registry` to scaffold a `scribe.toml` + `skills/` folder inside an existing GitHub repo instead of always creating a new one.

**Why:** Teams already have repos (e.g., a team hub, monorepo, or docs repo) where they want to add a skill registry. Currently the only option is to manually push the manifest and then `scribe connect`. The CLI should handle this end-to-end.

**Behavior:**
- `scribe create registry --existing Artistfy/hq` — clones (or fetches) the repo, creates `scribe.toml` + `skills/` on a branch, pushes, opens PR (or commits directly), then auto-connects
- Interactive mode: prompt for repo selection from user's orgs/repos if no `--existing` flag
- Detect if `scribe.toml` already exists and abort with message

**Context:** User tried to use `Artistfy/hq` as a registry (2026-04-03). Had to fall back to manual setup because the CLI only supports creating new repos.

**Depends on:** Nothing blocking.

---

## `scribe init` (package author mode)

**What:** Scaffold a new skill package in the current directory — creates `scribe.toml` with `[package]` section, detects existing SKILL.md files, prompts for name/description/author.

**Why:** The "publish your own skills" workflow. Needed for anyone who wants to create a skill package that others can install via Scribe.

**Context:** Originally planned as part of `scribe init` before the command was split. `initCmd` is currently removed from root.go. Re-register when implemented.

**Depends on:** Nothing blocking.
