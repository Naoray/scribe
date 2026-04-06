# TODOS

## ~~Multi-registry sync/list UX~~ **Completed:** PR #21 (2026-03-29)

`--registry` flag on sync/list, `--all` flag, grouped output by registry.

---

## ~~Registry column in `scribe list`~~ **Completed:** PR #21 (2026-03-29)

Skills grouped by registry in list output. `--registry` filter flag available.

---

## ~~Registry source tracking in state.json~~ **Completed:** PR #21 (2026-03-29)

`Registries []string` field added to `InstalledSkill`. `AddRegistry()` / `RemoveRegistry()` methods. Backfill migration via `MigrateRegistries()`.

---

## Guide command: table-driven path dispatch

**What:** Replace the options-list + switch-case pair in `runGuideInteractive` with a single `[]guidePath` slice that co-locates each path's label, availability predicate, and handler.

**Why:** Currently the huh options list and the `switch chosen` must be kept in sync manually — add a path in one place, forget the other. A table-driven slice eliminates that duplication.

**When:** When a fourth guide path is actually needed. Three paths is fine with a switch.

**Context:** Identified during PR #37 review (2026-04-03). Deliberately deferred — not worth doing for three cases.

---

## `scribe create registry --existing <owner/repo>` (initialize registry in existing repo)

**What:** Allow `scribe create registry` to scaffold a `scribe.yaml` + `skills/` folder inside an existing GitHub repo instead of always creating a new one.

**Why:** Teams already have repos (e.g., a team hub, monorepo, or docs repo) where they want to add a skill registry. Currently the only option is to manually push the manifest and then `scribe connect`. The CLI should handle this end-to-end.

**Behavior:**
- `scribe create registry --existing Artistfy/hq` — clones (or fetches) the repo, creates `scribe.toml` + `skills/` on a branch, pushes, opens PR (or commits directly), then auto-connects
- Interactive mode: prompt for repo selection from user's orgs/repos if no `--existing` flag
- Detect if `scribe.toml` already exists and abort with message

**Context:** User tried to use `Artistfy/hq` as a registry (2026-04-03). Had to fall back to manual setup because the CLI only supports creating new repos.

**Depends on:** Nothing blocking.

---

## `scribe init` (package author mode)

**What:** Scaffold a new skill package in the current directory — creates `scribe.yaml` with `package:` section, detects existing SKILL.md files, prompts for name/description/author.

**Why:** The "publish your own skills" workflow. Needed for anyone who wants to create a skill package that others can install via Scribe.

**Context:** Originally planned as part of `scribe init` before the command was split. `initCmd` is currently removed from root.go. Re-register when implemented.

**Depends on:** Nothing blocking.
