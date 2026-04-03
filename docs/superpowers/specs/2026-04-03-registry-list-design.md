# Registry List Command

**Date:** 2026-04-03
**Branch:** `feat/registry-list`

## Summary

Add `scribe registry list` to show connected registries with skill counts and last sync time. First subcommand of a new `registry` command group.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Command structure | `scribe registry list` (not `scribe registries`) | Opens the door for `registry add`, `registry remove` later |
| Output format | Label-free, one line per registry | Self-describing values: `owner/repo (N) - time ago` |
| Data source | Config + state only (offline) | No GitHub API calls — fast, works offline |
| No TUI | Styled text or JSON only | Too little data for an interactive view |

## 1. Command Wiring

### `cmd/registry.go` — parent command group

```go
var registryCmd = &cobra.Command{
    Use:   "registry",
    Short: "Manage connected skill registries",
}

func init() {
    registryCmd.AddCommand(registryListCmd)
}
```

No `RunE` — bare `scribe registry` prints help automatically.

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
LoadConfig → LoadState → StepPrintRegistryList
```

Reuses existing `StepLoadConfig` and `StepLoadState`.

### `StepPrintRegistryList`

Reads `b.Config.TeamRepos` and `b.State.Installed` to compute per-registry skill counts.

**Skill counting:** For each registry in `Config.TeamRepos`, count installed skills whose `Registries` slice contains that repo (case-insensitive match via `strings.EqualFold`).

**Last sync:** Uses `b.State.Team.LastSync` (global — not per-registry today).

**Time formatting:** Relative time ("2 hours ago", "3 days ago", "just now"). Simple helper — no external dependency. Falls back to absolute date if > 30 days ago.

## 3. TTY Output

```
ArtistfyHQ/skills (12) - 2 hours ago
Naoray/my-skills (3) - 2 hours ago

2 registries connected
```

- One line per registry, no header row
- Format: `owner/repo (count) - relative_time`
- Styled: repo name bold, count and time dimmed
- Footer: total count in dimmed style
- If `LastSync` is zero (never synced): omit the time portion, show `owner/repo (0) - never synced`

### Empty state

```
No registries connected.

  Connect a registry:  scribe connect <owner/repo>
```

## 4. JSON Output

Auto-JSON when stdout is not a TTY, explicit via `--json`:

```json
[
  {
    "registry": "ArtistfyHQ/skills",
    "skills": 12,
    "last_sync": "2026-04-03T10:00:00Z"
  },
  {
    "registry": "Naoray/my-skills",
    "skills": 3,
    "last_sync": "2026-04-03T10:00:00Z"
  }
]
```

- `last_sync` uses RFC 3339 format
- `last_sync` is `null` if never synced
- Empty registries list outputs `[]`

## 5. Files to Create/Modify

| File | Action |
|------|--------|
| `cmd/registry.go` | New — parent `registry` command |
| `cmd/registry_list.go` | New — `list` subcommand with `--json` flag |
| `cmd/root.go` | Add `registryCmd` to root |
| `internal/workflow/registry_list.go` | New — steps + styled table + JSON rendering |

## Out of Scope

- `registry add` / `registry remove` subcommands
- Per-registry last-sync tracking (uses global sync time for now)
- TUI / interactive view
- GitHub API calls for remote registry metadata
