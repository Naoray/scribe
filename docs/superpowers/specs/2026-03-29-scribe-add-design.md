# scribe add — Interactive Skill Discovery & Installation

**Date:** 2026-03-29
**Status:** Draft

## Problem

Users have skills on their machine (authored locally or synced from other registries) that they want to share with their team. There's no way to add a skill to a team registry without manually editing `scribe.toml` on GitHub.

## Decisions

| Question | Decision | Rationale (north star: convenience) |
|----------|----------|-------------------------------------|
| What does "add" mean? | Add a skill to a team registry's `scribe.toml` on GitHub | The registry is the source of truth for the team |
| Skill name resolves to? | A locally installed skill (either `~/.claude/skills/` or `~/.scribe/skills/`) | Users add what they already have |
| Discovery source | Local disk + all connected registries (merged, deduplicated) | Most complete — works for single and multi-registry users |
| Source vs upload? | Has source in state → add reference; local-only → upload files | Auto-detect, user doesn't think about it |
| Multi-registry target? | Interactive picker if TTY; `--registry` required if non-TTY | Convenience for humans, explicitness for scripts |
| TOML re-encoding | Accept reformatting (full encode/decode cycle) | Machine-managed manifest, consistent formatting is a feature |
| Ordering in browse | Local skills first, then remote, alphabetical within each | Simple, no usage tracking needed for v1 |

## Command Interface

```
scribe add [name] [flags]

Flags:
  --registry owner/repo   Target registry (required if multiple connected, picker if TTY)
  --yes                   Skip confirmation prompt
  --json                  JSON output (auto-enabled when stdout is not TTY)
```

### Mode 1: `scribe add cleanup` (name provided)

1. Resolve skill by name: scan `~/.claude/skills/`, `~/.scribe/skills/`, then state entries
2. If not found → error: `skill "cleanup" not found locally or in connected registries`
3. Determine strategy: has source → reference; no source → upload
4. If multiple registries and no `--registry` → picker (TTY) or error (non-TTY)
5. Confirm unless `--yes` (show: skill name, source/upload, target registry)
6. Push to registry's `scribe.toml` via GitHub API
7. Auto-sync to install locally

### Mode 2: `scribe add` (no args, TTY)

1. Resolve all discoverable skills, filter to "not in target registry"
2. Show Bubble Tea list — searchable, multi-select
3. User selects one or more, confirms
4. Push all to registry, auto-sync

### Mode 3: `scribe add` (no args, non-TTY)

Error: `skill name required when not running interactively`

## Skill Resolution

### Resolution chain

```
1. Scan ~/.claude/skills/*/   → local skills (may or may not have scribe state)
2. Scan ~/.scribe/skills/*/   → scribe-managed skills (always have state)
3. Fetch scribe.toml from all connected registries → remote skills
4. Deduplicate by name (local wins over remote)
5. Filter out skills already in the target registry's scribe.toml
```

A directory counts as a skill if it exists and is non-empty. The skill name is the directory basename (e.g. `~/.claude/skills/cleanup/` → `cleanup`).

### Add strategy by origin

| Origin | Has Source? | Action |
|--------|-----------|--------|
| `~/.scribe/skills/<name>/` | Yes | Add source reference to target `scribe.toml` |
| `~/.claude/skills/<name>/` with state entry | Yes | Add source reference |
| `~/.claude/skills/<name>/` without state entry | No | Upload files to registry + add self-referencing entry |
| Remote (in another registry, not installed locally) | Yes | Add source reference |

### Self-referencing entry for uploaded skills

When uploading local-only files, the `scribe.toml` entry points back at the registry:

```toml
[skills]
cleanup = { source = "github:ArtistfyHQ/team-skills@main", path = "skills/cleanup" }
```

The registry becomes both manifest and host. The `path` field distinguishes uploaded skills from external references.

## GitHub Write Operations

### Add source reference (read-modify-write)

1. Fetch current `scribe.toml` from registry (`client.FetchFile`)
2. Parse it (`manifest.Parse`)
3. Add new skill entry to `Manifest.Skills`
4. Re-encode to TOML
5. Commit via `client.PushFiles` (single file: `scribe.toml`)

### Upload files + add self-reference

1. Read all files from the local skill directory
2. Fetch + parse current `scribe.toml`
3. Add self-referencing entry
4. Commit via `client.PushFiles` (skill files + updated `scribe.toml` — one atomic commit)

`PushFiles` already supports multi-file atomic commits via the Git Trees API.

## Architecture

### New package: `internal/add/`

UI-agnostic core following the `Emit func(any)` pattern from `internal/sync/`.

```go
type Adder struct {
    Client  *gh.Client
    Targets []targets.Target
    Emit    func(any)
}

func (a *Adder) Discover(ctx context.Context, targetRepo string, cfg *config.Config, st *state.State) ([]Candidate, error)

func (a *Adder) Add(ctx context.Context, targetRepo string, candidates []Candidate) error
```

```go
type Candidate struct {
    Name      string
    Origin    string   // "local" or "registry:owner/repo"
    Source    string   // "github:owner/repo@ref" or empty for local-only
    LocalPath string   // path on disk, empty for remote-only
    Upload    bool     // true if files need uploading
}
```

### Event types

| Type | Fields | When |
|------|--------|------|
| `SkillDiscoveredMsg` | `Name, Origin, Source, LocalPath` | During discovery |
| `RegistrySelectedMsg` | `Registry` | After registry selection |
| `SkillAddingMsg` | `Name, Upload` | About to push |
| `SkillAddedMsg` | `Name, Registry, Source, Upload` | Committed |
| `SkillAddErrorMsg` | `Name, Err` | Failed |
| `AddCompleteMsg` | `Added, Failed, SyncStarted` | Done |

### Wiring

`cmd/add.go` wires core to output:
- TTY interactive → Bubble Tea list (inline in `cmd/`, consistent with `connect.go`'s use of `huh`)
- TTY with name arg → plain text with confirmation prompt
- Non-TTY → plain text or JSON

After `Add` completes, `cmd/` calls `syncer.Run()` to install locally — reuses existing sync machinery.

## Interactive TUI

```
┌─────────────────────────────────────────┐
│ Add skills to ArtistfyHQ/team-skills    │
│ > Search: _                             │
│                                         │
│ LOCAL                                   │
│   [x] cleanup        ~/.claude/skills   │
│   [ ] deploy-check   ~/.scribe/skills   │
│                                         │
│ FROM vercel/skills                      │
│   [ ] nextjs         github:vercel/...  │
│                                         │
│ ↑↓ navigate · space select · enter add  │
└─────────────────────────────────────────┘
```

- Two groups: **Local** (from disk) and **From \<registry\>** (one section per other connected registry)
- Search filters across both groups by name
- `space` toggles, `enter` confirms
- After confirm: summary + `y/n` unless `--yes`
- Empty state: "All available skills are already in \<registry\>."

## JSON Output

```json
{
  "added": [
    {
      "name": "cleanup",
      "registry": "ArtistfyHQ/team-skills",
      "source": "github:garrytan/gstack@v1.0.0",
      "uploaded": false
    }
  ],
  "synced": true
}
```

For uploaded skills: `"uploaded": true`, `"source"` shows the self-reference.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Not connected to any registry | Error: `no registries connected — run: scribe connect <owner/repo>` |
| Skill already in target registry | Skip: `cleanup is already in ArtistfyHQ/team-skills` |
| Skill name not found | Error: `skill "cleanup" not found locally or in connected registries` |
| Auth missing/expired | Existing `wrapErr()`: auth required message |
| Push fails (permissions, conflict) | Error with reason — no partial corruption (`PushFiles` is atomic) |
| Upload skill has no `SKILL.md` | Warn but proceed — a directory of `.md` files is valid |
| Multiple registries, no `--registry`, non-TTY | Error: `multiple registries connected — pass --registry owner/repo` |
| Sync after add fails | Warn but don't fail — skill was added to registry. User can `scribe sync` manually. |

### Concurrency

Simultaneous `scribe add` to the same registry: second `PushFiles` fails on stale base tree SHA. Error message tells user to retry. Git Trees API guarantees no silent data loss.

## Out of Scope

- Usage tracking / popularity ordering (v2)
- `scribe remove` (inverse of add — separate feature)
- Local skill indexing on install/setup (tracked as future work — scribe should become the single manager for all AI skills on the machine)
- Bubble Tea extraction to `internal/ui/` (do when more commands get TUI treatment)
- Skill name conflict resolution across registries (same skill name, different content)
