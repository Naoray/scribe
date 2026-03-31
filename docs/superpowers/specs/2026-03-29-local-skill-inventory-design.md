# Local Skill Inventory — Design Spec

**Issue:** #22
**Date:** 2026-03-29

## Problem

`scribe list` requires a registry connection and only shows skills relative to the team loadout. Users cannot see what's installed on their machine without a working registry connection. This blocks offline use and hides the local picture.

## Solution

Two changes to `scribe list`:

1. **`--local` flag** — always shows the local skill inventory from `state.json`, skipping all GitHub API calls
2. **Graceful fallback** — when no registries are connected, show the local inventory instead of erroring

## Behavior Matrix

| Scenario | Output |
|---|---|
| `scribe list` with registries | Current behavior — remote diff (`SKILL \| VERSION \| STATUS \| AGENTS`) |
| `scribe list` with no registries | Local inventory + hint to connect a registry |
| `scribe list --local` | Local inventory (ignores registries even if connected) |
| `scribe list --local --registry X` | Error: `--local` and `--registry` are mutually exclusive |
| `scribe list --local --json` | Local inventory as JSON |
| `scribe list` piped (no registries) | Local inventory as JSON (auto-detect non-TTY) |

## Local View — Table Output (TTY)

```
SKILL         VERSION        TARGETS         SOURCE
gstack        v0.12.9.0      claude, cursor  github:garrytan/gstack
laravel-init  main@a3f2c1b   claude          github:Naoray/scribe-skills
deploy        main@e4f8a2d   claude          github:ArtistfyHQ/team-skills
```

Columns:
- **SKILL** — skill name (map key from `state.Installed`)
- **VERSION** — `InstalledSkill.DisplayVersion()` (tag or `branch@sha`)
- **TARGETS** — comma-separated `InstalledSkill.Targets` (e.g. `claude, cursor`)
- **SOURCE** — `InstalledSkill.Source` with the `@ref` suffix stripped for brevity

Sorted alphabetically by skill name.

## Local View — JSON Output

```json
[
  {
    "name": "gstack",
    "version": "v0.12.9.0",
    "source": "github:garrytan/gstack@v0.12.9.0",
    "targets": ["claude", "cursor"],
    "installed_at": "2026-03-28T14:30:00Z",
    "registries": ["ArtistfyHQ/team-skills"]
  }
]
```

Full `InstalledSkill` fields, plus `name` from the map key. Sorted alphabetically by name.

## Empty State

**TTY:**
```
No skills installed.

  Install skills from a registry:  scribe connect <owner/repo>
```

**JSON:**
```json
[]
```

## Implementation

### Files Modified

- **`cmd/list.go`** — add `--local` flag, change no-registry path to call local renderer, mark `--local` and `--registry` as mutually exclusive
- No new packages — uses existing `state.Load()` and `InstalledSkill.DisplayVersion()`

### Data Flow

```
--local flag or no registries
  → state.Load()
  → sort map keys alphabetically
  → if TTY: render table (SKILL | VERSION | TARGETS | SOURCE)
  → if non-TTY or --json: marshal to JSON array
```

### Flag Interaction

```go
listCmd.MarkFlagsMutuallyExclusive("local", "registry")
```

## Scope Boundaries

**In scope:**
- `--local` flag on `scribe list`
- Graceful fallback when no registries connected
- Table and JSON rendering of local inventory

**Out of scope (future issues):**
- `scribe install <source>` — direct skill installation without a registry
- `scribe scan` — discover unmanaged skills in target directories
- Modifying `scribe add` behavior
