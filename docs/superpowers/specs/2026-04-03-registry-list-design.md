# Registry List Command

**Date:** 2026-04-03
**Branch:** `feat/registry-list`

## Summary

Add `scribe registry list` to show connected registries with skill counts and last sync time. First subcommand of a new `registry` command group.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Command structure | `scribe registry list` (not `scribe registries`) | Opens the door for `registry add`, `registry remove` later |
| Bare `scribe registry` | Delegates to `list` | Convenience first — only one subcommand exists, don't force users through a help screen |
| Output format | Label-free, one line per registry | Self-describing values: `owner/repo (N)` |
| Sync time | Footer only, not per-registry | `LastSync` is global — repeating it per line is misleading |
| JSON envelope | `{"registries": [...], "last_sync": ...}` | Matches `scribe list` pattern, extensible |
| Data source | Config + state only (offline) | No GitHub API calls — fast, works offline |
| No TUI | Styled text or JSON only | Too little data for an interactive view |

## 1. Command Wiring

### `cmd/registry.go` — extend existing file

This file already contains `resolveRegistry` and `filterRegistries` helpers. Add the parent command and `list` subcommand here.

```go
var registryCmd = &cobra.Command{
    Use:   "registry",
    Short: "Manage connected skill registries",
    RunE:  runRegistryList, // delegate bare command to list
    Args:  cobra.NoArgs,
}
```

Bare `scribe registry` delegates to `runRegistryList` directly. When more subcommands are added later, this can be changed to print help instead.

### `cmd/registry_list.go` — list subcommand

```go
var registryListCmd = &cobra.Command{
    Use:   "list",
    Short: "Show connected registries",
    RunE:  runRegistryList,
}
```

Flags: `--json` for explicit JSON output.

Reads flags, builds a `workflow.Bag` with `JSONFlag`, calls `workflow.Run(ctx, workflow.RegistryListSteps(), bag)`.

### `cmd/root.go`

Add `registryCmd` to `rootCmd.AddCommand(...)`.

## 2. Workflow

### `internal/workflow/registry_list.go`

Steps:

```
LoadConfig → LoadState → MigrateRegistries → StepPrintRegistryList
```

Reuses existing `StepLoadConfig`, `StepLoadState`, and `StepMigrateRegistries`.

**Migration guard:** `StepMigrateRegistries` calls `b.Config.TeamRepos[0]`, so the step must be skipped when `len(TeamRepos) == 0`. `StepPrintRegistryList` handles the empty case directly (prints empty-state message).

### `StepPrintRegistryList`

Reads `b.Config.TeamRepos` and `b.State.Installed` to compute per-registry skill counts.

**Skill counting:** For each registry in `Config.TeamRepos`, count installed skills whose `Registries` slice contains that repo (case-insensitive match via `strings.EqualFold`).

**Migration note:** After `MigrateRegistries`, legacy skills are attributed to `TeamRepos[0]` only. Secondary registries may show 0 skills until a full multi-registry sync runs. This is a model truth, not a counting bug.

**Last sync:** Uses `b.State.Team.LastSync` (global — not per-registry today).

**Time formatting:** Relative time helper with 5 buckets:
- `< 1 minute` -> "just now"
- `< 1 hour` -> "N minutes ago"
- `< 24 hours` -> "N hours ago"
- `< 30 days` -> "N days ago"
- `>= 30 days` -> absolute date "2026-03-01"

## 3. TTY Output

```
ArtistfyHQ/skills (12)
Naoray/my-skills (3)

2 registries connected · last sync 2 hours ago
```

- One line per registry, no header row
- Format: `owner/repo (count)`
- Styled: repo name bold, count dimmed
- Footer: total count + last sync time, dimmed
- If `LastSync` is zero (never synced): footer shows `N registries connected · never synced`

### Empty state

```
No registries connected.

  Connect a registry:  scribe connect <owner/repo>
```

## 4. JSON Output

Auto-JSON when stdout is not a TTY, explicit via `--json`:

```json
{
  "registries": [
    {
      "registry": "ArtistfyHQ/skills",
      "skill_count": 12
    },
    {
      "registry": "Naoray/my-skills",
      "skill_count": 3
    }
  ],
  "last_sync": "2026-04-03T10:00:00Z"
}
```

- `last_sync` at top level (global, not per-registry) in RFC 3339 format
- `last_sync` is `null` if never synced
- `skill_count` (not `skills`) to avoid ambiguity with other commands that use `skills` for arrays
- Empty registries: `{"registries": [], "last_sync": null}`

## 5. Testing

- Unit test for relative time helper (all 5 buckets + zero time)
- Unit test for skill counting logic (including nil/empty `Registries` field, multi-registry membership)
- Unit test for "never synced" path
- Workflow integration test: migration on/off, empty config
- JSON output shape validation

## 6. Files to Create/Modify

| File | Action |
|------|--------|
| `cmd/registry.go` | Extend — add `registryCmd` parent command with `RunE` delegating to list |
| `cmd/registry_list.go` | New — `list` subcommand with `--json` flag |
| `cmd/root.go` | Add `registryCmd` to root |
| `internal/workflow/registry_list.go` | New — steps + styled output + JSON rendering + relative time helper |

## Out of Scope

- `registry add` / `registry remove` subcommands
- Per-registry last-sync tracking (uses global sync time for now)
- TUI / interactive view
- GitHub API calls for remote registry metadata
