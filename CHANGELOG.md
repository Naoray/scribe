## v1.1.0 — Unreleased

### Added
- **Snippets now project during `scribe sync`** — `.scribe.yaml` `snippets:` entries load `~/.scribe/snippets/<name>.md`, strip frontmatter, and write managed rules into `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and Cursor `.cursor/rules/*.mdc` targets.
- **Project files can declare MCP servers directly** — `.scribe.yaml` now supports `mcp:` and `mcp_servers:` alongside kit-declared MCP server names.
- **`scribe show` and `scribe explain` understand snippets** — project snippets show with byte size/targets, and `scribe explain <snippet>` can render snippet source.
- **Plain `scribe sync` converges project kits** — when `.scribe.yaml` declares kits, sync installs missing kit-resolved skills without requiring an extra flag.

## v1.0.13 — 2026-05-04

### Fixed
- **Release archives no longer publish `checksums.txt`** — GoReleaser now omits the checksum asset, and `scribe upgrade` verifies downloaded archives against GitHub's per-asset SHA256 digest instead.
- **`scribe resolve --ours` strips conflict markers** — resolving a merge conflict by keeping the local version now removes leftover conflict marker text before saving the skill.
- **Reconcile removes missing projections cleanly** — stale projection entries are dropped when their target links no longer exist.
- **Codex project projections and budget checks stay aligned** — project-local Codex projections are preserved correctly, description budget checks model shortened descriptions, and orphaned legacy global Codex links are pruned during project sync.
- **Project kits can declare Claude MCP servers** — `scribe sync` now projects kit-defined MCP server commands into project-local Claude settings while preserving existing user-managed settings.

## v1.0.12 — 2026-05-04

### Fixed
- **`scribe skill tools --disable` now respects project-local projections** — disabling a tool for a skill now removes the symlink from the project-local path (e.g., `.agents/skills/`) and updates the `Projections` state entry, so the budget check correctly excludes the skill for that agent in that project. Previously the global path was unlinked but the project projection persisted.
- **`scribe upgrade` verifies installed binary version after upgrade** — after a successful Homebrew or `go install` upgrade, scribe now runs `scribe --version` on the resolved binary and confirms it reports the expected release tag. Mismatches return exit code 5 with a remediation hint.

## v1.0.11 — 2026-05-04

### Fixed
- **Stale `.codex/skills/` symlinks cleaned up on next sync** — older scribe versions created symlinks under `.codex/skills/` instead of `.agents/skills/`. Running `scribe sync` now silently removes those legacy links so Codex no longer sees duplicate or phantom skill entries.

## v1.0.10 — 2026-05-04

### Fixed
- **Codex skills now project to `.agents/skills/` not `.codex/skills/`** — Codex CLI reads skills from `~/.agents/skills/` (and project-local `.agents/skills/`). Scribe was writing to the wrong path; symlinks now land in the correct location.

### Docs
- Added `scribe kit create` to the commands reference.

## v1.0.9 — 2026-05-03

### Fixed
- **`scribe sync` now projects only kit-resolved skills** — previously, all installed skills were symlinked into `project/.claude/skills/` and `project/.codex/skills/` regardless of kit membership, producing ~125 projections instead of the expected ~32. `scribe sync` now filters the skill set through the same kit resolver used by `scribe show` and the agent budget check. Skills installed globally but not listed in any project kit are no longer projected into that project.

## v1.0.5 — 2026-05-02

### Breaking

- **`scribe migrate global-to-projects` now refuses without `--project`** — bare non-interactive migration no longer auto-selects every discovered project when global symlinks exist. Pass one or more `--project <path>` values, or select projects interactively.

### Added

- **Migration recovery snapshots and `--undo`** — successful migrations persist pre-apply JSON snapshots in `~/.scribe/migration-history/`, retain the latest 10, and support latest-only `scribe migrate global-to-projects --undo`.
- **Non-interactive confirmation controls** — `--yes` skips destructive confirmation prompts, and `--force` allows migration when budget preflight would otherwise refuse.
- **Budget-aware migration plans** — migration simulates each target project's post-migration skill set, refuses over-budget writes unless `--force` is passed, and prints per-agent budget status in dry-run output.
- **Doctor signal for migration budget overflow** — `scribe doctor` warns when migration-derived project projections exceed an agent budget even though sync preserves them for compatibility.
- **JSON output schema** — `scribe migrate global-to-projects` now registers an output schema for automation via `scribe schema migrate global-to-projects --json`.

### Fixed

- **Legacy projection state is cleaned up during migration** — migrated skills no longer keep empty-project legacy projection entries after global symlink removal.
- **Project-scoped migration projections are recorded immediately** — migrated projects get state entries tagged as migration-derived, preventing the legacy global projection banner from firing again after a clean migration.

## v1.0.4 — 2026-05-02

### Fixed

- **`scribe migrate global-to-projects` skips hidden dirs and tolerates transient walk errors** — v1.0.3 fixed the `~/.Trash` EACCES, but the same class of bug surfaced again as `open ~/.openclaw/.../runtime-mirror.lock: no such file or directory` whenever the walk hit any other hidden subtree with broken symlinks or weird state. Generalize the skip rule to drop *all* dot-prefixed directories (they're tool/runtime state, never scribe projects), and treat every non-root walk error as recoverable instead of fatal — the project search is best-effort by design. Only an error on the search root itself still aborts the scan. ([#151](https://github.com/Naoray/scribe/pull/151))

## v1.0.3 — 2026-05-02

### Fixed

- **`scribe migrate global-to-projects` no longer aborts on macOS-protected dirs** — `filepath.WalkDir` propagated EACCES from `~/.Trash` (and similar TCC-protected paths) as a fatal error, so running the migration from `~` failed with `discover migration candidates: scan project candidates: open /Users/<you>/.Trash: operation not permitted`. The walk now skips permission errors, and `.Trash` joins the in-tree skip list so the syscall is avoided entirely. Other walk errors still propagate. ([#150](https://github.com/Naoray/scribe/pull/150))

## v1.0.2 — 2026-05-01

### Fixed

- **`scribe upgrade` self-upgrades for `go install` builds** — the dev-build skip read the raw `Version` package var, which stays `"dev"` for `go install github.com/Naoray/scribe@vX.Y.Z` because goreleaser ldflags only run at release time. Module-versioned installs now flow through `currentVersion()`, which falls back to `debug.ReadBuildInfo()` so they participate in the upgrade comparison. Local source builds (`go build` / `go install ./...`) still see `Main.Version == "(devel)"` and remain dev-skipped. ([#147](https://github.com/Naoray/scribe/pull/147))

### Docs

- **Broader agent-coverage messaging** — README hero, cross-tool projection bullet, install-via-agent block, agents-first bullet, and one-manifest bullet now lead with "any AI coding agent" and call out built-in support for Claude Code, Codex, Cursor, and Gemini plus `scribe tools add` for any custom tool. Mirrored in `SKILL.md` and `docs/projects-and-kits.md`. ([#148](https://github.com/Naoray/scribe/pull/148))

## v1.0.1 — 2026-05-01

### Changed

- **BREAKING — embedded skill renamed `scribe-agent` → `scribe`** — first-run installs as `scribe`, and existing v1.0 installs are migrated automatically on next invocation (state entries + canonical store + projection symlinks repointed in one shot). Update any automation that hardcoded the skill name. ([#146](https://github.com/Naoray/scribe/pull/146))

### Docs

- **Embedded skill teaches kit + snippet authoring** — `SKILL.md` now walks agents through the `.scribe.yaml` kit and snippet flow, so the skill itself is the authoring contract. ([#144](https://github.com/Naoray/scribe/pull/144))
- **`docs/projects-and-kits.md` documents agent-driven authoring** — the v1.0 path for hand-authoring kits and snippets is now in the docs site. ([#145](https://github.com/Naoray/scribe/pull/145))

## SemVer commitment (from v1.0.0)

Starting with v1.0.0 we follow [Semantic Versioning](https://semver.org).

**Stable surface (breaking changes require a major version bump):**
- The `--json` envelope shape (`status`, `format_version`, `data`, `meta` fields).
- Documented exit codes per CLAUDE.md (0–10).
- Command names and required flags listed in `scribe schema --all --json`.
- The `SKILL.md` frontmatter fields scribe parses (`name`, `description`, `source`).
- `scribe.toml` / `scribe.yaml` package manifest schema.
- `scribe.lock` schema (`format_version: "1"`).

**Not stable (may change in minor releases):**
- Internal Go packages under `internal/`.
- Text output formatting (the JSON envelope is the agent contract).
- TUI behavior / keybindings.
- Diagnostic messages and `scribe doctor` output.

**Deprecation policy:** any breaking change to the stable surface gets a deprecation warning emitted via stderr for at least one minor version before removal in the next major.

## v1.0.0 — 2026-04-30

The "agent-first foundation" wave. Scribe's `--json` output is now a versioned envelope, mutator commands have semantic exit codes, every migrated command exposes its JSON Schema, and the data layer for kits + snippets + project files is in place. Most of this is foundation — user-facing flows ride on top in follow-up releases.

### Added

- **JSON envelope contract** (`format_version: "1"`) — every migrated command wraps its output in `{status, format_version, data, meta}`. `meta.duration_ms` measures leaf execution; `meta.bootstrap_ms` covers first-run + store migration + builtins. ([#117](https://github.com/Naoray/scribe/pull/117), [#118](https://github.com/Naoray/scribe/pull/118), [#119](https://github.com/Naoray/scribe/pull/119))
- **Semantic exit codes** at registry, network, validation, and conflict boundaries — `2` usage, `3` not-found, `4` permission, `5` conflict, `6` network, `7` dependency, `8` validation, `9` user-canceled, `10` partial success. ([#119](https://github.com/Naoray/scribe/pull/119))
- **Schema introspection** — `scribe schema list|status|doctor|explain|guide|sync|add|adopt|connect --json` returns JSON Schema 2020-12 for inputs and outputs, so agents can compose calls without guessing. ([#118](https://github.com/Naoray/scribe/pull/118), [#119](https://github.com/Naoray/scribe/pull/119))
- **Field projection** — opt-in `--fields name,version` flag on read-only commands with tabular output (gh-style). Wired per command via `output.AttachFieldsFlag`. ([#118](https://github.com/Naoray/scribe/pull/118))
- **`CLAUDE.md` agent contract** — generated from `docs/agent/CLAUDE.md.tmpl` via `go generate`, committed at the repo root, embedded in the binary, and materialized beside `SKILL.md` when scribe-agent installs. Drift is a build-time gate. ([#119](https://github.com/Naoray/scribe/pull/119))
- **`.scribe.yaml` project file** — schema + parser for declaring per-project kits, snippets, extra skills to add, and skills to remove. Empty or missing files are no-ops. ([#121](https://github.com/Naoray/scribe/pull/121))
- **Kit schema + resolution algorithm** — kits express ordered skill bundles (e.g., "laravel-baseline"); the resolver merges declared kits, projectfile add/remove, and installed-skill state into a target set. ([#124](https://github.com/Naoray/scribe/pull/124))
- **State schema v5** — projection indexes plus first-class storage for kits and snippets. Lays groundwork for fast lookup once the user-facing kit/snippet commands ship. ([#125](https://github.com/Naoray/scribe/pull/125))
- **`scribe-hook.sh` script** — embedded shim that lets Claude Code (and similar) call into `scribe` from session lifecycle hooks. ([#122](https://github.com/Naoray/scribe/pull/122))
- **Hook installer package** — internal `internal/hooks` package handles install/uninstall + status of `scribe-hook.sh` in Claude Code settings, with idempotent merge into existing user hooks. ([#123](https://github.com/Naoray/scribe/pull/123))
- **Skill deny-list** — explicitly removed skills are remembered, so `sync` no longer re-installs them on the next reconcile. ([#116](https://github.com/Naoray/scribe/pull/116))
- **Packages store** — multi-skill upstream packages now project as a single store entry instead of being splayed across tool skill dirs. ([#113](https://github.com/Naoray/scribe/pull/113))
- **Per-project projection writer** — Claude, Codex, and Cursor skill links now project under the resolved `.scribe.yaml` project root while preserving global fallback when no project file exists. ([#128](https://github.com/Naoray/scribe/pull/128))
- **`scribe migrate global-to-projects`** — discovers legacy global tool symlinks, lets users select a target project, writes `.scribe.yaml` `add:` entries idempotently, and removes migrated global links. ([#134](https://github.com/Naoray/scribe/pull/134))
- **Per-agent description-byte budget guardrail** — Codex and Claude projections estimate resolved skill description budgets, show utilization, and refuse over-budget projections unless `--force` is passed. ([#133](https://github.com/Naoray/scribe/pull/133))
- **Skill source attribution** — `SKILL.md` frontmatter can include `source.url`, `source.author`, and `source.note`; `explain --json`, `list`, and `browse` surface attribution when present. ([#135](https://github.com/Naoray/scribe/pull/135))
- **`scribe push <name>`** — local skill edits can be pushed back to their originating registry through the GitHub Contents API with author checks and divergence conflict handling. ([#136](https://github.com/Naoray/scribe/pull/136))
- **Alias support for name conflicts** — `sync`, `install`, and `add` can resolve real-directory conflicts with `--alias` or an interactive Adopt / Alias / Skip prompt. ([#137](https://github.com/Naoray/scribe/pull/137))
- **`scribe init` package author scaffold** — discovers local `SKILL.md` files, prompts for package metadata in TTY mode, and writes `scribe.toml` for publishing skill packages. ([#138](https://github.com/Naoray/scribe/pull/138))
- **`scribe.lock` reproducibility flow** — lockfiles pin commit SHA, content hash, and install command hash; `scribe check` plans updates and `scribe update --apply` refreshes pins. ([#139](https://github.com/Naoray/scribe/pull/139))
- **Comparison docs** — `docs/comparison.md` compares scribe with skills.sh, superpowers, Anthropic skills, Cursor MDC, Cline/Roo, and MCP across practical adoption dimensions. ([#140](https://github.com/Naoray/scribe/pull/140))

### Changed

- **BREAKING — `--json` payload shape** for `scribe list`, `status`, `doctor`, `explain`, `guide`. Previously top-level keys are now under `data`. Migrate parsers from `jq '.foo'` to `jq '.data.foo'`. ([#118](https://github.com/Naoray/scribe/pull/118))
- **BREAKING — mutator `--json` payload shape** for `scribe sync`, `add`, `adopt`, `connect`. Same envelope rules. When `data.summary.failed > 0`, `status` becomes `"partial_success"` and exit code is `10`. ([#119](https://github.com/Naoray/scribe/pull/119))
- **State schema bumped to v5** — projection indexes plus kits/snippets storage. Migration runs automatically at first invocation; no user action required. ([#125](https://github.com/Naoray/scribe/pull/125))
- **README split into focused docs** — the README is now a front page with quickstart and positioning, with command, JSON envelope, project/kit, adoption, and troubleshooting reference moved into `docs/`. ([#131](https://github.com/Naoray/scribe/pull/131))
- **Legacy global-projection compatibility mode** — when no `.scribe.yaml` is present, scribe preserves existing global projection behavior and emits a once-per-day deprecation banner for `scribe migrate global-to-projects`. ([#132](https://github.com/Naoray/scribe/pull/132))
- **Essentials registry pointer** — README now links to the public `Naoray/scribe-skills-essentials` starter registry. ([#141](https://github.com/Naoray/scribe/pull/141))

### Fixed

- **`scribe --version`** now falls back to `debug.ReadBuildInfo` for `go install`-based builds, so installs without a release-time ldflag still report a meaningful version. ([#115](https://github.com/Naoray/scribe/pull/115))
- **Hook installer hardening** — malformed Claude Code hook settings now return actionable errors without rewriting user config, and the embedded hook script no longer blocks on inherited stdin. ([#127](https://github.com/Naoray/scribe/pull/127))
- **`scribe doctor` opaque-tool drift check** — opaque tools such as Gemini no longer produce false `projection_drift` errors when their skill path is intentionally unavailable. ([#129](https://github.com/Naoray/scribe/pull/129))
- **`scribe list` viewport and bootstrap filtering** — cursor scrolling now matches rendered group headers, stale offsets clamp correctly, and the auto-managed `scribe-agent` bootstrap skill is hidden from list output. ([#130](https://github.com/Naoray/scribe/pull/130))

### Internal

- **`cli/output` and `cli/workflow` foundation packages** — agent-first plumbing for the envelope, persistent `--json` flag, and `wrapRunE`. ([#117](https://github.com/Naoray/scribe/pull/117))
- **Envelope unification** — `jsonFormatter.Flush` now routes through `cli/output.Renderer`, so every JSON path emits a single envelope instead of writing partial blobs. ([#120](https://github.com/Naoray/scribe/pull/120))
- **Spec deviation: `--fields` is its own flag**, not overloaded `--json name,version`. Preserves boolean `--json=true` compatibility for existing shell scripts. See `scribe schema <cmd> --json` for valid field names per command.

### Deferred

A handful of older mutator commands (`install`, `remove`, `resolve`, `restore`, `skill`, `tools`, `config`, `create`, `registry*`, `upgrade`, `migrate`, `browse`) still emit pre-envelope output and reject `--json` with `JSON_NOT_SUPPORTED` until they migrate. Track via `scribe schema --all --json`.

The CLI surfaces for kits, snippets, hooks, and `.scribe.yaml` activation are designed and in flight; only the foundation ships in this wave.
