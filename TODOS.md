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

## Parallelize multi-registry API calls in `scribe add` and `scribe registry add`

**What:** `discoverEntries` (cmd/add.go) and the `otherManifests` fetch loop in `runRegistryAdd` (cmd/registry_add.go) make N sequential GitHub API calls. Parallelize with `errgroup`.

**Why:** With several connected registries, `scribe add` blocks for N round-trips before the TUI appears. Each `syncer.Diff` call is independent.

**Fix:** Replace the sequential loops with `golang.org/x/sync/errgroup` + goroutines, merging results after all calls complete.

**Context:** Identified during PR #69 review (2026-04-07).

---

## Add test coverage for new `scribe add` / install browser functions

**What:** Zero test coverage for: `parseSkillRef`, `filterEntries`, `sortEntries`, `isPackageManifestMissingErr`, `collectInstallCommands`, and `installModel` TUI helpers (`filteredItems`, `selectedCount`, `selectedEntries`, viewport math).

**Why:** Pure functions with multiple guard clauses, no tests. `isPackageManifestMissingErr` has brittle string-matching that especially needs exercising.

**Fix:** Table-driven tests in `cmd/add_test.go` and `cmd/install_tui_test.go`, mirroring existing `list_tui_test.go` style. Use `t.Setenv("HOME", t.TempDir())` for filesystem isolation.

**Context:** Identified during PR #69 review (2026-04-07).

---

## Replace `isPackageManifestMissingErr` string matching with sentinel errors

**What:** `isPackageManifestMissingErr` in `cmd/registry_add.go` detects error types by matching substrings of `err.Error()`. Any wording change in `internal/add` silently breaks the fallback-to-prompt path.

**Fix:** Define typed errors in `internal/add` (`ErrNoPackageManifest`, `ErrNotAPackage`, `ErrNoInstallCommands`) and replace string matching with `errors.Is`. TODO comment already added at the call site.

**Context:** Identified during PR #69 review (2026-04-07).

---

## TUI: selected-but-filtered items install silently

**What:** In the install browser, items selected before a search filter is applied remain selected when hidden by search. Pressing enter installs them. A user can select 5 items, search-narrow to 0 visible, and get all 5 installed.

**Fix:** Show a "N selected (X hidden)" indicator in the footer, or restrict installs to items visible in the current filter.

**Context:** Identified during PR #69 review (2026-04-07).

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
