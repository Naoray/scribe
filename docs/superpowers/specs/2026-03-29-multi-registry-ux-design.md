# Multi-Registry Sync/List UX

**Issue:** #3
**Date:** 2026-03-29
**Status:** Draft

## Problem

When connected to multiple registries, `scribe sync` silently syncs all and `scribe list` only shows the first registry. There's no way to filter by registry or see which registry a skill belongs to.

## Decisions

| Question | Decision | Rationale (north star: convenience) |
|----------|----------|-------------------------------------|
| Default `sync` behavior | Sync all registries | Most users want everything in sync — don't make them opt in |
| Default `list` behavior | Show all, grouped by registry | Visibility without flags |
| Interactive prompt? | No | Convenience = just do it, don't ask |
| Skill identity | Keyed by skill name (not compound) | Same skill from two registries = one install, not two |
| Version conflicts | Install newer, warn | Don't block sync over a version mismatch |
| `--all` flag | Accepted silently, not advertised | Alias for the default so scripts don't break |

## State Schema Changes

### `InstalledSkill` — add `Registries` field

```go
type InstalledSkill struct {
    Version     string    `json:"version"`
    CommitSHA   string    `json:"commit_sha,omitempty"`
    Source      string    `json:"source"`
    InstalledAt time.Time `json:"installed_at"`
    Targets     []string  `json:"agents"`
    Paths       []string  `json:"paths"`
    Registries  []string  `json:"registries"` // NEW: which registries reference this skill
}
```

Map stays keyed by **skill name** (e.g. `"gstack"`). `Registries` tracks which connected registries declare this skill. If two registries both list `gstack`, one install, two entries in `Registries`.

### Migration

On `state.Load()`, existing skills with a nil/empty `Registries` field get backfilled from `config.TeamRepos[0]` (the only registry that could have existed pre-migration). No user action needed — next `sync` just works.

### `TeamState`

No changes. Single `LastSync` timestamp — syncing all is the default, so one timestamp captures "when did I last sync."

## `scribe sync` Behavior

| Scenario | Behavior |
|----------|----------|
| Single registry connected | Same as today, zero UX change |
| Multiple registries, no flag | Sync all sequentially with per-registry header |
| `--registry owner/repo` | Sync only that registry |
| `--all` | Accepted, same as no flag |
| Non-TTY / `--json` | Sync all, JSON includes `registries` per skill |
| `--registry` unknown repo | Error: `not connected to <repo> — run: scribe connect <repo>` |

### Per-registry header output

```
── ArtistfyHQ/team-skills ──
  ✓ deploy v1.2.0 (current)
  ↑ lint-rules v0.7.0 → v0.8.1

── vercel/skills ──
  + nextjs v2.0.0 (installed)
```

### Registry tracking during sync

When `syncer.Run()` completes for a registry, the cmd layer updates each synced skill's `Registries` slice — appending the current registry if not already present. If a skill was removed from a registry's `scribe.toml`, that registry is removed from `Registries`. A skill with an empty `Registries` slice is pruned from state (it's no longer wanted by any registry).

### Version conflicts

If skill `X` is declared by two registries at different versions, install the **newer** version. During sync output, warn:

```
  ⚠ gstack: ArtistfyHQ/team-skills pins v0.12.9, vercel/skills pins v1.0.0 — using v1.0.0
```

## `scribe list` Behavior

| Scenario | Behavior |
|----------|----------|
| Single registry connected | Same table as today, no grouping |
| Multiple registries, no flag | Grouped by registry with section headers |
| `--registry owner/repo` | Filter to one registry, no section header |
| Non-TTY / `--json` | JSON with `registries` array |

### Grouped output

```
── ArtistfyHQ/team-skills ──
  SKILL       VERSION   STATUS    AGENTS
  deploy      v1.2.0    current   claude, cursor
  lint-rules  v0.8.1    outdated  claude

── vercel/skills ──
  SKILL       VERSION   STATUS    AGENTS
  nextjs      v2.0.0    missing   claude
```

Footer: `ArtistfyHQ/team-skills: 1 current · 1 outdated  ·  vercel/skills: 1 missing`

### Skills in multiple registries

If `gstack` appears in both registries, it shows under **both** groups (since both registries declare it). The version/status is the same in both — it's one installation.

## `--registry` Flag

Shared by `sync` and `list`. Accepts `owner/repo` (full match) or just `repo` (partial match — matches on repo name if exactly one connected registry matches). Case-insensitive.

Partial matching example: `--registry team-skills` matches `ArtistfyHQ/team-skills` if that's the only connected registry with repo name `team-skills`.

If partial match is ambiguous (two registries share the repo name), error with suggestions:

```
ambiguous registry "skills" — did you mean:
  ArtistfyHQ/skills
  vercel/skills
```

## JSON Output

### `scribe sync --json`

```json
{
  "registries": [
    {
      "registry": "ArtistfyHQ/team-skills",
      "skills": [
        {
          "name": "deploy",
          "version": "v1.2.0",
          "status": "current",
          "agents": ["claude", "cursor"]
        }
      ]
    }
  ]
}
```

### `scribe list --json`

Same structure. This replaces the current flat `skills` array with a `registries` array. Breaking change — acceptable pre-1.0.

## Non-TTY

All behavior follows the existing `isatty` pattern:
- `isatty(stdout)` false → auto-JSON (existing)
- `isatty(stdin)` false → no prompts (existing, no new prompts added)
- `--registry` works in both TTY and non-TTY

## Out of Scope

- Interactive registry selection prompts (convenience = just do it all)
- Per-registry sync timestamps (YAGNI)
- `--all` as an advertised flag (accepted silently)
- Skill conflict resolution UI (newer wins + warning is sufficient)
- Registry column in table (grouped headers make a column redundant)
