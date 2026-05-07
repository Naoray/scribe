# TODOS

## Install all skills from a registry

**What:** Add a first-class flow to install every installable skill from a connected registry in one command, instead of requiring per-skill installs.

**Why:** Once `browse` is the remote catalog surface, users will reasonably expect a bulk action for “bring me everything from this registry” during first-time setup or when trialing a curated registry.

**Fix:** Add an explicit registry-wide install command or browse action that resolves the selected registry, filters out already-installed/current entries, presents a confirmation summary, then installs the remaining skills through the existing sync/install path.

**Context:** Requested during browse/list UX follow-up (2026-04-15).

---

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
- `scribe create registry --existing Artistfy/hq` — clones (or fetches) the repo, creates `scribe.yaml` + `skills/` on a branch, pushes, opens PR (or commits directly), then auto-connects
- Interactive mode: prompt for repo selection from user's orgs/repos if no `--existing` flag
- Detect if `scribe.yaml` already exists and abort with message

**Context:** User tried to use `Artistfy/hq` as a registry (2026-04-03). Had to fall back to manual setup because the CLI only supports creating new repos.

**Depends on:** Nothing blocking.

---

## `scribe init` (package author mode)

**What:** Scaffold a new skill package in the current directory — creates `scribe.yaml` with `package:` section, detects existing SKILL.md files, prompts for name/description/author.

**Why:** The "publish your own skills" workflow. Needed for anyone who wants to create a skill package that others can install via Scribe.

**Context:** Originally planned as part of `scribe init` before the command was split. `initCmd` is currently removed from root.go. Re-register when implemented.

**Depends on:** Nothing blocking.

---

## `scribe registry remove` command

**What:** Inverse of `scribe registry add`. Remove a catalog entry from a team registry's `scribe.yaml`. Should work for both skill entries and package entries (e.g. `scribe registry remove superpowers --registry artistfy/hq`).

**Why:** Today, removing an entry requires hand-editing the manifest on GitHub (or via `gh api`). The CLI has an `add` half but no `remove` half.

**Fix:** New subcommand in `cmd/registry_remove.go`. Fetch manifest, drop the entry from `catalog`, push back. Same auth/TTY plumbing as `registry add`. Add a `--no-interaction` flag for non-interactive use.

**Context:** Identified while removing `obra/superpowers` from `artistfy/hq` manually (2026-04-07).

---

## `scribe registry add`: paste keyboard shortcut doesn't work in install-command prompts

**What:** When `scribe registry add owner/repo` falls through to the per-tool install command prompts (because the upstream package has no `scribe.yaml` or no declared installs), `ctrl+v` / `cmd+v` paste does not work in the Huh input fields.

**Why:** Bubble Tea raw-mode terminals must forward paste events explicitly. Huh inputs likely aren't receiving `tea.PasteMsg` — or bracketed paste isn't being requested on the program.

**Fix:** Investigate the Huh standalone `NewInput().Run()` path in `collectInstallCommands` (cmd/registry_add.go). May need to enable bracketed paste mode or switch to a full `huh.NewForm(...).RunWithContext(...)` that handles `PasteMsg` correctly. Verify against charm.md rule: "use `.Content`, not string(msg)" on `PasteMsg`.

**Context:** Observed 2026-04-07 while trying to paste an install command into the prompt.

---

## `scribe registry add`: only prompts for claude + cursor, then no output after submit

**What:** Two separate bugs in the per-tool install command flow of `scribe registry add owner/repo`:

1. The prompt only iterates `claude` and `cursor` even if other tools are active/configured. Tool list appears hard-coded instead of derived from the caller's active `tools.Tool` set.
2. After the user answers both prompts, the command produces no output — no success message, no error, no JSON result. The entry may or may not have been pushed; the user has no feedback.

**Why:** Silent success is a worse UX failure than a loud error. And hard-coding the tool list means adding a new tool (e.g. `aider`, `copilot`) won't automatically show up in the prompt loop.

**Fix:**
- Derive the prompted tool list from the resolved `targets []tools.Tool` (same list `sync` uses), not a literal slice.
- Wire the `SkillAddingMsg` / `SkillAddedMsg` events through the same formatter as the skill-add path so the success line renders. Check `wireAddEmit` / `finishAdd` in `cmd/registry_add.go` — the package-ref branch may be skipping the emit or the final `finishAdd` call.

**Context:** Observed 2026-04-07 while testing `scribe registry add obra/superpowers --registry artistfy/hq`.

---

## Sharable snippets — portable behavior directives

**What:** New content type alongside skills. Snippets are excerpts from `~/.claude/CLAUDE.md` (or equivalent) that steer agent behavior — things like commit discipline, output style, review standards, caveman mode.

**Why:** Skills add *capability* (do X). Snippets add *behavior* (do X *this way*). Users craft useful agent behavior rules in their global CLAUDE.md but have no way to share them. Snippets make these first-class Scribe artifacts — installable, versionable, sharable via registries.

**Behavior:**
- Snippets live in `~/.scribe/snippets/` (or similar)
- `scribe add` distinguishes skill vs snippet
- Snippets get **injected into** config files (CLAUDE.md, .cursorrules, etc.) rather than symlinked as standalone files
- Ties into "sync rules across LLMs" — same snippet, different target files per tool

**Context:** Idea captured 2026-04-10.
