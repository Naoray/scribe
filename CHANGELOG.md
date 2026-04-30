## Unreleased

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

### Changed

- **BREAKING — `--json` payload shape** for `scribe list`, `status`, `doctor`, `explain`, `guide`. Previously top-level keys are now under `data`. Migrate parsers from `jq '.foo'` to `jq '.data.foo'`. ([#118](https://github.com/Naoray/scribe/pull/118))
- **BREAKING — mutator `--json` payload shape** for `scribe sync`, `add`, `adopt`, `connect`. Same envelope rules. When `data.summary.failed > 0`, `status` becomes `"partial_success"` and exit code is `10`. ([#119](https://github.com/Naoray/scribe/pull/119))
- **State schema bumped to v5** — projection indexes plus kits/snippets storage. Migration runs automatically at first invocation; no user action required. ([#125](https://github.com/Naoray/scribe/pull/125))

### Fixed

- **`scribe --version`** now falls back to `debug.ReadBuildInfo` for `go install`-based builds, so installs without a release-time ldflag still report a meaningful version. ([#115](https://github.com/Naoray/scribe/pull/115))

### Internal

- **`cli/output` and `cli/workflow` foundation packages** — agent-first plumbing for the envelope, persistent `--json` flag, and `wrapRunE`. ([#117](https://github.com/Naoray/scribe/pull/117))
- **Envelope unification** — `jsonFormatter.Flush` now routes through `cli/output.Renderer`, so every JSON path emits a single envelope instead of writing partial blobs. ([#120](https://github.com/Naoray/scribe/pull/120))
- **Spec deviation: `--fields` is its own flag**, not overloaded `--json name,version`. Preserves boolean `--json=true` compatibility for existing shell scripts. See `scribe schema <cmd> --json` for valid field names per command.

### Deferred

A handful of older mutator commands (`install`, `remove`, `resolve`, `restore`, `skill`, `tools`, `config`, `create`, `registry*`, `upgrade`, `migrate`, `browse`) still emit pre-envelope output and reject `--json` with `JSON_NOT_SUPPORTED` until they migrate. Track via `scribe schema --all --json`.

The CLI surfaces for kits, snippets, hooks, and `.scribe.yaml` activation are designed and in flight; only the foundation ships in this wave.
