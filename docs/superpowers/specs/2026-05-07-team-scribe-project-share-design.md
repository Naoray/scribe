# Team-Sharable Scribe Projects — Design

Status: draft v3.1 (2026-05-07, third counselor pass — final polish)
Scope: v1 — kits + skills only. Snippets and MCP server definitions deferred to v2.

Revision history:
- v1 → v2 (2026-05-07): rewrite after Opus + Codex counselor review. Reuses `internal/lockfile`. Adds explicit `OriginProject`. Picks `lockfile.HashFiles` for content hashing. Adds drift matrix, trust model, and Frankenstein-folder protection. Removes `scribe sync --update-lock`. Hardens Boost interop against `replaceSymlink`'s real-dir refusal.
- v2 → v3 (2026-05-07): second counselor pass. Extends `ProjectLockfile.Entry` with a frozen fetch descriptor (path, source repo, type, per-tool install commands) so teammate sync no longer depends on a live registry manifest. Defines the content-hash file set explicitly with git-tracked-only selection and LF normalization. Hardens `OriginRegistry` classification against zero-value legacy state. Threads Boost ownership through `state.InstalledSkill.ExcludedTools` so reconcile and installer agree. Unifies `CommandHash` on the lockfile-format SHA-256. Distinguishes project-lock validation from registry-side `validateInstalledAgainstLock`. Specifies qualified `add:` parsing. Adds `state.VendorState` for first-seen tracking. Adds project-skill command semantics. Drops the spurious `OriginAdopted` row.
- v3 → v3.1 (2026-05-07): third counselor pass — final polish. R3-1: hash-set git mode now uses `git ls-files --cached --others --exclude-standard` so freshly-vendored-but-not-yet-staged files are included (closes the first-author-run blocker). R3-2: `ExcludedTools` moves from `state.InstalledSkill` (global) to `state.ProjectionEntry` (per-project), and the filter is applied in `EffectiveToolsForProject` as well as `EffectiveTools`. R3-3: split package `install_command_hash` semantics — lockfile self-consistency mismatch is a validation error (exit 8); approval-state-vs-lockfile mismatch triggers re-approval. Drift matrix purged of any "re-fetched manifest commands" wording.

## Problem

A `.scribe.yaml` at a repo root already declares the project's intent: which `kits:`, `add:` skills, `mcp:` servers, etc. The dotfile is committed, so the *intent* is shared with the team.

Gap: the **artifacts those names refer to** aren't shared. Today:

- `kits: [foo]` resolves against `~/.scribe/kits/foo.yaml` — author-machine-only.
- `add: [owner/repo:bar]` resolves through registries the author has connected — teammate may have no such registry.
- `snippets: [baz]` resolves against `~/.scribe/snippets/baz.md` — author-machine-only.

A teammate cloning the repo and running `scribe sync` will fail-fast on any reference whose source is missing on their machine.

We want: *clone repo → connect referenced registries → `scribe sync` works → identical kits-and-skills loadout to the author.*

## Goals

1. Teammate cloning a scribe-enabled repo runs `scribe registry connect <repo>` for any referenced registries they don't already have, then runs `scribe sync` and gets the same kits + skills installed as the author. Deterministic given those connect prerequisites. (Goal narrowed from v2: connect prerequisites are explicit; not "zero-setup".)
2. The repo carries enough information to reproduce the kits-and-skills loadout — no hidden author-machine state required beyond connected-registry credentials.
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
- **Registry skills** — pulled from a connected registry. Pinned in `.ai/scribe.lock` with a self-contained `ProjectEntry` (see D5). Teammate's `scribe sync` fetches each pinned commit into the existing name-keyed cache (`~/.scribe/skills/<name>/`) and symlinks into agent skill directories.

**Vendoring is opt-in, not inferred.**

| Origin | Default action in `scribe project sync` |
|---|---|
| `OriginProject` | Auto-vendor into `.ai/skills/<name>/` |
| `OriginRegistry` (with valid sources — see below) | Auto-pin into `.ai/scribe.lock` |
| `OriginRegistry` (zero-value, no usable `Sources`) | **Refuse** with hint to reinstall via `scribe install` or claim via `scribe project skill claim` |
| `OriginLocal` | **Refuse** unless `--vendor <name>` flag explicitly opts the skill in (sets `Origin = OriginProject` on success) |
| `OriginBootstrap` | **Skip** entirely; bootstrap skills are part of the binary contract and must not be vendored |

`OriginRegistry` is the zero value of `state.Origin` (per `internal/state/state.go`). Legacy or corrupted state entries can have `Origin == OriginRegistry` without populated `Sources`, which would otherwise produce unpinnable lockfile entries. **`OriginRegistry` is necessary but not sufficient** for auto-pinning. The classification rule:

1. `Origin == OriginRegistry` AND
2. `len(InstalledSkill.Sources) >= 1` AND
3. The chosen source has a non-empty `SourceRepo` (or `Registry`) and a known `Path`/ref AND
4. The on-disk content under `~/.scribe/skills/<name>/` matches the source's recorded `LastSHA`/`BlobSHAs` for that ref

Otherwise refuse and surface remediation.

`OriginAdopted` is *not* a real origin in current state; `scribe adopt` produces `OriginLocal`. Adopted skills follow the `OriginLocal` row above.

This addresses the "silent private-skill leak" risk and the "zero-value-Registry pin" risk together: the author must hold a valid registry source for every auto-pinned skill, and explicitly elect each personal skill into the team repo. The default is safe.

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

### D4 — Project skill commands

`scribe project skill create <name>` — creates a new project-authored skill. Behavior:
- Writes `~/.scribe/skills/<name>/` exactly like the existing `scribe skill create`.
- Sets `state.InstalledSkill{Origin: OriginProject}` for that name.
- Does NOT write to `.ai/skills/` directly; vendoring happens at `scribe project sync` time. This keeps "create" a machine-side authoring action and "publish" a project-side action.
- Refuses if a skill of that name already exists with a different origin (suggests `scribe project skill claim` instead).

`scribe project skill claim <name>` — converts an existing local-origin skill to project-origin. Behavior:
- Refuses if `Origin == OriginRegistry` (would silently detach from registry source — user must explicitly remove + recreate, or vendor with `--vendor` and accept the consequences).
- Refuses if `Origin == OriginBootstrap` (binary-shipped contract).
- Sets `Origin = OriginProject` for `OriginLocal` skills.
- One-time, idempotent for same input.
- Does not modify on-disk skill content.

Both commands respect `--json` and emit the project envelope.

### D5 — Layering: project + global, project precedence

Inside a scribe project, kits resolve from `.ai/kits/*.yaml` first, then from `~/.scribe/kits/*.yaml`. On name conflict, project wins. Same applies to skills: `.ai/skills/<name>/` (vendored) takes precedence over anything resolved from the lockfile or global cache.

The merge is implemented through a new `internal/projectstore` package that exposes a `Resolver` composing two `Store` layers (project + global) — *not* a `LoadAllMerged` shortcut on `internal/kit`. This keeps the layering reusable for future v2 snippet/MCP shipping.

### D6 — Lockfile: extend `internal/lockfile` with self-contained project entries

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

The registry-side struct is fetched from a registry repo's root by `FetchFile(... "scribe.lock", "HEAD")` and works in tandem with a live `manifest.Entry`. Project-side team-share needs more: the project lockfile is the canonical fetch contract on the teammate machine, and the live registry manifest may have moved or been edited since the author published.

**Decision: extend the package with a discriminator and a project-side type that embeds the existing `Entry` plus a frozen fetch descriptor.**

```go
const ProjectFilename = "scribe.lock"   // when found at .ai/scribe.lock
const ProjectKind     = "ProjectLock"

type ProjectLockfile struct {
    FormatVersion int            `yaml:"format_version"`
    Kind          string         `yaml:"kind"`           // "ProjectLock"
    GeneratedAt   string         `yaml:"generated_at,omitempty"`
    GeneratedBy   string         `yaml:"generated_by,omitempty"`
    Entries       []ProjectEntry `yaml:"entries"`
}

type ProjectEntry struct {
    Entry          `yaml:",inline"`                    // embeds: name, source_registry, commit_sha, content_hash, install_command_hash
    SourceRepo     string            `yaml:"source_repo,omitempty"`     // owner/repo of skill source if different from source_registry
    Path           string            `yaml:"path,omitempty"`            // path within source repo (defaults to name)
    Type           string            `yaml:"type,omitempty"`            // "skill" | "package", default "skill"
    Install        string            `yaml:"install,omitempty"`         // global install command frozen from manifest
    Update         string            `yaml:"update,omitempty"`          // global update command frozen from manifest
    Installs       map[string]string `yaml:"installs,omitempty"`        // per-tool install commands, frozen
    Updates        map[string]string `yaml:"updates,omitempty"`         // per-tool update commands, frozen
}
```

Why each field:
- `SourceRepo` distinguishes catalog `source` from registry `source_registry` (a registry can list a skill that lives in a different repo).
- `Path` carries `manifest.Entry.Path` so a skill named `tdd` registered at `skills/tdd-pro/` is fetchable.
- `Type` lets project sync apply the existing package-vs-skill split (packages bypass per-tool routing).
- `Install`/`Update`/`Installs`/`Updates` are frozen at lock time; together with `install_command_hash`, they make package commands reproducible and approval-gateable on the teammate machine even if the registry catalog later drifts.

- Parser disambiguates by inspecting `kind:` first; missing or empty `kind:` → existing registry-side `Lockfile{Registry, Entries}`.
- Hashing primitives: `lockfile.HashFiles(files []File)` for `content_hash`. Command hashing: see "CommandHash unification" below.

**Content-hash file set — explicit allow-list.** The `content_hash` field is computed by `lockfile.HashFiles` over a deterministic, closed set of files inside the skill folder:

1. **When the skill folder is inside a git working tree:** include exactly the files reported by `git ls-files --cached --others --exclude-standard <skill-dir>` (filtered to regular files, excluding submodules). The `--cached --others --exclude-standard` triplet covers tracked files *plus* untracked-not-ignored files — the second part is essential because `scribe project sync` writes vendored files into `.ai/skills/<name>/` *before* the author runs `git add`, so plain `git ls-files <skill-dir>` would return an empty or partial set on the very first author run, producing a `.scribe-content-hash` that doesn't match the post-commit teammate verify.
2. **When not inside git** (e.g. `~/.scribe/skills/<name>/` global cache): include every file under the skill root *except* the explicit denylist below.
3. **Denylist (always excluded, in both modes):** `.git/`, `versions/` (existing exclusion), `.DS_Store`, `Thumbs.db`, `*.swp`, `*.swo`, `.idea/`, `.vscode/`, `node_modules/`, `*.bak.*`, `.scribe-content-hash` itself.
4. **Line-ending normalization:** every file's content is normalized (`\r\n` → `\n`) before hashing. This eliminates Windows `core.autocrlf=true` divergence.

The git-mode rule must produce identical results before and after the author commits the vendored files: `--cached --others --exclude-standard` includes both tracked and untracked-not-ignored files, so the hash is stable across the commit boundary. Tests must cover: (a) fresh repo, vendor + hash *before* `git add`; (b) same repo *after* commit; (c) teammate clone — all three must produce the same `.scribe-content-hash`.

The same ruleset is used to compute and verify the per-skill `.scribe-content-hash` marker. The hash function lives at `internal/lockfile/hashset.go` (new helper alongside `HashFiles`).

This resolves the v2 risk that `.DS_Store` / line-ending differences would trigger spurious Frankenstein exits on a clean teammate clone.

**CommandHash unification.** Two implementations exist today: `internal/sync.CommandHash` (returns 16 hex chars, used for package approval state) and `internal/lockfile.CommandHash` (returns full SHA-256). v3 makes the lockfile version canonical. Migration: `internal/sync.CommandHash` becomes a thin alias to `internal/lockfile.CommandHash`; package approval state is upgraded on next read (state file v3 records full hash; v2 short hashes compare as upgrade-required, prompting re-approval via the existing approval flow). Lockfile entries always carry the full SHA-256.

Vendored skills are not in the lockfile — their `.scribe-content-hash` marker file (next section) is their pin.

### D7 — Vendored-skill content fingerprint

Each vendored skill folder contains a `.scribe-content-hash` file at its root:

```
.ai/skills/tdd/.scribe-content-hash
─────────────────────────────────
sha256:9f3c1d8a... 
generated_at: 2026-05-07T14:23:00Z
generated_by: scribe@v0.8.0
```

Computed via the hash-set rules in D6 (git-tracked-only when in git, deterministic denylist otherwise; LF-normalized; excludes `.scribe-content-hash` itself). On `scribe sync`:
- Recompute the hash and compare against the marker file.
- Mismatch → exit 8 (validation) with "Frankenstein folder detected; the vendored content was modified outside `scribe project sync`. Reconcile by running `scribe project sync --force` (overwrite) or restoring the file you edited."

This blocks the git per-file merge case where two authors push divergent vendor states and git produces a folder that exists in no single author's tree.

### D8 — Drift handling: fail-fast, lockfile is canonical

If `.scribe.yaml` references a skill not in the lockfile (and not vendored), exit 8 (validation) with a remediation pointing to `scribe project sync`.

If a lockfile entry's `content_hash` doesn't match the fetched content, exit 6 (network/remote). The user decides whether to bump the lockfile (`scribe project sync --force`) or restore the registry to the pinned state.

**Lockfile is canonical inside a project.** `scribe sync` reconciles machine state to the lockfile; `scribe sync` never updates the lockfile. `scribe project sync` reads machine state to *propose* a new lockfile; the user sees a diff before write (`--force` to skip the diff prompt).

This rule eliminates the "two pin sources of truth" risk: machine state can be rebuilt from lockfile + vendored content, but the lockfile is always written from a deliberate author action.

### D9 — Project-lock validation algorithm (distinct from registry-side)

Registry-side `Syncer.validateInstalledAgainstLock` assumes the lockfile is the *latest registry state* and refuses installations that disagree. Reusing it for project-lock teammate sync would fail in the very common case of a stale name-keyed cache from another project pinning a different commit.

**v1 introduces `validateProjectLock(*ProjectLockfile, statuses []SkillStatus)`** as a sibling routine in `internal/sync/`:

For each `ProjectEntry`:
1. **Self-consistency check (always first):** for package-type entries, recompute `lockfile.CommandHash(Install, Update, Installs, Updates)` over the entry's own frozen command fields and compare against the entry's `install_command_hash`. Mismatch indicates lockfile corruption or tampering, not approval drift — exit 8 (validation) with "lockfile self-inconsistency: install_command_hash does not match the embedded command fields. The lockfile may be tampered or hand-edited."
2. Read cache state at `~/.scribe/skills/<name>/` and any associated state.
3. If `cache.commit_sha != entry.commit_sha` → mark for refetch (do not error).
4. If cache absent → mark for fetch.
5. After fetch (or if cache matched), recompute `content_hash` over the fetched bytes using D6's hash-set rules.
6. Mismatch → exit 6 with remediation.
7. For package-type entries, compare the lockfile's `install_command_hash` against the user's stored *approval-state* hash (existing approval-state path in `internal/sync` / `internal/state`). Mismatch here is **approval drift** (e.g. lockfile bumped to a newer command set, or first run on a new machine) — trigger the existing package approval prompt; on approval, update the approval-state hash. This is distinct from step 1: step 1 catches a tampered lockfile; step 7 catches an unapproved-but-internally-consistent command set.

Crucially, `validateProjectLock` *expects* cache divergence and refetches; it does not error on stale name-keyed cache. Two-project alternation works without manual cleanup.

### D10 — `add:` parsing: bare names vs qualified refs

`.scribe.yaml` `add:` and `remove:` accept either:

- **Bare name** (e.g. `tdd`): resolves through machine state. If two registries both contain a skill named `tdd`, exit 8 with hint to qualify the entry. Disambiguation is the author's responsibility; project sync refuses ambiguous bare names.
- **Qualified ref** (e.g. `Naoray/scribe:tdd`): resolves directly to that registry/repo + skill. Always wins over a bare name with the same skill identifier (project precedence applies inside the qualified refs already).

The kit transitive resolution follows the same rule: kit `Skills:` entries that are bare names use the disambiguation logic; qualified refs resolve directly.

`scribe project sync` records the resolved source in the lockfile entry's `source_registry` + `source_repo` + `path`, so the lockfile is unambiguous regardless of how the intent was specified.

### D11 — Laravel Boost interop (state-level ownership filter)

Boost research (scratchpad id 1316) confirms `boost:update` reads `.ai/skills/` as input and writes a *real folder copy* into `.claude/skills/<name>/`. Counselor review caught two consequences: (a) scribe's `tools.replaceSymlink` returns `ErrRealDirectoryExists` when its target is a real directory, and (b) scribe's `ReconcilePre`/`ReconcilePost` recompute expected projections from state and active tools, so a naive "skip Claude in installer" still leaves Claude in the expected-projection set and reconcile re-creates the conflict.

**v3 fix: ownership is recorded in state, not just at the installer.**

When a skill is classified as vendored under a Boost project (detected by `composer.json` containing `laravel/boost` plus presence of `.ai/skills/<name>/`), the **per-project** `state.ProjectionEntry` carries an explicit tool-exclusion field:

```go
type ProjectionEntry struct {
    // ... existing fields ...
    ExcludedTools []string `json:"excluded_tools,omitempty"` // tools NOT projected by scribe in THIS project (e.g. "claude" in Boost projects)
}
```

The field lives on `ProjectionEntry`, not `InstalledSkill`, because the same skill can be vendored in project A (Boost — exclude Claude) *and* registry-pinned in project B (non-Boost — all tools). A global field on `InstalledSkill` would flip on every project switch and corrupt reconcile state.

Both helpers in `internal/state/tools_resolve.go` apply the filter:

- `EffectiveTools(available)` — used outside a project — does not consult `ExcludedTools` (no project context).
- `EffectiveToolsForProject(activeNames, projectRoot)` — used by reconcile and project-aware install paths — looks up the matching `ProjectionEntry` for `projectRoot` and subtracts its `ExcludedTools` from the resolved tool list.

This:

- Keeps installer code simple (`tool.Install` is never called for excluded tools).
- Makes `ReconcilePre`/`ReconcilePost` see "Claude is not expected" — they don't try to repair the symlink, so no `ErrRealDirectoryExists`.
- Per-project: `tdd` vendored in Boost project A and registry-pinned in non-Boost project B coexist with correct projections.
- Survives state save/load (existing `ProjectionEntry` already round-trips per project).
- Excludes only specific tools per skill per project, not globally.

**Boost ownership table:**

| Skill source | scribe `ExcludedTools` | Result |
|---|---|---|
| Vendored at `.ai/skills/<name>/`, project is Boost | `["claude"]` | Boost owns `.claude/skills/<name>/`; scribe owns codex/cursor/gemini |
| Vendored, project is non-Boost | `[]` | scribe owns all tools |
| Registry-pinned (lockfile entry) | `[]` | scribe owns all tools (Boost can't see registry skills) |

Documentation: spec includes a "Boost projects: run order" note recommending `boost:update && scribe sync` as the canonical sequence; the operations are idempotent under D7's content-hash check. The state-level filter ensures correctness independent of run order.

### D12 — Trust model (v1 baseline)

- Registry skills carry `install_command_hash` in lockfile entries (full SHA-256 per D6); scribe refuses to run install commands when the hash doesn't match the registry's manifest. Carries over from existing per-machine sync flow.
- Vendored skills do not carry an install-command pin in v1; they have no install commands today (skill `SKILL.md` is content-only). If/when project-authored skills gain runtime install commands, this gap must be closed before that feature ships.
- Connected-registries gate: scribe only fetches from registries the user has explicitly `scribe registry connect`-ed. Lockfile entries pointing to non-connected registries → exit 4 (permission) with hint to run `scribe registry connect <repo>`. (Goal #1 acknowledges this prerequisite.)
- First-time projection of vendored content: `scribe sync` consults `state.VendorState` (new map keyed by skill name on `state.State`) for `FirstSeenAt`. When absent, scribe warns once with the path before symlinking, and writes the timestamp. User is expected to inspect repo content; scribe does not sandbox or validate skill bodies.

This baseline is intentionally conservative for v1; richer signing or registry allow-listing is parking-lot.

## Architecture

### Author flow

```
~/.scribe/kits/foo.yaml ─┐
~/.scribe/skills/bar/    ├──► scribe project sync ──► .ai/kits/foo.yaml
state.OriginProject only │                            .ai/skills/bar/{SKILL.md,...,.scribe-content-hash}
state.OriginRegistry  ───┘                            .ai/scribe.lock  (ProjectEntry per registry skill)
                                                      (state stays unchanged)
```

1. Author edits `.scribe.yaml` (existing flow). Optionally invokes `scribe project skill create` / `scribe project skill claim` to mark project-authored skills.
2. `scribe project sync` reads intent, classifies each referenced skill by `Origin` + sources validity, vendors `OriginProject`, pins valid `OriginRegistry`, refuses zero-value `OriginRegistry` and `OriginLocal` without `--vendor`, skips `OriginBootstrap`. Writes `.scribe-content-hash` markers using the D6 hash-set. Computes new lockfile, shows diff, asks confirmation (or `--force`). Records `state.InstalledSkill.ExcludedTools` for vendored skills in Boost projects.
3. Author commits `.ai/` along with `.scribe.yaml`. Pushes.

### Teammate flow

```
.scribe.yaml          ─┐
.ai/kits/             ├──► scribe sync ──► ~/.claude/skills/<links>  (subject to ExcludedTools)
.ai/skills/           │                    ~/.codex/skills/<links>
.ai/scribe.lock       │                    ~/.scribe/skills/<name>/  (cache, name-keyed; refetch on stale)
                      └──► fetch + verify pinned skills via validateProjectLock
```

1. Teammate clones, runs `scribe registry connect <repo>` for each registry referenced in `.ai/scribe.lock` they don't already have, runs `scribe sync` in the repo.
2. Sync detects `.ai/scribe.lock` present → enters team-share mode.
3. Loads merged kits via `projectstore.Resolver`. Project entries win on conflict.
4. For each `ProjectEntry`: runs `validateProjectLock` (D9) — refetches stale name-keyed cache, validates `content_hash` against fetched content, checks `install_command_hash` for packages. Fail-fast on mismatch.
5. For each vendored skill under `.ai/skills/<name>/`: verifies `.scribe-content-hash` matches recomputed hash; on first sight on this machine, warns and writes `state.VendorState.FirstSeenAt`; symlinks into agent skill dirs filtered by `ExcludedTools`.
6. Snippet projection: in team-share mode, missing `~/.scribe/snippets/<name>.md` is a *no-op with warning*, not an error. Existing managed blocks in `CLAUDE.md`/`AGENTS.md` are preserved. `internal/sync` snippet step gains a `teamShareMode bool` flag.
7. MCP projection: `StepProjectMCPServers` similarly downgrades missing-source errors to warnings in team-share mode (today it errors when `.mcp.json` or definitions are absent).
8. Existing per-tool projection logic continues unchanged for non-vendored, non-team-share targets. `EffectiveTools(available)` is the single point that subtracts `ExcludedTools`, so installer + reconcile see the same expected set.

## Components

### New

| Path | Purpose |
|---|---|
| `internal/projectstore/projectstore.go` | Reads `.ai/skills/`, `.ai/kits/`, `.ai/scribe.lock` from a project root; verifies `.scribe-content-hash` markers |
| `internal/projectstore/resolver.go` | `Resolver` composes [project, global] stores with project precedence |
| `internal/lockfile/hashset.go` | `HashSet(skillDir string) (string, error)` — git-tracked-or-denylist file selection + LF normalization, then `HashFiles` |
| `cmd/project.go` | Parent `scribe project` command group |
| `cmd/project_sync.go` | `scribe project sync` (with `--check`, `--force`, `--vendor <name>`, `--json`) |
| `cmd/project_skill.go` | `scribe project skill create`, `scribe project skill claim` |

### Extended

| Path | Change |
|---|---|
| `internal/lockfile/lockfile.go` | Add `ProjectLockfile` + `ProjectEntry` (with `SourceRepo`, `Path`, `Type`, `Install`, `Update`, `Installs`, `Updates`) + `kind: ProjectLock` discriminator. Parser dispatches on `kind:`. Embed existing `Entry`. |
| `internal/sync/executor.go` | `CommandHash` becomes a thin alias to `internal/lockfile.CommandHash`. Drop the 16-hex-char path; state file v3 stores full SHA-256 for package approval. |
| `internal/state/state.go` | Add `OriginProject` constant. Add `ExcludedTools []string` to `state.ProjectionEntry` (per-project, not the global `InstalledSkill`). Add `state.VendorState` map (or sibling fields) keyed by skill name with `FirstSeenAt`. Bump state file format to v3; existing entries migrate with empty defaults. |
| `internal/state/tools_resolve.go` | `EffectiveToolsForProject(activeNames, projectRoot)` looks up the matching `ProjectionEntry` and subtracts its `ExcludedTools` from the resolved set so reconcile and project-aware installers see the same expected-projection set. `EffectiveTools(available)` (no project context) is unchanged. |
| `internal/sync/syncer.go` | Add `validateProjectLock` distinct from `validateInstalledAgainstLock`; consult `projectstore.Resolver`; team-share-mode snippet+MCP behavior changed (warn, don't error, when sources missing); package approval re-checks against full SHA-256 `install_command_hash` from `ProjectEntry`. |
| `cmd/sync.go` | Detects team-share mode (presence of `.ai/scribe.lock`); removes `--update-lock` flag (the operation must use `scribe project sync` instead). |

### Untouched

- `.scribe.yaml` schema. v1 is a storage layer change, not an intent change.
- `internal/snippet/` API surface; v1 only changes the *call site* in sync to handle team-share-mode missing-source as a warning.

## Resolution algorithm

### `scribe project sync`

1. Walk project root upward to find `.scribe.yaml`.
2. Parse intent: `kits:`, `add:`, `mcp:`, `remove:`. For each `add:` entry, split by `:` to detect qualified refs per D10.
3. For each kit name in `kits:`:
   - If `~/.scribe/kits/<name>.yaml` doesn't exist → exit 3.
   - Compare project-side `.ai/kits/<name>.yaml` if present.
     - Identical → no-op.
     - Project newer (mtime + diff) → exit 5 unless `--force`.
     - Otherwise copy and update vendoring state.
4. For each entry in `add:` (qualified ref or bare name) and each transitive skill from kits:
   - Resolve to `(SourceRepo, Path, registry)` per D10. Bare names that resolve to multiple registries → exit 8.
   - Read state. Apply D1 classification.
   - `OriginProject` → vendor as folder copy. Compute `.scribe-content-hash` via D6 hash-set and write into folder. Update vendoring state. In Boost projects, set `state.InstalledSkill.ExcludedTools = ["claude"]` for this name.
   - `OriginRegistry` (with valid sources) → resolve `commit_sha` from registry, fetch content, compute `content_hash` via D6 hash-set, snapshot `manifest.Entry` fields into `ProjectEntry` (Path, Type, Install, Update, Installs, Updates, install_command_hash via `lockfile.CommandHash`). Write into `.ai/scribe.lock`.
   - `OriginRegistry` (zero-value, no usable sources) → exit 5 with remediation.
   - `OriginLocal` / adopted → exit 5 with `--vendor <name>` hint, OR vendor + claim if the flag was passed.
   - `OriginBootstrap` → skip with informational note.
5. Compare lockfile entries against `add:` ∪ kits' transitive skills. Drop stale entries.
6. Sort lockfile deterministically and write atomically (`tmp + rename`).
7. If `--check`, compute the diff against on-disk content; non-empty diff → exit 8.
8. Print summary; emit JSON envelope when `--json`.

### `scribe sync` (extended)

1. Existing project root detection.
2. If `.ai/scribe.lock` present → enter team-share mode.
3. Load merged kits via `projectstore.Resolver`. Project entries win on conflict.
4. Run `validateProjectLock` (D9):
   - For each `ProjectEntry`, check cache `~/.scribe/skills/<name>/`.
   - Stale or absent → fetch using `SourceRepo`/`Path`/`commit_sha` (NOT `manifest.Entry`; the lockfile is self-contained).
   - Verify `content_hash` over fetched bytes using D6 hash-set. Fail-fast on mismatch (exit 6).
   - For package-type, verify `install_command_hash`; mismatch → require re-approval.
5. For vendored skills under `.ai/skills/<name>/`:
   - Verify `.scribe-content-hash` matches recomputed hash via D6 hash-set. Mismatch → exit 8 (Frankenstein protection).
   - If `state.VendorState[name].FirstSeenAt` is empty, warn with the path and write the current timestamp.
   - Symlink into agent skill dirs per `EffectiveTools(available)` minus `ExcludedTools`.
   - If a target real directory blocks the symlink in a non-Boost project, surface exit 5.
6. Snippet projection: in team-share mode, missing source → log warning and skip. Otherwise unchanged.
7. MCP projection: in team-share mode, missing `.mcp.json` or server definitions → log warning and skip. Otherwise unchanged.
8. Existing per-tool projection logic continues unchanged for non-vendored, non-team-share targets.

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
| Lockfile entry `S` | `~/.scribe/skills/S/` has different commit_sha (e.g. another project) | **Lockfile wins** | `validateProjectLock` re-fetches into cache, verifies `content_hash` |
| Lockfile entry `S` `install_command_hash` | Recomputed `CommandHash` over the entry's own frozen Install/Update/Installs/Updates | Self-inconsistent → **lockfile tampered**, exit 8 | `validateProjectLock` step 1 |
| Lockfile entry `S` `install_command_hash` | User's approval-state hash for `S` | Mismatch → **re-approval needed** | Existing approval prompt; updates approval-state hash |
| `~/.scribe/state.json` says skill `S` Origin=Project | No `.ai/skills/S/` in repo | Author hasn't run `project sync` yet | Surface in `--check` |
| `~/.scribe/state.json` missing entry for vendored skill | Vendored `.ai/skills/S/` exists | Migration / fresh clone | `scribe sync` populates state from project artifacts |
| `~/.scribe/state.json` entry for vendored skill missing `ExcludedTools` in Boost project | Boost project detected | `scribe sync` writes `ExcludedTools=["claude"]` and continues | Idempotent on subsequent runs |

## Components-of-existing-code summary

A reviewer-friendly table of what's reused vs. introduced:

| Existing | Reused as-is in v1 | Extended | Replaced |
|---|---|---|---|
| `internal/lockfile.Entry` | yes (registry-side) | embedded into `ProjectEntry` | — |
| `internal/lockfile.HashFiles`, `HashDir` | `HashFiles` reused | `HashSet` wraps with allow-list + LF | — |
| `internal/lockfile.CommandHash` | yes | becomes single canonical source | replaces `internal/sync.CommandHash` |
| `internal/state.Origin` constants | `OriginRegistry`, `OriginLocal`, `OriginBootstrap` reused | + new `OriginProject` | — |
| `state.InstalledSkill` | yes | unchanged | — |
| `state.ProjectionEntry` | yes | + `ExcludedTools` | — |
| `state.VendorState` (new sibling map) | — | introduced | — |
| `state.tools_resolve.EffectiveToolsForProject` | yes | subtracts per-project `ExcludedTools` | — |
| `internal/sync.Syncer` registry-side path | yes | snippet/MCP teamshare-mode flags, `validateProjectLock` sibling | — |
| `internal/snippet` | yes | call-site change in sync | — |

## Edge cases

| Case | Behavior |
|---|---|
| First-time author bootstrap (no `.ai/`, no lockfile, fresh `.scribe.yaml`) | `scribe sync` runs in legacy/author mode (no team-share). Warns "this project is not yet team-shared; run `scribe project sync` to enable." Skills resolve from machine state as today. |
| Lockfile present but `add:` entries exist with no lockfile entry | Exit 8. Hint: `scribe project sync` (author) or report drift to author (teammate). |
| Lockfile entry pin no longer fetchable | Exit 6. Surface registry + skill name. |
| Lockfile entry references a registry the teammate hasn't connected | Exit 4 (permission) with hint `scribe registry connect <repo>`. |
| Vendored skill name collides with lockfile pin name | Vendored wins (project precedence); warn. `scribe project sync` should refuse to write both. |
| `~/.scribe/kits/foo.yaml` differs from `.ai/kits/foo.yaml` | Project wins silently. |
| `scribe project sync` would overwrite a hand-edited project copy | Exit 5 unless `--force`. With `--force`, write a backup file (`.ai/kits/foo.yaml.bak.<timestamp>`) before overwrite. |
| `.scribe-content-hash` missing inside vendored skill folder | Treated as "user-owned" content (e.g. pre-existing `.ai/skills/<name>/` from before scribe team-share). `scribe sync` warns and projects but doesn't validate; `scribe project sync --force` adopts and writes the marker. |
| Concurrent `scribe project sync` runs in same project | Existing per-state lock at `~/.scribe/state.json` is *not sufficient*. Add a project-scoped lock at `~/.scribe/state/project-locks/<project-root-hash>.lock` covering project sync writes to `.ai/`. |
| Two projects pin different commits of the same skill name | `validateProjectLock` refetches on stale cache; user sees no error, only some refetch latency. Symptom documented. Content-addressed cache is parking-lot. |
| `boost:update` runs after `scribe sync` | In Boost projects: `ExcludedTools=["claude"]` for vendored skills means scribe never owned Claude projection. Boost rebuild is harmless. Reconcile sees the exclusion and doesn't try to repair. |
| `scribe sync` after `boost:update` rebuilt `.claude/skills/foo/` as a real dir | In Boost projects: Claude excluded from `EffectiveTools` for vendored skills, so scribe never targets it. In non-Boost projects: scribe surfaces exit 5 with a remediation. |
| Two authors push divergent `scribe project sync` results, git per-file merges | `.scribe-content-hash` mismatch → next `scribe sync` exits 8. Reconciliation: rerun `scribe project sync` from a clean state. |

## Testing

- **Unit**: `ProjectLockfile`/`ProjectEntry` parse/write round-trip + discriminator dispatch; `Resolver` precedence; vendor classification by `Origin` including zero-value-Registry refusal; `.scribe-content-hash` round-trip via `HashSet`; LF normalization; git-ls-files vs denylist file selection; Boost detection; `EffectiveTools` minus `ExcludedTools`; `CommandHash` consistency between sync and lockfile.
- **Integration**: golden-file end-to-end via `testdata/`. Synthetic project with `.scribe.yaml` + `.ai/`, run sync against fake registry, snapshot resulting agent skill dirs. Cover Boost-mode (composer.json present, `ReconcilePre`/`ReconcilePost` exercised) vs non-Boost. Cross-project skill-name collision: project A pins `tdd@old`, project B pins `tdd@new`; switch dirs and confirm refetch. Hash-set determinism: author commits with `.DS_Store`, teammate clones on Linux, `scribe sync` succeeds.
- **E2E**: anvil worktree pair. Author runs `scribe project skill create custom-skill`, `scribe project sync`, commits. Teammate runs `scribe sync`, asserts identical skill dirs in `~/.claude/skills/`. Edit `.ai/skills/custom-skill/SKILL.md` on disk and assert exit 8 (Frankenstein). For Boost case: pre-create `.claude/skills/custom-skill/` as a real dir (simulating `boost:update`) and confirm scribe sync converges.
- **Concurrency**: two parallel `scribe project sync` invocations; project-scoped lock serializes them.

## Migration

No automatic migration. Existing scribe projects continue working with author-machine-only resolution. Upgrade path: author runs `scribe project sync` once; on next push, repo becomes team-shareable.

For projects with hand-rolled `.ai/skills/<name>/` content predating scribe vendoring (e.g. Laravel Boost authoring), `scribe project sync --force` adopts those folders into vendoring state by writing `.scribe-content-hash` against the existing content (without changing it). Without `--force`, the command refuses (exit 5) to avoid silent overwrites. (No separate `--adopt` flag; v2's mention was inconsistent.)

`OriginProject` does not auto-migrate from `OriginLocal`. Authors run `scribe project skill claim <name>` once per personal-but-now-team-shared skill to opt in. This is intentional — see D1's safety rationale.

State file format bumps to v3 to accommodate `ExcludedTools`, `VendorState`, and full-SHA-256 package approval hashes. Existing v2 state loads, populates new fields with empty defaults, and is rewritten on next save. Package approvals stored with the old 16-hex-char hash require one re-approval per package on next sync (existing approval prompt path).

## Parking lot

- **Snippets in team-share (v2)** — vendor `.ai/snippets/<name>.md` and adapt projection to read from project store. Coordinate with Laravel Boost's `<laravel-boost-guidelines>` block; scribe's `<!-- scribe-snippet:... -->` markers don't collide.
- **MCP server definitions** — share `.mcp.json` content per tool. Out of scope.
- **Per-skill install_command pinning for vendored skills** — needed if/when project-authored skills can carry runtime install commands. Today they cannot.
- **Registry allow-listing / signing** — richer trust model beyond the connected-registries gate.
- **Content-addressed cache (`~/.scribe/skills/<sha>/`)** — would let two projects pin different revs of the same name without refetch. Not needed for v1 given `validateProjectLock`'s refetch-on-stale behavior; document as ergonomic improvement.
- **Final naming** — `scribe project sync` vs `scribe vendor` vs `scribe project publish`. Decide before implementation.

## Resolved (was open in v1 / v2)

- `blob_sha` semantics → resolved (v2): reuse `lockfile.HashFiles`; field renamed `content_hash`.
- Lockfile package collision → resolved (v2): extend `internal/lockfile` with discriminated `ProjectLockfile`.
- Vendor-vs-pin classification → resolved (v2): explicit `OriginProject`; `OriginLocal` requires `--vendor` opt-in; bootstrap origins skipped.
- Boost interop → hardened (v3): `state.InstalledSkill.ExcludedTools` so reconcile and installer agree.
- Two pin sources of truth → resolved (v2): lockfile is canonical inside project.
- Frankenstein vendored folders → resolved (v2): `.scribe-content-hash` marker per skill.
- `ProjectEntry` insufficient for fetching → resolved (v3): frozen fetch descriptor (path, source repo, type, install commands).
- Hash-set undefined → resolved (v3): git-tracked-or-denylist + LF normalization.
- `OriginRegistry` zero-value safety → resolved (v3): require valid sources.
- `CommandHash` two-implementations → resolved (v3): unified on lockfile's full SHA-256.
- Stale name-keyed cache failing project-lock validation → resolved (v3): distinct `validateProjectLock`.
- Qualified `add:` parsing → resolved (v3): bare-name disambiguation rule + qualified ref direct resolution.
- Connected-registry prerequisite → acknowledged in Goal #1 (v3): explicit prerequisite, not zero-setup.
- First-seen warning state field → resolved (v3): `state.VendorState` map.
- Project skill commands semantics → resolved (v3): D4 specifies `create` and `claim`.
- `--adopt` flag inconsistency → resolved (v3): `--force` covers adoption; no separate flag.
- Hash-set git mode missed first-vendor-before-`git add` → resolved (v3.1): `git ls-files --cached --others --exclude-standard` covers tracked + untracked-not-ignored.
- `ExcludedTools` on global `InstalledSkill` would corrupt cross-project projections → resolved (v3.1): moved to per-project `state.ProjectionEntry`; `EffectiveToolsForProject` applies the filter.
- Package `install_command_hash` blurred corruption vs approval drift → resolved (v3.1): `validateProjectLock` step 1 self-consistency check (exit 8 on tamper); step 7 approval-drift check (re-approval prompt).

## Round-3 minors not addressed in v3.1 (parking lot)

- R3-4 (Codex): connect-gate ownership between `source_registry` and `source_repo` — recommended rule (require `source_registry` connected; require `source_repo` connected only when private/cross-namespace) is implementation-time decision, not protocol-blocking.
- R3-5 (Opus NV2): Boost detection escape hatch (`.ai/scribe-tools.yaml claude: external`) for non-composer projects — add when first non-composer Boost-style consumer reports the issue.
