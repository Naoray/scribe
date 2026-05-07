# Team-Sharable Scribe Projects — Design

Status: draft v2 (2026-05-07, post-counselor-review)
Scope: v1 — kits + skills only. Snippets and MCP server definitions deferred to v2.

Revision history:
- v1 → v2 (2026-05-07): rewrite after Opus + Codex counselor review. Reuses `internal/lockfile`. Adds explicit `OriginProject`. Picks `lockfile.HashFiles` for content hashing. Adds drift matrix, trust model, and Frankenstein-folder protection. Removes `scribe sync --update-lock`. Hardens Boost interop against `replaceSymlink`'s real-dir refusal.

## Problem

A `.scribe.yaml` at a repo root already declares the project's intent: which `kits:`, `add:` skills, `mcp:` servers, etc. The dotfile is committed, so the *intent* is shared with the team.

Gap: the **artifacts those names refer to** aren't shared. Today:

- `kits: [foo]` resolves against `~/.scribe/kits/foo.yaml` — author-machine-only.
- `add: [owner/repo:bar]` resolves through registries the author has connected — teammate may have no such registry.
- `snippets: [baz]` resolves against `~/.scribe/snippets/baz.md` — author-machine-only.

A teammate cloning the repo and running `scribe sync` will fail-fast on any reference whose source is missing on their machine.

We want: *clone repo → `scribe sync` works → identical skill loadout to the author.*

## Goals

1. Teammate cloning a scribe-enabled repo can run `scribe sync` and get the same kits + skills installed as the author, deterministically.
2. The repo carries enough information to reproduce the kits-and-skills loadout — no hidden author-machine state required.
3. Author has a single command to publish their loadout into the repo.
4. Project-local artifacts coexist with the author's global `~/.scribe/` store; project precedence wins.
5. Adding team-share to a project does **not** silently leak the author's personal/private skills into the team repo.

## Non-Goals (v1)

- **Snippets are out of scope for v1.** v1 changes `scribe sync` to *skip* snippet projection when sources are missing in team-share mode; the snippet-vendoring design is a follow-up.
- **MCP server definitions** are out of scope. v1 still supports the existing `mcp:` field in `.scribe.yaml` (projection of *names* into `.claude/settings.json` is unchanged), but it does not ship the MCP definitions themselves. v1 documents this as a partial guarantee: "kits + skills are reproducible; MCP names project but their definitions remain a teammate-machine concern."
- No new `.scribe.yaml` schema fields. The intent file stays as-is.
- No new top-level dotfile or directory at the repo root (everything new lives inside the existing `.ai/` directory).
- No team registry of kits/snippets as a first-class concept.
- No automatic CI hook. Authors run `scribe project sync` explicitly. (`scribe project sync --check` is in v1 for opt-in CI.)

## Decisions

### D1 — Hybrid skill model with explicit origin classification

Two kinds of skills, each shared differently:

- **Project-authored skills** — created explicitly for a project via a new `scribe project skill create` command (or migrated via `scribe project skill claim`). Marked in machine state with a new `Origin = OriginProject`. Vendored as full folders inside the repo at `.ai/skills/<name>/`.
- **Registry skills** — pulled from a connected registry. Pinned in `.ai/scribe.lock` (entry per skill: `commit_sha` + `content_hash` + optional `install_command_hash`). Teammate's `scribe sync` fetches each pinned commit into the existing name-keyed cache (`~/.scribe/skills/<name>/`) and symlinks into agent skill directories.

**Vendoring is opt-in, not inferred.** Specifically:

| Origin | Default action in `scribe project sync` |
|---|---|
| `OriginProject` | Auto-vendor into `.ai/skills/<name>/` |
| `OriginRegistry` | Auto-pin into `.ai/scribe.lock` |
| `OriginLocal` | **Refuse** unless `--vendor <name>` flag explicitly opts the skill in (sets `Origin = OriginProject` on success) |
| `OriginBootstrap` | **Skip** entirely; bootstrap skills are part of the binary contract and must not be vendored |
| `OriginAdopted` | Same as `OriginLocal` — refuse without explicit `--vendor` |

This addresses the "silent private-skill leak" risk: the author must explicitly elect each personal skill into the team repo. The default is safe.

### D2 — Repo layout

```
repo/
├── .scribe.yaml          # intent (existing, unchanged)
└── .ai/
    ├── skills/
    │   └── tdd/                          # vendored skill folder
    │       ├── SKILL.md
    │       ├── ...
    │       └── .scribe-content-hash      # per-skill content fingerprint
    ├── kits/
    │   └── laravel-baseline.yaml         # vendored kit definition
    └── scribe.lock                       # pins for registry skills
```

Reasoning:
- `.ai/` is an existing standard (Laravel Boost convention) for AI-related project artifacts.
- `.scribe.yaml` stays at repo root because that's where it shipped.
- `.ai/scribe.lock` and the registry-side `scribe.lock` (which lives at the *registry repo's* root) share a filename but never coexist in the same directory; the file format and parser is shared (see D5).
- The `.scribe-content-hash` marker inside each vendored skill protects against git per-file merge "Frankenstein" content (see Edge Case F).

### D3 — Two commands, separate concerns

- `scribe project sync` (NEW, author-side) — reads `.scribe.yaml`, materializes referenced kits + project-authored skills into the repo, writes lockfile entries for registry skills. Outbound: machine → repo.
- `scribe sync` (existing, extended for teammate-side) — reads project artifacts when present, fetches pinned skills, symlinks into agent dirs. Inbound: repo → machine. **Never writes inside `.ai/`.**

The split is intentional. `scribe sync` running inside an author's repo never silently rewrites the repo. Vendoring is an explicit publish step. The previously-considered `scribe sync --update-lock` flag is **removed**: any lockfile rewrite must go through `scribe project sync [--force]` so the same validation, conflict detection, and JSON envelope apply uniformly.

`scribe project sync --check` (added to v1, not parking-lot) computes what `scribe project sync` would write and exits 8 (validation) if the on-disk artifacts disagree. Useful in CI to prevent committed `.ai/` content from drifting from `.scribe.yaml`.

Naming concern: `scribe project sync` next to `scribe sync` is provisional. Alternatives considered: `scribe vendor`, `scribe project publish`, `scribe project bundle`. Final naming will be locked during implementation; this spec uses `scribe project sync` consistently.

### D4 — Layering: project + global, project precedence

Inside a scribe project, kits resolve from `.ai/kits/*.yaml` first, then from `~/.scribe/kits/*.yaml`. On name conflict, project wins. Same applies to skills: `.ai/skills/<name>/` (vendored) takes precedence over anything resolved from the lockfile or global cache.

The merge is implemented through a new `internal/projectstore` package that exposes a `Resolver` composing two `Store` layers (project + global) — *not* a `LoadAllMerged` shortcut on `internal/kit`. This keeps the layering reusable for future v2 snippet/MCP shipping.

### D5 — Lockfile: extend `internal/lockfile`, don't duplicate

The existing `internal/lockfile` package already defines:

```go
const Filename = "scribe.lock"
const SchemaVersion = 1

type Lockfile struct {
    FormatVersion int     `yaml:"format_version"`
    Registry      string  `yaml:"registry"`
    Entries       []Entry `yaml:"entries"`
}

type Entry struct {
    Name               string `yaml:"name"`
    SourceRegistry     string `yaml:"source_registry"`
    CommitSHA          string `yaml:"commit_sha"`
    ContentHash        string `yaml:"content_hash"`
    InstallCommandHash string `yaml:"install_command_hash,omitempty"`
}
```

This struct is registry-side (lives at a registry repo's root, fetched by `FetchFile(... "scribe.lock", "HEAD")`). Project-side team-share needs a closely-related but distinguishable shape, because:
- A registry-side lockfile pins skills *within one registry*; the `Registry` field is mandatory and singular.
- A project-side lockfile pins skills across *multiple registries* — each entry's `SourceRegistry` is the source of truth.

**Decision: extend the package with a discriminator and a project-side type, sharing parsing + hashing infrastructure.**

```go
const ProjectFilename = "scribe.lock"   // when found at .ai/scribe.lock
const ProjectKind     = "ProjectLock"

type ProjectLockfile struct {
    FormatVersion int     `yaml:"format_version"`
    Kind          string  `yaml:"kind"`           // "ProjectLock"
    GeneratedAt   string  `yaml:"generated_at,omitempty"`
    GeneratedBy   string  `yaml:"generated_by,omitempty"`
    Entries       []Entry `yaml:"entries"`        // reuses existing Entry
}
```

- Parser disambiguates by inspecting `kind:` first; missing or empty `kind:` → existing registry-side `Lockfile{Registry, Entries}`.
- Same `Entry` struct: `name`, `source_registry`, `commit_sha`, `content_hash`, `install_command_hash`.
- Same hashing primitives: `lockfile.HashFiles(files []File)` for content_hash, `lockfile.CommandHash(parts...)` for install_command_hash.

**Hash mechanism — single source of truth.** The `content_hash` field is computed by `lockfile.HashFiles` over the skill folder's *installable files* (the same set used by registry-side hashing today). This:
- Resolves the "blob_sha vs tree hash vs SKILL.md sha" ambiguity flagged by both counselors.
- Reuses code paths already exercised in production.
- Catches `scripts/run.sh`-only changes (the case the SKILL.md-only blob hash misses).
- Field name in the lockfile is `content_hash`, not `blob_sha`. The v1 draft's name was wrong.

Vendored skills are not in the lockfile — their `.scribe-content-hash` marker file (next section) is their pin.

### D6 — Vendored-skill content fingerprint

Each vendored skill folder contains a `.scribe-content-hash` file at its root:

```
.ai/skills/tdd/.scribe-content-hash
─────────────────────────────────
sha256:9f3c1d8a... 
generated_at: 2026-05-07T14:23:00Z
generated_by: scribe@v0.8.0
```

Computed via `lockfile.HashFiles` over the folder's installable files (excluding `.scribe-content-hash` itself). On `scribe sync`:
- Recompute the hash and compare against the marker file.
- Mismatch → exit 8 (validation) with "Frankenstein folder detected; the vendored content was modified outside `scribe project sync`. Reconcile by running `scribe project sync --force` (overwrite) or restoring the file you edited."

This blocks the git per-file merge case where two authors push divergent vendor states and git produces a folder that exists in no single author's tree.

### D7 — Drift handling: fail-fast, lockfile is canonical

If `.scribe.yaml` references a skill not in the lockfile (and not vendored), exit 8 (validation) with a remediation pointing to `scribe project sync`.

If a lockfile entry's `content_hash` doesn't match the fetched content, exit 6 (network/remote). The user decides whether to bump the lockfile (`scribe project sync --force`) or restore the registry to the pinned state.

**Lockfile is canonical inside a project.** `scribe sync` reconciles machine state to the lockfile; `scribe sync` never updates the lockfile. `scribe project sync` reads machine state to *propose* a new lockfile; the user sees a diff before write (`--force` to skip the diff prompt).

This rule eliminates the "two pin sources of truth" risk: machine state can be rebuilt from lockfile + vendored content, but the lockfile is always written from a deliberate author action.

## Architecture

### Author flow

```
~/.scribe/kits/foo.yaml ─┐
~/.scribe/skills/bar/    ├──► scribe project sync ──► .ai/kits/foo.yaml
state.OriginProject only │                            .ai/skills/bar/{SKILL.md,...,.scribe-content-hash}
state.OriginRegistry  ───┘                            .ai/scribe.lock
                                                      (state stays unchanged)
```

1. Author edits `.scribe.yaml` (existing flow).
2. `scribe project sync` reads intent, classifies each referenced skill by `Origin`, vendors `OriginProject`, pins `OriginRegistry`, refuses `OriginLocal`/`OriginAdopted` without `--vendor`, skips `OriginBootstrap`. Writes `.scribe-content-hash` markers. Computes new lockfile, shows diff, asks confirmation (or `--force`).
3. Author commits `.ai/` along with `.scribe.yaml`. Pushes.

### Teammate flow

```
.scribe.yaml          ─┐
.ai/kits/             ├──► scribe sync ──► ~/.claude/skills/<links>
.ai/skills/           │                    ~/.codex/skills/<links>  (skipped in Boost projects)
.ai/scribe.lock       │                    ~/.scribe/skills/<name>/  (cache, name-keyed)
                      └──► fetch + verify pinned skills
```

1. Teammate clones, runs `scribe sync` in the repo.
2. Sync detects `.ai/scribe.lock` present → "team-share mode."
3. For each lockfile entry: ensure `~/.scribe/skills/<name>/` cache matches `commit_sha`; fetch from `source_registry@commit_sha` if absent or stale; verify against `content_hash`; fail-fast on mismatch.
4. For each vendored skill under `.ai/skills/<name>/`: verify `.scribe-content-hash` matches recomputed hash; symlink into agent skill dirs *unless* the project is a Boost project and Boost owns Claude projection (D8 below).
5. Snippet projection: in team-share mode, missing `~/.scribe/snippets/<name>.md` is a *no-op with warning*, not an error. Existing managed blocks in `CLAUDE.md`/`AGENTS.md` are preserved (the author's commit carries them). Documented as a v1 limitation; full snippet vendoring is v2.
6. MCP projection unchanged. Missing local MCP server definitions for names referenced in `.scribe.yaml` produces a warning, not an error.

### D8 — Laravel Boost interop

Boost research (scratchpad id 1316) confirms `boost:update` reads `.ai/skills/` as input and writes a *real folder copy* into `.claude/skills/<name>/`. Counselor review caught the consequence: scribe's `tools.replaceSymlink` returns `ErrRealDirectoryExists` when its target is a real directory, so a naive `scribe sync` after `boost:update` would fail, not converge.

**Behavior in Boost projects** (detected by `composer.json` containing `laravel/boost`):

| Skill source | Claude projection owner | Codex/other projection owner |
|---|---|---|
| Vendored at `.ai/skills/<name>/` | **Boost** (scribe skips Claude) | **scribe** |
| Registry-pinned via lockfile | **scribe** | **scribe** |

Vendored skills coexist by ownership split: Boost handles Claude (its convention); scribe handles non-Claude tools. Both project from the same `.ai/skills/<name>/` source, so end content matches.

Registry-pinned skills are not in `.ai/skills/`, so Boost never sees them; scribe owns all projections.

Documentation: spec includes a "Boost projects: run order" note recommending `boost:update && scribe sync` as the canonical sequence (idempotent under D6's content-hash check), and explains that Claude skill links are managed by Boost in such projects.

Non-Boost projects retain the original behavior: scribe symlinks all targets including Claude.

### D9 — Trust model (v1 baseline)

- Registry skills carry `install_command_hash` in lockfile entries (existing field); scribe refuses to run install commands when the hash doesn't match the registry's manifest. Carries over from existing per-machine sync flow.
- Vendored skills do not carry an install-command pin in v1; they have no install commands today (skill.SKILL.md is content-only). If/when project-authored skills gain runtime install commands, this gap must be closed before v1 can ship that feature.
- Connected-registries gate: scribe only fetches from registries the user has explicitly `scribe registry connect`-ed. Lockfile entries pointing to non-connected registries → exit 4 (permission) with hint to run `scribe registry connect`.
- First-time projection of vendored content: `scribe sync` warns when symlinking a `.ai/skills/<name>/` not previously seen on this machine, with the path. User is expected to inspect repo content; scribe does not sandbox or validate skill bodies.

This baseline is intentionally conservative for v1; richer signing or registry allow-listing is parking-lot.

## Drift Matrix

Each row pairs two sources of truth and names the owner that resolves disagreement.

| State A | State B | Owner / heal direction | Action |
|---|---|---|---|
| `.scribe.yaml` references kit `K` | `.ai/kits/K.yaml` missing | Author runs `scribe project sync` | `scribe sync` exits 8 with hint |
| `.ai/kits/K.yaml` exists | `.scribe.yaml` doesn't reference `K` | Orphaned vendor; warn, never auto-delete | Surface in `scribe project sync --check` |
| `.scribe.yaml` `add: [S]` | `.ai/scribe.lock` missing entry for `S` | Author runs `scribe project sync` | `scribe sync` exits 8 |
| `.ai/scribe.lock` entry for `S` | `.scribe.yaml` doesn't reference `S` (and no kit transitively does) | Stale pin; `scribe project sync` removes | Surface in `--check` |
| Vendored `.ai/skills/S/` | `.scribe-content-hash` mismatch with actual files | User edited or git-merged | `scribe sync` exits 8 (Frankenstein protection) |
| Vendored `.ai/skills/S/` | `~/.scribe/skills/S/` differs (author edited locally) | **Project wins** | Used as projection source; global cache untouched |
| Lockfile entry `S` | `~/.scribe/skills/S/` has different commit_sha | **Lockfile wins** | `scribe sync` re-fetches into cache, verifies content_hash |
| `~/.scribe/state.json` says skill `S` Origin=Project | No `.ai/skills/S/` in repo | Author hasn't run `project sync` yet | Surface in `--check` |
| `~/.scribe/state.json` missing entry for vendored skill | Vendored `.ai/skills/S/` exists | Migration / fresh clone | `scribe sync` populates state from project artifacts |

## Components

### New

| Path | Purpose |
|---|---|
| `internal/projectstore/projectstore.go` | Reads `.ai/skills/`, `.ai/kits/`, `.ai/scribe.lock` from a project root; verifies `.scribe-content-hash` markers |
| `internal/projectstore/resolver.go` | `Resolver` composes [project, global] stores with project precedence |
| `cmd/project.go` | Parent `scribe project` command group |
| `cmd/project_sync.go` | `scribe project sync` (with `--check`, `--force`, `--vendor <name>`, `--json`) |
| `cmd/project_skill.go` | `scribe project skill create`, `scribe project skill claim` (sets `Origin = OriginProject`) |

### Extended

| Path | Change |
|---|---|
| `internal/lockfile/lockfile.go` | Add `ProjectLockfile` type + `kind: ProjectLock` discriminator. Parser dispatches on `kind:`. Reuse `Entry`, `HashFiles`, `CommandHash`. |
| `internal/state/state.go` | Add `OriginProject` constant. Migration: existing `OriginLocal` skills stay `OriginLocal`; users opt them into `OriginProject` via `scribe project skill claim` (one-time). |
| `internal/sync/` | Sync executor consults `projectstore.Resolver`; respects lockfile precedence; handles team-share-mode snippet skip; in Boost projects, skips Claude projection for vendored skills. |
| `cmd/sync.go` | Detects team-share mode (presence of `.ai/scribe.lock`); removes `--update-lock` flag (the operation must use `scribe project sync` instead). |

### Untouched

- `.scribe.yaml` schema. v1 is a storage layer change, not an intent change.
- `internal/snippet/` API surface; v1 only changes the *call site* in sync to handle team-share-mode missing-source as a warning.

## Resolution algorithm

### `scribe project sync`

1. Walk project root upward to find `.scribe.yaml`.
2. Parse intent: `kits:`, `add:`, `mcp:`.
3. For each kit name in `kits:`:
   - If `~/.scribe/kits/<name>.yaml` doesn't exist → exit 3.
   - Compare project-side `.ai/kits/<name>.yaml` if present.
     - Identical → no-op.
     - Project newer (mtime + diff) → exit 5 unless `--force`.
     - Otherwise copy and update vendoring state.
4. For each entry in `add:` (and each transitive skill from kits):
   - Read `Origin` from machine state (`internal/state`).
   - `OriginProject` → vendor as folder copy. Compute `.scribe-content-hash` and write into folder. Update vendoring state.
   - `OriginRegistry` → resolve through registry (`internal/manifest`, `internal/sync` plumbing) for `commit_sha` + fetch content for `content_hash`. Carry over `install_command_hash` from registry's lockfile entry if present. Write into `.ai/scribe.lock`.
   - `OriginLocal` / `OriginAdopted` → exit 5 with "skill `<name>` has Origin=Local; pass `--vendor <name>` to elect it into the project, or recreate via `scribe project skill claim <name>`."
   - `OriginBootstrap` → skip with informational note ("bootstrap skill `<name>` is shipped by the scribe binary; not vendored").
5. Compare lockfile entries against `add:` ∪ kits' transitive skills. Drop stale entries.
6. Sort lockfile deterministically and write atomically (`tmp + rename`).
7. If `--check`, compute the diff against on-disk content; non-empty diff → exit 8.
8. Print summary; emit JSON envelope when `--json`.

### `scribe sync` (extended)

1. Existing project root detection.
2. If `.ai/scribe.lock` present → enter team-share mode.
3. Load merged kits via `projectstore.Resolver`. Project entries win on conflict.
4. Load lockfile:
   - For each pinned skill, ensure `~/.scribe/skills/<name>/` cache reflects `commit_sha`.
   - Missing or stale → fetch `source_registry@commit_sha`, populate cache, verify `content_hash`. Fail fast on mismatch (exit 6).
5. For vendored skills under `.ai/skills/<name>/`:
   - Verify `.scribe-content-hash` matches recomputed hash. Mismatch → exit 8 (Frankenstein protection).
   - Symlink into agent skill dirs per active tools, **except** for Claude in Boost projects (where Boost owns the projection).
   - If a target `.claude/skills/<name>/` already exists as a real directory (e.g. Boost just ran), and we're not in a Boost project (so we expected to symlink), surface exit 5 (conflict) with a hint.
6. Snippet projection: if a `snippets:` name has no `~/.scribe/snippets/<name>.md` source AND we're in team-share mode → log a warning and skip. Otherwise behave as today.
7. MCP projection unchanged. Warn (don't error) on missing local MCP server definitions for names referenced in `.scribe.yaml`.
8. Existing per-tool projection logic continues unchanged for non-vendored, non-team-share targets.

## Edge cases

| Case | Behavior |
|---|---|
| First-time author bootstrap (no `.ai/`, no lockfile, fresh `.scribe.yaml`) | `scribe sync` runs in legacy/author mode (no team-share). Warns "this project is not yet team-shared; run `scribe project sync` to enable." Skills resolve from machine state as today. |
| Lockfile present but `add:` entries exist with no lockfile entry | Exit 8. Hint: `scribe project sync` (author) or report drift to author (teammate). |
| Lockfile entry pin no longer fetchable | Exit 6. Surface registry + skill name. |
| Vendored skill name collides with lockfile pin name | Vendored wins (project precedence); warn. This is a config error; `scribe project sync` should refuse to write both. |
| `~/.scribe/kits/foo.yaml` differs from `.ai/kits/foo.yaml` | Project wins silently. |
| `scribe project sync` would overwrite a hand-edited project copy | Exit 5 unless `--force`. With `--force`, write a backup file (`.ai/kits/foo.yaml.bak.<timestamp>`) before overwrite. |
| `.scribe-content-hash` missing inside vendored skill folder | Treated as "user-owned" content (e.g. pre-existing `.ai/skills/<name>/` from before scribe team-share). `scribe sync` warns and projects but doesn't validate; `scribe project sync --force` adopts and writes the marker. |
| Concurrent `scribe project sync` runs in same project | Existing per-state lock at `~/.scribe/state.json` is *not sufficient*. Add a project-scoped lock at `~/.scribe/state/project-locks/<project-root-hash>.lock` covering project sync writes to `.ai/`. |
| `boost:update` runs after `scribe sync` | In Boost projects: scribe never owns Claude projection for vendored skills, so no clash. Registry-pinned skills aren't in `.ai/skills/` so Boost can't see them. |
| `scribe sync` after `boost:update` rebuilt `.claude/skills/foo/` as a real dir | In Boost projects: scribe skips Claude projection for `foo` (vendored case). In non-Boost projects: scribe surfaces exit 5 with a remediation. |
| Two authors push divergent `scribe project sync` results, git per-file merges | `.scribe-content-hash` mismatch → next `scribe sync` exits 8. Reconciliation: rerun `scribe project sync` from a clean state. |

## Testing

- **Unit**: `ProjectLockfile` parse/write round-trip + discriminator dispatch; `Resolver` precedence; vendor classification by `Origin`; `.scribe-content-hash` round-trip; Boost detection.
- **Integration**: golden-file end-to-end via `testdata/`. Synthetic project with `.scribe.yaml` + `.ai/`, run sync against fake registry, snapshot resulting agent skill dirs. Cover Boost-mode (composer.json present) vs non-Boost.
- **E2E**: anvil worktree pair. Author worktree runs `scribe project skill create custom-skill`, `scribe project sync`, commits. Teammate worktree runs `scribe sync`, asserts identical skill dirs in `~/.claude/skills/`. Edit `.ai/skills/custom-skill/SKILL.md` on disk (simulating Frankenstein) and assert exit 8.
- **Concurrency**: two parallel `scribe project sync` invocations; one wins, the other waits or errors clearly.

## Migration

No automatic migration. Existing scribe projects continue working with author-machine-only resolution. Upgrade path: author runs `scribe project sync` once; on next push, repo becomes team-shareable.

For projects with hand-rolled `.ai/skills/<name>/` content predating scribe vendoring (e.g. Laravel Boost authoring), `scribe project sync --force --adopt` adopts those folders into vendoring state by writing `.scribe-content-hash` against the existing content (without changing it). Without `--adopt`, the command refuses (exit 5) to avoid silent overwrites.

`OriginProject` does not auto-migrate from `OriginLocal`. Authors run `scribe project skill claim <name>` once per personal-but-now-team-shared skill to opt in. This is intentional — see D1's safety rationale.

## Parking lot

- **Snippets in team-share (v2)** — vendor `.ai/snippets/<name>.md` and adapt projection to read from project store.
- **MCP server definitions** — share `.mcp.json` content per tool. Out of scope.
- **Per-skill install_command pinning for vendored skills** — needed if/when project-authored skills can carry runtime install commands. Today they cannot.
- **Registry allow-listing / signing** — richer trust model beyond the connected-registries gate.
- **Content-addressed cache (`~/.scribe/skills/<sha>/`)** — would let two projects pin different revs of the same name. Not needed for v1; today's name-keyed cache is sufficient given lockfile validation.
- **Final naming** — `scribe project sync` vs `scribe vendor` vs `scribe project publish`. Decide before implementation.

## Resolved (was open in v1)

- `blob_sha` semantics → resolved: reuse `lockfile.HashFiles`; field renamed `content_hash` for consistency with existing schema.
- Lockfile package collision → resolved: extend `internal/lockfile` with discriminated `ProjectLockfile`; share `Entry`, `HashFiles`, `CommandHash`.
- Vendor-vs-pin classification → resolved: explicit `OriginProject`; `OriginLocal` requires `--vendor` opt-in; bootstrap origins skipped.
- Boost interop → resolved: detect Boost project, partition projection ownership (Boost = Claude, scribe = others); registry pins unaffected.
- Two pin sources of truth → resolved: lockfile is canonical inside project; sync reconciles state to lockfile; `scribe sync --update-lock` removed.
- Frankenstein vendored folders → resolved: `.scribe-content-hash` marker per skill; sync verifies.
