# Team-Sharable Scribe Projects — Design

Status: draft (2026-05-07)
Scope: v1 — kits + skills only. Snippets deferred to v2.

## Problem

A `.scribe.yaml` at a repo root already declares the project's intent: which `kits:`, `add:` skills, `mcp:` servers, etc. The dotfile is committed, so the *intent* is shared with the team.

Gap: the **artifacts those names refer to** aren't shared. Today:

- `kits: [foo]` resolves against `~/.scribe/kits/foo.yaml` — author-machine-only.
- `add: [owner/repo:bar]` resolves through registries the author has connected — teammate may have no such registry, or may resolve a different sha.
- `snippets: [baz]` resolves against `~/.scribe/snippets/baz.md` — author-machine-only.

A teammate cloning the repo and running `scribe sync` will fail-fast on any reference whose source is missing on their machine.

We want: *clone repo → `scribe sync` works → identical skill loadout to the author.*

## Goals

1. A teammate cloning a scribe-enabled repo can run `scribe sync` and get the same skills installed as the author, deterministically.
2. The repo carries enough information to reproduce the loadout — no hidden author-machine state required for kits + skills.
3. Author has a single command to publish their loadout into the repo.
4. Project-local artifacts coexist with the author's global `~/.scribe/` store; project precedence wins.

## Non-Goals (v1)

- **Snippets are out of scope for v1.** Existing rendered output in committed `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and `.cursor/rules/*.mdc` already ships when the author commits those files. v2 will revisit a source-file-based snippet share.
- No new `.scribe.yaml` schema fields. The intent file stays as-is.
- No new top-level dotfile or directory at repo root (we extend existing `.ai/` and keep existing `.scribe.yaml`).
- No team registry of kits/snippets as a first-class concept. Author vendors per project.
- No automatic CI hook. Authors run `scribe project sync` explicitly.

## Decisions

### D1 — Hybrid skill model (vendor + lockfile pin)

Two kinds of skills, each shared differently:

- **Project-authored skills** — created by the author (e.g. with `scribe kit create` then `scribe skill create`) and not pulled from a registry. Vendored as full folders inside the repo at `.ai/skills/<name>/`.
- **Registry skills** — pulled from a connected registry. Pinned by source repo + commit sha + content tree hash in a lockfile at `.ai/scribe.lock`. The teammate's `scribe sync` fetches each pinned sha into the existing machine cache (`~/.scribe/skills/<sha>/`) and symlinks it into agent skill directories.

Detection at vendoring time: a skill whose machine state has a registry origin gets pinned; one with no registry origin gets vendored.

### D2 — Repo layout

```
repo/
├── .scribe.yaml          # intent (existing, unchanged)
└── .ai/
    ├── skills/
    │   └── tdd/          # vendored skill folder (project-authored)
    ├── kits/
    │   └── laravel-baseline.yaml   # vendored kit definition
    └── scribe.lock       # pins for registry skills
```

Reasoning:
- `.ai/` is an existing standard (Laravel Boost convention) for AI-related project artifacts. Reusing it avoids inventing a new top-level directory.
- `.scribe.yaml` stays at the repo root because that's where it shipped already. Moving it would break existing setups for no v1 benefit.
- `.ai/scribe.lock` keeps the lockfile alongside other scribe-managed content under `.ai/`.

### D3 — Two commands, separate concerns

- `scribe project sync` (NEW, author-side) — reads `.scribe.yaml`, materializes referenced kits + skills into the repo, writes lockfile. Outbound write: machine → repo.
- `scribe sync` (existing, extended for teammate-side) — reads project artifacts when present, fetches pinned skills, symlinks into agent dirs. Inbound write: repo → machine.

The split is intentional. `scribe sync` running inside an author's repo should not silently rewrite the repo with whatever the author happened to have on their machine that day. Vendoring is an explicit publish step.

### D4 — Layering: project + global, project precedence

Inside a scribe project, `scribe sync` loads kits from both `.ai/kits/*.yaml` and `~/.scribe/kits/*.yaml`. On name conflict, project wins. Same applies to skills: a vendored `.ai/skills/<name>/` takes precedence over anything in `~/.scribe/skills/`.

This lets a teammate keep their own personal kits/snippets in `~/.scribe/` while the team-shared loadout takes precedence inside the project.

### D5 — Lockfile format: rev + blob_sha

```yaml
apiVersion: scribe/v1
kind: Lockfile
generated_at: 2026-05-07T14:23:00Z
generated_by: scribe@<version>
skills:
  tdd:
    source: Naoray/scribe        # owner/repo of registry that owns the skill
    rev: 8f3c1d9...              # commit sha — exact registry commit
    blob_sha: a1b2c3d4...        # tree hash of skill folder content at that rev
  code-review:
    source: ArtistfyHQ/team-skills
    rev: 1e4f...
    blob_sha: 7c9d...
```

Both pin fields exist on purpose. `rev` gives reproducible fetch (immutable GitHub-side reference). `blob_sha` gives content-fingerprint drift detection — if a registry force-pushes the same `rev` to a different content state, the verify step catches it. This resolves the open issue tracked in memory note `project_update_detection_bug`.

Vendored skills are not in the lockfile. Their committed source is the pin.

### D6 — Drift handling: fail-fast

If `.scribe.yaml` references a skill not in the lockfile, teammate-side `scribe sync` exits with code 8 (validation) and a message pointing to `scribe project sync` (author) or `scribe sync --update-lock` (teammate escape hatch).

If a lockfile entry's `blob_sha` doesn't match what's fetched, exit 6 (network/remote) and surface registry + skill name. The user decides whether to bump the lockfile or restore the registry to the pinned content.

Defaulting to fail-fast keeps teammate runs deterministic; the escape hatch is opt-in.

## Architecture

### Author flow

```
~/.scribe/kits/foo.yaml ─┐
~/.scribe/skills/bar/    ├──► scribe project sync ──► .ai/kits/foo.yaml
registry-origin skills   │                            .ai/skills/bar/
                         └──► resolve registry pins ──► .ai/scribe.lock
```

1. Author edits `.scribe.yaml` — already does this today.
2. `scribe project sync` reads intent, copies kit YAMLs and project-authored skill folders from `~/.scribe/` into `.ai/`, resolves registry skills against connected registries to get rev + blob_sha, writes lockfile.
3. Author commits `.ai/` and `.scribe.lock` along with `.scribe.yaml`. Pushes.

### Teammate flow

```
.scribe.yaml          ─┐
.ai/kits/             ├──► scribe sync ──► ~/.claude/skills/<links>
.ai/skills/           │                    ~/.codex/skills/<links>
.ai/scribe.lock       │                    ~/.scribe/skills/<sha>/  (cache)
                      └──► fetch pinned registry skills missing from cache
```

1. Teammate clones, runs `scribe sync` in the repo.
2. Sync detects project artifacts under `.ai/`, loads merged kits (project + global), reads lockfile.
3. For each pinned registry skill, ensures the cache has the rev; fetches if absent; verifies blob_sha; fails fast on mismatch.
4. Symlinks vendored skills (from `.ai/skills/<name>/`) and cached skills (from `~/.scribe/skills/<sha>/`) into the active agent skill dirs.
5. Existing snippet/MCP projection unchanged.

## Components

### New

| Path | Purpose |
|---|---|
| `internal/projectstore/projectstore.go` | Reads `.ai/skills/`, `.ai/kits/`, `.ai/scribe.lock` from a project root |
| `internal/lockfile/lockfile.go` | Parses + writes `.ai/scribe.lock` |
| `cmd/project.go` | Parent `scribe project` command group |
| `cmd/project_sync.go` | `scribe project sync` implementation |
| `cmd/project_sync_schema.go` | JSON schema + envelope plumbing for the new command |

### Extended

| Path | Change |
|---|---|
| `internal/kit/kit.go` | Add `LoadAllMerged(projectDir, homeDir)` returning project ∪ global with project precedence |
| `internal/sync/` | Sync executor consults `projectstore` first; pinned skills resolved against lockfile; vendored symlinks resolved from project root |
| `internal/state/` | Track per-project vendoring state (last-vendored hash per kit/skill) so `scribe project sync` can diff |

### Untouched

- `.scribe.yaml` schema. v1 is a storage layer change, not an intent change.
- `internal/snippet/` and `internal/manifest/`. v1 doesn't change snippet or registry-manifest behavior.

## Resolution algorithm

### `scribe project sync`

1. Walk project root upward to find `.scribe.yaml`.
2. Parse intent: `kits:`, `add:`, `mcp:`.
3. For each kit name in `kits:`:
   - If `~/.scribe/kits/<name>.yaml` doesn't exist → exit 3 (not found) with a remediation hint.
   - Compare `~/.scribe/kits/<name>.yaml` against `.ai/kits/<name>.yaml` if it already exists.
     - Identical → no-op.
     - Project copy newer (mtime + content diff) and not `--force` → exit 5 (conflict). Surface diff.
     - Otherwise copy and update vendoring state.
4. For each entry in `add:`:
   - Look up the skill in machine state. If origin is "project-authored" → vendor by copying `~/.scribe/skills/<name>/` to `.ai/skills/<name>/` (mirroring the kit conflict rules).
   - If origin is "registry" → resolve through the registry to get current rev + blob_sha. Write entry into lockfile.
5. Compare lockfile entries against `add:` ∪ kits' transitive skills. Drop stale entries.
6. Sort lockfile deterministically and write atomically (`tmp + rename`).
7. Print summary; emit JSON envelope when `--json`.

### `scribe sync` (extended)

1. Existing project root detection.
2. Load merged kits: `.ai/kits/*.yaml` ∪ `~/.scribe/kits/*.yaml`. Project entries win on conflict.
3. Load lockfile if present:
   - For each pinned skill, check `~/.scribe/skills/<sha>/` cache.
   - Missing → fetch `source@rev` from the registry, populate cache, verify against `blob_sha`. Fail fast on mismatch (exit 6).
4. For vendored skills under `.ai/skills/<name>/`, treat the project path itself as the skill source. Symlink directly into agent skill dirs (`~/.claude/skills/<name>`, `~/.codex/skills/<name>`, etc.).
5. Existing flows for snippets, MCP server names, and budget validation continue unchanged.

## Edge cases

| Case | Behavior |
|---|---|
| Lockfile missing entirely, intent has `add:` entries | Exit 3 (not found). Tell user to run `scribe project sync`. |
| Lockfile has pin, registry no longer serves the skill | Exit 6. Surface registry + skill name. |
| Vendored skill name collides with lockfile pin name | Vendored wins; warn in stderr (rare; a config error to call out). |
| `~/.scribe/kits/foo.yaml` differs from `.ai/kits/foo.yaml` | Project wins silently (expected; the project is the team source of truth). |
| `scribe project sync` would overwrite a hand-edited project copy | Exit 5 (conflict). User chooses `--force` or merges by hand. |
| `boost:update` runs after `scribe sync` and rebuilds `.claude/skills/` | Acceptable. Both tools project from the same source content. Run order is documented; no permanent state damage. |
| Concurrent `scribe sync` runs in same project | Existing per-project lockfile in `~/.scribe/state/` already prevents this. No change needed. |

## Laravel Boost interop

Investigation finding (boost research scratchpad, project 18 / id 1316):

- `boost:update` reads `.ai/skills/` and `.ai/guidelines/` as **input only**. It never deletes or overwrites source folders there.
- The destructive copy step happens at *target* agent skill dirs (e.g. `.claude/skills/<name>/`), where Boost rebuilds folders from the source.
- Boost has no kits/packs concept that would collide with `.ai/kits/`.
- Boost's managed-block marker in `CLAUDE.md` is `<laravel-boost-guidelines>...</laravel-boost-guidelines>`. Different name from scribe's `<!-- scribe-snippet:... -->` markers. No marker collision today, even if v2 adds snippet projection.

Implications:

- `.ai/skills/<name>/` is safe for vendored content; Boost will not clobber the source.
- `.claude/skills/<name>/` may be rebuilt by either tool. End state is identical when both project from the same `.ai/skills/<name>/` source. The only observable difference is link-vs-copy, and that doesn't affect agents reading the directory.
- v2 snippet design must use a marker name that doesn't conflict with `<laravel-boost-guidelines>`. Today's marker scheme already satisfies this.

Risk classified as **low**.

## Testing

- **Unit** — lockfile parse/write round-trip, project store loader, kit-merge precedence, vendor detection (project-authored vs registry origin).
- **Integration** — golden-file end-to-end through `testdata/`. Synthetic project with `.scribe.yaml` + `.ai/`, run sync against existing fake-registry test infra, snapshot resulting agent skill dirs.
- **E2E** — anvil worktree pair: author worktree runs `scribe project sync` + commits; teammate worktree runs `scribe sync` and asserts identical skill set in `~/.claude/skills/`.

## Migration

No automatic migration. Existing scribe projects continue working with author-machine-only resolution until the author runs `scribe project sync`. After running, the project becomes team-shareable on the next push.

The `--force` flag covers the upgrade case where a project already has hand-rolled `.ai/skills/<name>/` content predating scribe vendoring; running `scribe project sync --force` adopts those folders into vendoring state.

## Parking lot

- **Snippets in team-share (v2)** — needs design that respects Boost's `<laravel-boost-guidelines>` block and any other tool's managed regions.
- **MCP server definitions** — today the `mcp:` field projects names into `.claude/settings.json`. Sharing the actual MCP server definitions (not just names) requires shipping `.mcp.json` content, which is per-tool and out of scope here.
- **Marker file inside vendored skill folders** — if Boost or another tool ever turns destructive on `.ai/skills/`, we'd add a `.scribe-managed` marker inside vendored folders. Not needed today.
- **CI verification command** — `scribe project sync --check` to fail CI when the repo's lockfile drifts from the intent. Easy follow-up; not v1.

## Open questions

None blocking implementation. The boost-interop and pin-format questions are resolved above.
