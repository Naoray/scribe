# Local-Only Skill Usage Analytics — Design

Status: draft v1 (2026-05-17)
Scope: local state and local reporting only. No remote upload.

## Problem

Scribe can install, sync, list, update, and project skills across local AI coding agent tool directories, but it does not currently answer basic local questions:

- Which skills have I installed or updated most recently?
- Which skills are repeatedly projected by `scribe sync`?
- Which registries or local skills are most active on this machine?
- Which skills look unused based on Scribe-observable activity?

The feature request is deliberately local-only. The data must stay on the user's machine by default and must not be conflated with public registry visibility or any future opt-in registry telemetry.

## Goals

1. Record local Scribe-observable skill activity without sending it anywhere.
2. Keep usage analytics separate from public registry discovery and registry visibility state.
3. Avoid collecting prompts, code contents, local project paths, secrets, AI tool names, or public telemetry identifiers.
4. Expose an agent-friendly read surface for local summaries and reset/export controls.
5. Make runtime "skill used" limitations explicit so the implementation does not pretend to observe agent behavior it cannot see.

## Non-Goals

- No remote upload, background sync, public registry reporting, or phone-home behavior.
- No reuse of public registry telemetry identifiers, visibility fields, or registry index files.
- No collection of prompt text, file contents, project paths, secret values, or AI tool names.
- No runtime agent hook implementation in v1.
- No per-user or cross-machine identity.

## Current Observability

Inspected code paths:

- `internal/state/state.go`: `State` is stored at `~/.scribe/state.json`, schema version 6, with installed skills, kits, snippets, projection metadata, removal intent, registry failures, update checks, and vendor state.
- `internal/workflow/bag.go`: workflow steps share a `Bag` with loaded config/state, selected skills, kit filters, project root, formatter, and dirty-state tracking.
- `internal/workflow/sync.go`: `SyncSteps` and `SyncTail` converge skills and projections. `StepSyncSkills` sees resolved, skipped, downloaded, installed, error, and budget messages from `sync.Syncer`.
- `internal/workflow/install.go`: `InstallSteps` selects explicit or interactive skills and then routes through `StepSyncSkills`.
- `internal/workflow/list.go` and `internal/workflow/list_load.go`: local and remote list paths can observe list invocations, local inventory shape, and state repair writes.
- `cmd/list.go`, `cmd/sync.go`, `cmd/install.go`, `cmd/add.go`, `cmd/update.go`: CLI entrypoints can classify command-level proxy events.
- `internal/sync/events.go`, `internal/add/events.go`, `internal/adopt/events.go`: existing event structs cover install/sync/adopt progress, but not runtime skill usage.
- `internal/registryindex/index.go` and `docs/registry-visibility.md`: public registry visibility/index state is local plumbing for future discovery and must remain separate from local usage analytics.

Conclusion: Scribe can observe CLI-side proxy events it owns. It cannot currently observe that an AI agent actually selected or applied a skill at runtime. A "skill used" counter would be inaccurate unless a future runtime hook is added outside the current CLI projection model.

## Event Model

Create a new `internal/usage` package with an append-only local event log:

```go
type Event struct {
    Version   int       `json:"version"`
    ID        string    `json:"id"`
    Kind      string    `json:"kind"`
    Skill     string    `json:"skill,omitempty"`
    Registry  string    `json:"registry,omitempty"`
    SourceKey string    `json:"source_key,omitempty"`
    Origin    string    `json:"origin,omitempty"`
    Result    string    `json:"result,omitempty"`
    Count     int       `json:"count,omitempty"`
    At        time.Time `json:"at"`
}
```

Allowed `kind` values for v1:

- `skill_installed`: a skill was installed by `sync`, `install`, `add`, `browse --install`, or registry install paths.
- `skill_updated`: a previously installed skill revision changed.
- `skill_projected`: `sync` projected or refreshed a skill into local tool directories. This is a proxy event, not runtime usage.
- `skill_listed`: `scribe list` or `scribe list --json` included the skill in local output. This is a discovery proxy, not runtime usage.
- `skill_removed`: user removed a skill through Scribe.
- `registry_synced`: registry sync completed for a registry, with aggregate counts only.

Explicitly disallowed fields:

- prompt text
- code snippets or file contents
- local project paths
- absolute projection paths
- environment variables
- secrets
- AI tool names
- stable machine/user IDs

Do not record `ProjectionEntry.Project`, `ProjectionConflict.Path`, or `InstalledSkill.ManagedPaths`. They may contain local paths.

## Storage

Store usage under a dedicated local directory:

```text
~/.scribe/usage/
├── events.jsonl
└── summary.json
```

This keeps analytics separate from:

- `~/.scribe/state.json`
- `~/.scribe/config.yaml`
- `~/.scribe/index/registries.json`
- registry visibility metadata

`events.jsonl` is the source of truth. `summary.json` is a cache that can be rebuilt. Use the same atomic-write pattern already used by `internal/registryindex.Save` and state save paths. Event writes should be best-effort and must not fail the user command unless the user explicitly runs a usage maintenance command.

Suggested retention defaults:

- Keep 180 days of events.
- Keep aggregate all-time counters in `summary.json`.
- Prune on write when the file is opened, bounded to once per day by a timestamp in the usage directory.

## Privacy UX

The first reporting command should state plainly:

```text
Local usage stats are stored only on this machine under ~/.scribe/usage/.
They are not uploaded. Runtime agent skill use is not observable yet; projected/listed counts are Scribe CLI proxy events.
```

No prompt is required to collect local-only events because collection remains on-machine and excludes sensitive content. Users still need controls:

- `scribe usage` or `scribe stats`: read-only summary.
- `scribe usage reset`: delete `~/.scribe/usage/events.jsonl` and `summary.json`.
- `scribe usage export --json`: print local usage data to stdout for the user to redirect.
- `scribe usage doctor`: validate parseability and report corrupt files with the exact local path.

Prefer `usage` over `telemetry` in command names. "Telemetry" implies remote reporting and conflicts with the separation from opt-in public registry telemetry.

## CLI Surface

Recommended v1 command:

```text
scribe usage [--json] [--since 30d] [--skill <name>] [--registry <repo>]
scribe usage reset [--yes] [--json]
scribe usage export --json [--since 30d]
```

Default text columns:

```text
SKILL        INSTALLED  UPDATED  PROJECTED  LISTED  LAST_OBSERVED
review       1          2        17         4       2026-05-17
```

JSON shape:

```json
{
  "format_version": 1,
  "scope": "local-only",
  "runtime_usage_observable": false,
  "retention_days": 180,
  "skills": [
    {
      "name": "review",
      "registry": "owner/repo",
      "installed": 1,
      "updated": 2,
      "projected": 17,
      "listed": 4,
      "last_observed_at": "2026-05-17T09:00:00Z"
    }
  ]
}
```

## Implementation Plan

1. Add `internal/usage` with event types, path helpers, append, load, summarize, prune, reset, and tests using temporary `SCRIBE_HOME`/path overrides.
2. Add usage recorder hooks at command-owned boundaries:
   - `StepSyncSkills`: record installed, updated, projected, and registry aggregate events from existing sync messages and filters.
   - `runList` JSON/local load path and TUI model load path: record `skill_listed` for local list results only.
   - remove/update command paths: record removed or updated events after state writes succeed.
3. Add `cmd/usage.go` with `newUsageCommand`, JSON envelope support, reset/export subcommands, and command schema tests.
4. Document privacy behavior in `docs/commands.md` and `docs/registry-visibility.md` with an explicit separation note.
5. Add tests covering:
   - event redaction and absence of path/tool/prompt fields
   - append/load/summarize/prune/reset
   - `scribe usage --json`
   - corrupt event-file behavior
   - sync/list proxy events do not include local project paths or tool names

## Separation From Public Telemetry

Public registry visibility currently stores local registry metadata in `~/.scribe/index/registries.json` and is documented as local plumbing. Local usage analytics must not write to that index or depend on registry visibility values except to display already-known registry names from state.

If a future ADR adds opt-in public telemetry, it must use a different package, file path, schema, consent flow, and identifiers. Local usage event IDs should be per-event random IDs, not stable user, machine, registry, or telemetry IDs.

## Open Questions

- Should `skill_listed` be recorded by default? It is useful for local discovery stats but may inflate "interest" counts. The summary should label it as listed, not used.
- Should the default command be `scribe usage` or `scribe stats`? Recommendation: `usage`, because it is specific and avoids overloaded operational metrics.
- Should users be able to disable local event recording entirely? A simple config flag such as `usage.local_enabled: false` is reasonable, but v1 can start with reset/export controls if product wants fewer settings.

## North Star Fit

Project north star is not set. Local-only usage analytics supports Scribe as a trustworthy local skill manager: it helps users understand their skill inventory without weakening privacy or registry trust boundaries. The main deviation risk is terminology; avoid "telemetry" for this feature unless a future public-telemetry ADR defines a separate opt-in surface.
