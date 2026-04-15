# List: Local-First + Command Palette + Scribe-Agent Bootstrap

**Date:** 2026-04-15
**Status:** Design — counselor-reviewed (claude-opus + codex-5.3-high, 2 rounds)
**Related:** 2026-04-11-scribe-agent-skill-design.md, 2026-04-11-skill-adoption-design.md

## Problem

`scribe list` currently fetches every connected registry at startup:

1. Shows a "new built-in registries available" banner printed from `PersistentPreRunE` (`cmd/root.go:58`) that cannot be gated on `--json` because it runs before subcommand flag parsing.
2. Calls `syncer.Diff` per registry from `buildRows` (`cmd/list_tui.go:216`). A broken registry (e.g. `openai/codex-skills` has no manifest) used to abort the whole command; now it surfaces as a warning that bleeds past the right edge of the terminal.
3. Even after the warning fix, the default view is noisy: users asking "what's installed?" get registry diffing, network I/O, and auth token lookups they never requested.

Separately, Scribe is pivoting toward AI agents as the primary audience (per MemPalace `project_agentic_first_pivot`). An agent invoking `scribe` on a fresh machine gets zero skills, including the scribe-agent entry-point skill that would teach it how to use Scribe. The current answer is "run `scribe add Naoray/scribe`" — but an agent does not know that yet.

## Goals

- `scribe list` is instant, offline, and local-only by default. No network, no registry fetches, no shell-outs for tool detection.
- Registry interaction is explicit — users signal install intent to cross the local/remote boundary.
- Scribe-agent skill ships embedded in the binary so every user, new or upgraded, has it without a network round-trip.
- The `list_tui.go` file gets less tangled, not more, as part of this work.

## Non-goals

- Rewriting the sync engine or the provider chain.
- Changing the `list --json` schema. Default JSON is already local-only; `--remote` still emits `{registries, warnings}`.
- A full VS-Code-style command palette inside `list_tui` (see Phase 2 alternatives below).
- Catalog caching for offline registry browsing (tracked separately in `project_local_registry_index`).

## North-Star Alignment

- **Convenience first:** instant local `list` removes the top reported friction today.
- **Show exactly what's useful:** registries disappear from the default view; banners move behind first-run only.
- **Best-designed CLI:** wrapped error warnings, right-sized empty state, explicit opt-in for remote browsing, footer hints that earn their line.
- **Agent-first:** the agentic-skill is bootstrapped without a network call, so `scribe` always works for an LLM that just woke up.

---

## Phase 1 — `list` goes local-first

Ship as **two commits**, not one, so each reviewable unit stays small.

### Commit 1 — `buildRows` honors `RemoteFlag`

- `cmd/list_tui.go:216`, top of `buildRows`: if `!bag.RemoteFlag`, return `buildLocalRows(localSkills, bag.State), nil, nil` immediately. The registry path is already well-tested as the `--remote` branch; we just stop entering it by default.
- Loading spinner copy at `list_tui.go:1001` collapses to `"Loading skills…"` — the `RemoteFlag ? "Fetching team skills…" : "Discovering local skills…"` split no longer carries weight when the default is local.
- No footer-hint change yet. We do not advertise `:add` until Phase 2 actually ships.

### Commit 2 — split the load path + move the banner

Two concerns surfaced by counselors that must not wait for Phase 2:

1. **`StepLoadConfig` + `ResolveTools` defeat "instant offline list" even after the `RemoteFlag` fix.** Token resolution runs `gh auth token` with a 5s timeout (`internal/github/client.go:45`); custom-tool detection can execute shell commands (`internal/tools/runtime.go:134`, `internal/tools/command.go:62`). A user running `scribe list` with no network still pays those costs.
2. **Builtins banner prints from `PersistentPreRunE` (`cmd/root.go:58`) before subcommand flags parse.** It cannot be gated on `--json`, so agents calling `scribe list --json` get stderr noise after any upgrade that adds a builtin.

Changes:

- Introduce `ListLoadStepsLocal() []Step` in `internal/workflow/list.go` containing only `LoadConfig` (in a lightweight mode that does not eagerly build a GitHub client) and `LoadState`. The current `ListLoadSteps()` becomes `ListLoadStepsRemote()` for the `--remote` path and keeps `ResolveTools`.
- Add a `Config.LazyGitHub` (or equivalent) internal flag read by `StepLoadConfig`. When true, the GitHub client is constructed lazily on first use rather than during load. The local-list path never touches it.
- In `PersistentPreRunE`, gate the "new built-ins available" banner on `isFirstRun` (i.e. only print when `ApplyBuiltins` actually wrote a brand-new registry for the very first time, not every time a new builtin is appended to an existing config). The cleanest implementation: have `ApplyBuiltins` return `(added []string, firstRun bool)`; print only when `firstRun` is true. Long-term, move non-first-run user-facing output into each command's `RunE` so it can respect `--json`.

### Config schema addition (Phase 1, no-op)

Add the scaffolding Phase 3 depends on now, so Phase 3 is a pure wiring change:

```yaml
# ~/.scribe/config.yaml
scribe_agent:
  enabled: true        # default true; set false to opt out of embedded bootstrap
```

Field is read by nothing in Phase 1. Included here only so users on older scribe versions pick up the default without a one-shot migration when Phase 3 lands.

### Tests

- `TestListDefaultIsLocalOnly` — run `runList` with no flags against a fake provider that panics on `Discover`. Asserts the provider is never called.
- `TestListRemoteFlagStillFetches` — `--remote` still hits the registry path. Verifies the old behavior survives behind the explicit flag.
- `TestListLocalPathSkipsGitHubClient` — `bag.Client` is nil after `ListLoadStepsLocal`. Proves no token resolution, no `gh auth token` invocation.
- `TestApplyBuiltinsBannerOnlyOnFirstRun` — second invocation after a builtin is appended does not print.
- `TestConfigRoundtripScribeAgentEnabled` — config with `scribe_agent.enabled: false` survives load/save without losing the field.

---

## Phase 2 — command palette (deferred shape)

Counselors converged against embedding a full palette state machine into `list_tui.go`. The file is 1050+ lines with four tangled responsibilities (data loading, state machine, rendering, actions) and hard-coded test contracts around type-as-filter behaviour (`TestUpdateDetail_FocusListTypingFiltersRows` at `list_tui_test.go:319`). Adding a palette grammar to the same input loop is high-regression risk.

We reserve `:` as the command prefix (the only delimiter that does not collide — `/` is search, `>` is add_tui's search prompt) but defer the in-list parser. **Ship both shapes** — each serves a different primary audience:

### Shape A (ships first) — `scribe browse` top-level command

**Primary audience: AI agents + scripts.**

- New Cobra command that elevates the existing `add_tui` browse surface into a first-class command.
- `scribe browse` opens the interactive browser when invoked in a TTY. `scribe browse --json` returns the full cross-registry catalog as structured output — the agent-facing path.
- `scribe browse --query foo` filters non-interactively, `scribe browse --install <name>` short-circuits straight to install (mirroring `scribe add` but with the explicit "I want to see what's available first" semantic).
- Agents prefer this over `:add` inside `list_tui` because a Cobra command is stable, flag-documented, machine-invocable, and JSON-scriptable. A palette embedded in a TUI is invisible to an LLM.
- Pros: single source of truth for remote browse/install; no new TUI state machine; stable agent surface; tests are normal Cobra tests not Bubble Tea harness tests.
- Cons: a human reviewing their list has to quit and re-invoke to browse — solved by Shape B.

### Shape B (ships second) — `:` command prefix inside `list_tui`

**Primary audience: humans running the interactive list.**

- Typing `:` in the list search line (only when `m.search == ""`) enters command mode with a mode pill on the left.
- `:add <query>` invokes `tea.ExecProcess` to shell out to `scribe browse --query <query>` (Shape A's command) with the user's TTY. When it exits, control returns to `list` and the row count refreshes.
- `:remove <name>`, `:sync`, `:help` follow the same pattern — delegate to the real Cobra command via `tea.ExecProcess`.
- No second TUI embedded; `list_tui` stays a projection layer on top of Cobra commands.
- Documented trade-off: users cannot search for skills whose names start with `:` when search is empty.

### Why both

The CLI has two audiences and they want opposite things:

- An **agent** reads stdout and invokes commands. It needs `scribe browse --json` as a stable Cobra endpoint. A TUI palette is useless to it.
- A **human** is already in `scribe list` looking at their skills. They want `:add foo` without quitting the TUI.

Shipping only Shape A forces humans to mode-switch. Shipping only Shape B forces agents to parse an interactive TUI. Both are cheap once the data-loader extraction (see below) lands, because Shape B is ~80 LOC of `ExecProcess` plumbing on top of Shape A.

### Ship order inside Phase 2

1. Extract `buildRows` / `buildLocalRows` into `internal/workflow/list_load.go` (mechanical, no behaviour change).
2. Ship `scribe browse` (Shape A) with both TTY and `--json`/`--query`/`--install` modes. This is the agent-facing deliverable.
3. Add `:` command prefix (Shape B) that shells to `scribe browse` via `tea.ExecProcess`. Pure sugar on top of step 2.

### Hard constraints on palette grammar

- No flags inside palette commands. `:add foo` works; `:add --registry bar foo` does not. The moment a user needs flags, they use the real Cobra command.
- No palette-only actions. Every palette command maps 1:1 onto an existing top-level Cobra command.
- Palette never fetches registries eagerly. `:add <query>` only fetches when the user confirms the query.

### Extract before expand

Before Phase 2 starts, pull data loading out of `list_tui.go` into `internal/workflow/list_load.go`. `buildRows`, `buildLocalRows`, `buildLocalRowsExcluding` (`list_tui.go:216–370`) are already UI-agnostic and do not import bubbletea. This is a mechanical refactor with no behaviour change, makes Phase 2 land on a smaller surface, and is reviewable on its own.

---

## Phase 3 — `EnsureScribeAgent` (embedded bootstrap, never network)

### Shape

- New package `internal/agent/embed.go`:
  - `//go:embed scribe_agent/SKILL.md` captures the agentic skill at build time from a checked-in copy of the canonical `Naoray/scribe@<pinned-tag>` SKILL.md. Build script copies it from the scribe-agent repo during release; the embedded bytes are the single source of truth.
  - Exports `EmbeddedSkillMD []byte` and `EmbeddedVersion string` (matched tag or commit SHA).
- New function `bootstrap.EnsureScribeAgent(store string) error`:
  1. Honor `config.ScribeAgent.Enabled` (default true). If false, return nil.
  2. Stat `~/.scribe/skills/scribe-agent/SKILL.md`. If present and hash matches `EmbeddedVersion`, return nil — the fast path for every subsequent invocation.
  3. If missing or stale, `os.WriteFile` the embedded bytes. Record in state with `Origin=OriginBootstrap` (new origin kind) and pinned `Version=EmbeddedVersion`.
  4. On any error, log to stderr once per session and continue. Never block the command.

### Where it runs

**Not in `PersistentPreRunE`.** That path runs on every command, including `version` and `help`, and counselors correctly flagged that escalating side effects there is a trust-boundary change.

Instead, add `EnsureScribeAgent` as a step at the end of `ListLoadStepsLocal()`, `ListLoadStepsRemote()`, and `SyncLoadSteps()`. Commands that should not trigger bootstrap (version, help, init scaffolding) do not include it. This is more selective than pre-run and keeps the side effect scoped to commands where the skill is actually useful.

### Refresh (explicit, user-initiated)

- `scribe upgrade-agent` command: fetches the latest `Naoray/scribe@<tag>` via the existing provider, validates schema, overwrites the store copy, updates state. Pure opt-in.
- On binary upgrade, `EmbeddedVersion` changes. Next invocation's stat-check notices the hash mismatch and silently re-installs from embed. No user action needed.

### Governance — preventing scope creep

This is the hard constraint that makes Phase 3 defensible rather than self-promotion:

> Scribe auto-installs **exactly one** first-party bootstrap skill. It is read-only markdown, authored by the Scribe project, shipped embedded in the binary. We do not auto-install:
> - Third-party skills
> - Package-type skills (anything that runs shell commands)
> - Skills that touch user data outside `~/.scribe/skills/scribe-agent/`
>
> Adding a second auto-installed skill requires a design document and changelog entry.

Document this in the project CLAUDE.md and reference it here.

### What Phase 3 explicitly does NOT do

- No network fetch during `EnsureScribeAgent`. The `@HEAD` supply-chain risk codex flagged (mutable branch ref in `internal/provider/treescan.go:16`) is avoided entirely because we never follow `@HEAD`.
- No reuse of the `add`/`sync` pipeline. That pipeline is auth-gated (`cmd/add.go:104, 207`) and networked; neither is acceptable in a silent bootstrap.
- No prompt. A prompt would need a non-TTY auto-skip, at which point the prompt branch is dead code.
- No writes outside `~/.scribe/skills/scribe-agent/` and the state file.

### Tests

- `TestEnsureScribeAgentNoOpWhenPresent` — pre-seed store + state with matching `EmbeddedVersion`; verify zero writes.
- `TestEnsureScribeAgentInstallsWhenMissing` — empty store; verify `SKILL.md` appears with embedded bytes and state records `OriginBootstrap`.
- `TestEnsureScribeAgentReinstallsOnVersionMismatch` — store has older `EmbeddedVersion`; verify overwrite.
- `TestEnsureScribeAgentRespectsOptOut` — `scribe_agent.enabled: false`; verify no-op.
- `TestEnsureScribeAgentSurvivesReadOnlyStore` — `os.WriteFile` errors; verify command does not abort and warning is emitted once.
- `TestEnsureScribeAgentNotCalledByVersionCommand` — `scribe version` runs without triggering bootstrap.
- `TestUpgradeAgentCommandRefreshesFromNetwork` — the explicit refresh path fetches, validates, and writes.

### Adopt interaction (non-issue, documented)

Counselor codex confirmed in round 2 (`internal/adopt/candidate.go:65, 152`) that `adopt` scans `~/.claude/skills` and `~/.codex/skills`, not `~/.scribe/skills/`, and already short-circuits on hash equality. The embedded bootstrap writes to `~/.scribe/skills/scribe-agent/` and projects into tool dirs via the normal sync mechanism, so `adopt` will see an identical hash and skip. No special case needed.

### Tools-empty edge case

If the user has no active tools detected (fresh install, no `~/.claude` or `~/.codex`), `EnsureScribeAgent` still installs to `~/.scribe/skills/scribe-agent/` but the skill never projects into a tool directory. This is acceptable — the skill exists in the store for future tool detection to pick up — but the `scribe` empty-state message should hint: "scribe-agent installed but no AI tool detected. Run `scribe tools` to see what's supported."

---

## Phase 4 — anthropic/skills tree-scan investigation

Round-2 claude-opus withdrew the "tree-scan path logic is wrong" blind spot after re-reading `internal/provider/github.go:191`. Root and nested layouts both resolve correctly in `Fetch`. So the empty result is coming from `Discover`, not `Fetch`.

Real investigation steps:

1. `gh api repos/anthropic/skills/git/trees/HEAD?recursive=1` — capture the live tree.
2. Run that through `ScanTreeForSkills` in isolation (`internal/provider/treescan.go`) with a test fixture.
3. Likely culprits: directory-depth cap, filename casing, or a requirement that skills live in a named subdirectory rather than root.
4. Fix with a fixture-driven test using the captured tree.

Not a blocker for Phase 1. Ship separately.

---

## Phase 5 — `openai/codex-skills` removal with migration

`ApplyBuiltins` is **append-only** (`internal/firstrun/firstrun.go:130`). Removing `openai/codex-skills` from `builtinRepos` does not remove it from existing users' `config.yaml`. Users who upgraded with `openai/codex-skills` already connected will keep seeing "no skills found" warnings on `list --remote` forever.

Changes:

- Add `ApplyBuiltinsRemove(removed []string)` that prunes matching entries from connected registries on first post-upgrade invocation, guarded by a one-shot marker in state (e.g. `state.Migrations["remove_openai_codex_v1"] = true`).
- Emit a single-line explanatory note to stderr on the migration run: `scribe: removed openai/codex-skills (no manifest) from connected registries`.
- `scribe registry forget <repo>` as a user-facing escape hatch so anyone can prune entries without waiting for a migration.

Pair with a backoff mechanism for *any* persistent registry failure: if a registry's `Discover` fails N consecutive invocations, mark it muted in state and stop warning. User can unmute with `scribe registry resync <repo>`.

---

## Sequencing & risk

| Phase | Size | Risk | Blocked by |
|-------|------|------|------------|
| 1 (commit 1: `RemoteFlag`) | ~20 LOC | Low | — |
| 1 (commit 2: split load + banner) | ~150 LOC | Medium — touches config load path | Commit 1 |
| 1 (config field scaffold) | ~30 LOC | Low | — |
| 4 (anthropic fix) | ~80 LOC | Low | — |
| 5 (openai removal + backoff) | ~200 LOC | Medium — state migration | — |
| 3 (EnsureScribeAgent) | ~300 LOC + embed | Medium — governance + embed plumbing | Phase 1 commit 2, config field |
| 2 (data-loader extract) | ~150 LOC | Low — mechanical refactor | Phase 1 complete |
| 2 (Shape A: `scribe browse`) | ~400 LOC | Medium — new TUI + JSON command | Data-loader extract |
| 2 (Shape B: `:` ExecProcess) | ~80 LOC | Low — thin shell-out | Shape A |

**Ship order:** Phase 1 → Phase 4 → Phase 5 → Phase 3 → Phase 2 (extract → Shape A → Shape B).

## Open questions for the user

1. **Embedded source of truth for Phase 3:** pin to a Naoray/scribe **tag** (manual cadence) or a **commit SHA** (CI-bumped every release)? Tag is simpler, SHA is more precise.
2. **Registry failure backoff policy:** mute after N failures, or time-based (mute for 24h after failure)? Counselors flagged the problem, not the cure.
3. **`scribe browse` vs `scribe add`:** `add` already has an interactive browser. Options: (a) `browse` is a new command that reuses the browser UI; (b) `add` with no args becomes `browse`, `add <name>` stays direct install; (c) deprecate `add`'s interactive mode in favour of `browse`. Current lean: (a) — additive, no breaking change.

## Appendix — counselor evidence

Full reports: `agents/counselors/1776260237-review-request-question-design-review-f/`

Key file citations from the reviews (preserved for future reference):

- `buildRows` ignores `RemoteFlag`: `cmd/list_tui.go:216`
- JSON path already honors `RemoteFlag`: `internal/workflow/list.go:44`
- Banner prints from pre-run: `cmd/root.go:58`
- `gh auth token` 5s timeout: `internal/github/client.go:45`
- Custom-tool shell execution: `internal/tools/runtime.go:134`, `internal/tools/command.go:62`
- Tree-scan `@HEAD` mutability: `internal/provider/treescan.go:16`
- `ApplyBuiltins` append-only: `internal/firstrun/firstrun.go:130`
- Adopt scans `~/.claude`/`~/.codex`, hash-dedup: `internal/adopt/candidate.go:65, 152`
- Add pipeline is auth-gated: `cmd/add.go:104, 207`
- Install projection depends on active tools: `internal/sync/syncer.go:382`
- Type-as-filter test contract: `cmd/list_tui_test.go:319`
