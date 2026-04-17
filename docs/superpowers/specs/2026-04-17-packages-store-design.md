# Packages Store Design

Date: 2026-04-17
Status: Proposed

## Problem

Repos installed via `scribe install <owner>/<repo>` are not always skills. Some
are multi-skill bundles / self-installing CLI toolkits. Current example:
`Naoray/gstack`. Its repo contains:

- A root `SKILL.md`
- Dozens of sibling directories each with their own `SKILL.md` (`codex/`,
  `ship/`, `browse/`, …)
- Tool-specific projections (`.cursor/skills/…`, `.openclaw/…`, `.codex/…`)
- Build artifacts (`node_modules/`, `bin/`, `scripts/`)

Scribe stores the whole tree under `~/.scribe/skills/gstack/` and creates a
directory symlink from `~/.codex/skills/gstack` → canonical. Codex then walks
the target recursively and trips on every nested `SKILL.md` — several of which
have invalid YAML frontmatter (unquoted colons in `description:`). Result:

```
⚠ /Users/.../gstack/openclaw/skills/gstack-openclaw-office-hours/SKILL.md:
  invalid YAML: mapping values are not allowed in this context
⚠ Skipped loading 3 skill(s) due to invalid SKILL.md files.
```

Fixing the YAML treats the symptom. Root cause: scribe is projecting an
entire package repo as if it were a single skill.

## Proposal

Split storage into two top-level kinds under `~/.scribe/`:

```
~/.scribe/
  skills/<name>/     # single-authorship skills; dir-symlinked into tool skill dirs (today's behavior)
  packages/<name>/   # multi-skill bundles / self-installing toolkits; NOT projected
```

Codex and Claude only walk their own skill dirs (`~/.codex/skills/`,
`~/.claude/skills/`). Packages never live there → zero nested-skill leak.

Packages own their tool wiring. They run their declared install script once
when scribe installs them. Scribe tracks their state for list / update /
remove — it does not symlink their contents into agent skill dirs.

### Detection rule (automatic, no config required)

On `scribe install <ref>`:

1. Fetch repo / payload into a staging area.
2. Walk for any `SKILL.md` below the root (`find ./*/**/SKILL.md`).
   - Has nested `SKILL.md` → **package**.
   - No `SKILL.md` at root but repo has install script (`setup`, `install.sh`,
     package.json "scripts.install", Makefile `install` target) → **package**.
   - Otherwise (root SKILL.md and no nested) → **skill**.
3. Route to the matching store:
   - skill → `~/.scribe/skills/<name>/` (today's flow, unchanged)
   - package → `~/.scribe/packages/<name>/` (new flow)

No manifest flag required. Users can override with `kind: package` in
`scribe.yaml` if auto-detection is wrong.

### Package install flow

```
Fetch repo → stage/
  detect → package
  move stage/ → ~/.scribe/packages/<name>/
  run install command (see order below) in package dir
  record state entry kind=package, paths=[package dir], tools=[] (no projection)
```

Install command resolution, first match wins:
1. `scribe.yaml` → `install.command` field
2. Executable `setup` at repo root
3. `install.sh` at repo root
4. `package.json` → `scripts.install` via `bun install` / `npm install`
5. `Makefile` target `install`
6. No install command → no-op, still tracked

Stdout/stderr from the install command streams to scribe output as an event;
non-zero exit fails the install and rolls back the package dir.

### Package uninstall flow

`scribe remove <name>` for a package:
1. Run uninstall command if declared (`scribe.yaml` → `install.uninstall`, or
   `uninstall.sh` at repo root). Best-effort — non-zero exit logged, not
   aborted.
2. Delete `~/.scribe/packages/<name>/`.
3. Delete state entry.

No tool projection cleanup needed — none was created.

### Migration for existing installs

First `scribe sync` after upgrade:

For each state entry in `kind=skill` (legacy default):
- If canonical dir is in `skills/` AND contains nested `SKILL.md` → reclassify:
  1. Move `~/.scribe/skills/<name>/` → `~/.scribe/packages/<name>/`.
  2. Remove tool projection symlinks (`~/.claude/skills/<name>`, etc.).
  3. Update state entry: `kind=package`, clear `tools`/`paths`, new
     `canonical_dir`.
  4. Emit `PackageReclassifiedMsg{Name, OldPath, NewPath, InstallHint}`
     carrying a hint to run the package's setup if needed (we don't auto-run
     during migration — user chose to install when it was a skill, re-running
     setup is their call).

No repair required for clean skill installs — unchanged.

### Non-projection invariant

Packages must never be symlinked into `~/.{claude,codex,cursor,…}/skills/`.
Reconcile gains a guard: if state entry `kind=package`, skip projection
entirely; if a stale projection is detected pointing at a package canonical,
remove it and emit a repair event.

### State schema

Add `kind` field to installed entries:

```json
{
  "installed": {
    "gstack": {
      "kind": "package",
      "canonical_dir": "/Users/.../.scribe/packages/gstack",
      "installed_at": "...",
      "install_command": "./setup",
      "paths": [],
      "tools": []
    },
    "brand-guidelines": {
      "kind": "skill",
      "canonical_dir": "/Users/.../.scribe/skills/brand-guidelines",
      "paths": ["/Users/.../.claude/skills/brand-guidelines"],
      "tools": ["claude"]
    }
  }
}
```

Missing `kind` on legacy entries is treated as `skill` (migration rule above
may flip them).

### List output

TUI and JSON output separate into two sections:

```
Skills  (42)
  brand-guidelines    Apply Anthropic brand colors            v1.0.0   claude, codex
  canvas-design       Create visual art in .png/.pdf          v1.2.0   claude

Packages  (2)
  gstack              Fast headless browser QA + toolkit      v3.1.0   self-managed
  mattpocock/skills   Matt's TypeScript skill bundle          v0.4.0   self-managed
```

JSON:
```json
{
  "skills":   [ ... ],
  "packages": [ ... ]
}
```

Packages display `self-managed` in the tools column because scribe does not
project them.

## Alternatives considered

1. **Quote description YAML on install** — fixes the symptom, not the cause;
   we'd still be exposing dozens of unintended skills from every package repo.
2. **Pruned-view projection** (earlier draft) — build a real target dir per
   tool, symlink only non-nested-SKILL entries. Works but "many symlinks to
   recreate a skill dir" is odd and fragile; explicit kind split is cleaner.
3. **Projection allowlist in scribe.yaml** — forces every package author to
   declare what to project. Too much burden; breaks convenience-first.

## Open questions

- Does `scribe.yaml` still apply to packages? Likely only `install.command` and
  `uninstall.command` fields are meaningful; other fields (version, files)
  ignored. TBD in impl.
- Should packages have an `update` command (re-run install) or just git
  pull + re-run install? Proposal: `scribe sync` pulls + re-runs install if
  HEAD sha changed.

## Scope

In scope:
- Detection + routing
- Package install/remove flows
- Migration for existing gstack-style installs
- List UI / JSON split
- Reconcile guard for packages
- Tests for each

Out of scope for this PR:
- Package sub-skill discovery (listing gstack's inner skills in scribe list)
- `scribe install --kind=package` flag override (auto-detection covers MVP)
- Cross-agent package manifest format (beyond `install.command`)
