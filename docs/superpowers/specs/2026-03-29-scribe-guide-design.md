# `scribe guide` — Interactive Onboarding Command

## Problem

New users install Scribe and face a blank terminal. They don't know which commands to run, in what order, or what prerequisites exist. The README has a quickstart, but nobody reads READMEs when they have a binary in hand. Agents using Scribe skills face the same problem — they need a machine-readable way to discover the setup steps.

## Solution

A permanent `scribe guide` command that walks users (and agents) through Scribe setup interactively. Not a one-time wizard — always available, adapts to current state.

## Command Surface

```
scribe guide [--json]
```

- **No positional args, no required flags.**
- **TTY mode:** interactive guided flow (huh forms + Bubble Tea progress + lipgloss output).
- **Non-TTY / `--json`:** structured JSON output with prereq status and step-by-step commands for agents to execute.
- Always available — re-running adapts to current state (skips satisfied prereqs, adjusts available paths).

### Non-TTY JSON Output

```json
{
  "status": "not_connected",
  "prerequisites": {
    "github_auth": {"ok": true, "method": "gh_cli"},
    "scribe_dir": {"ok": true, "path": "~/.scribe"}
  },
  "steps": [
    {"command": "scribe connect ArtistfyHQ/team-skills", "description": "Connect to your team's skill registry"},
    {"command": "scribe sync", "description": "Sync skills to your local machine"},
    {"command": "scribe list", "description": "Verify installed skills"}
  ]
}
```

The steps array adapts to state — if already connected, it might only contain `sync` and `list`. If auth is missing, the first step becomes `gh auth login`.

## Architecture

### UI Approach: Hybrid

- **Huh v2 forms** for decision points and text input (proven pattern in this codebase)
- **Bubble Tea** for the sync/install progress view (real-time feedback)
- **Lipgloss** for prereq checklist and final summary (styled static output)

These are sequential, not interleaved — huh collects input, then Bubble Tea runs the sync, then lipgloss shows the summary.

### Code Organization

- `cmd/guide.go` — Cobra command, orchestrates the flow
- `internal/ui/syncprogress.go` — Bubble Tea model for sync progress (reusable by `sync` command later)
- Reuses existing functions: `connectToRepo()` from connect.go, create registry logic, list/sync internals

The guide is an orchestration layer over existing logic. It calls the same internal functions that `connect`, `create registry`, `list`, and `sync` use — no reimplementation.

## Flow

### Phase 1: Prerequisite Checks

Runs automatically on start. Checks:

1. **GitHub auth** — tries the auth chain (`gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.toml` token). Reports which method succeeded or that none did.
2. **Scribe directory** — does `~/.scribe/` exist? (Not an error if missing — gets created on first save.)
3. **Existing connections** — are there repos in `config.toml`'s `team_repos`?

TTY display — lipgloss-styled checklist:

```
  Scribe Guide

  ✓ GitHub authenticated (gh cli)
  ✓ Scribe directory exists
  ○ No team registries connected

  Let's get you set up.
```

If auth fails, the guide doesn't bail. It prints what to do (`run gh auth login or set GITHUB_TOKEN`), then shows a `huh.Confirm` asking "Ready to re-check?" When confirmed, it re-runs the auth check. This loops until auth passes or the user exits (Ctrl+C). The guide is a guide, not a gatekeeper.

### Phase 2: Choose Your Path

Huh select prompt:

```
What would you like to do?

> Join an existing team
  Create a new skill registry
  View my current setup
```

The options adapt to state — "View my current setup" only appears if there are existing connections.

### Phase 3A: Join an Existing Team

1. `huh.Input` for `owner/repo` (same validation as `connect.go`)
2. Calls `connectToRepo()` — the same function `connect` uses
3. Hands off to Phase 4 (sync progress)

### Phase 3B: Create a New Skill Registry

1. Reuses `create registry` flow's logic — prompts for team name, owner, repo name, visibility
2. Creates the repo, pushes scaffold
3. Connects and syncs (enters Phase 4)

### Phase 3C: View My Current Setup

1. Shows connected registries
2. Shows installed skills (reuses `list` logic)
3. Shows last sync time
4. Offers: "Connect another registry?" or "Run a sync now?" — loops back into the guide

### Phase 4: Sync Progress (Bubble Tea)

A Bubble Tea model in `internal/ui/syncprogress.go` that consumes events the `Syncer` already emits:

```
  Syncing ArtistfyHQ/team-skills

  ✓ cleanup          v2.1.0  → claude, cursor
  ✓ code-review      v1.3.0  → claude
  ◐ investigate       downloading...
  ○ ship             pending
  ○ dark-mode        pending

  3/5 skills installed
```

Event mapping:
- `SkillResolvedMsg` → adds row as `○ pending`
- `SkillDownloadingMsg` → `◐ downloading...` with spinner
- `SkillInstalledMsg` → `✓` with version and targets
- `SkillSkippedMsg` → `– current` (dimmed)
- `SyncCompleteMsg` → ends the Bubble Tea program

Wiring: the `Syncer.Emit` callback (`func(any)`) sends events into the Bubble Tea program via `p.Send(msg)`. No changes to the sync engine.

**Reusability:** this model is not guide-specific. `scribe sync` can adopt it for TTY output by replacing its current `fmt.Printf` callback with `p.Send()`.

### Phase 5: Summary and Next Steps

After sync completes, lipgloss-styled summary:

```
  All set!

  Registry    ArtistfyHQ/team-skills
  Skills      5 installed, 0 skipped
  Targets     claude, cursor

  What's next:
  • scribe sync       Keep skills up to date
  • scribe list       See installed skills and status
  • scribe add        Add skills to your registry
  • scribe guide      Run this guide again anytime
```

The "What's next" section adapts based on path taken:
- **Joined a team:** emphasizes `sync` and `list`
- **Created a registry:** emphasizes `add` and editing `scribe.toml`
- **Viewed setup:** only shows commands relevant to what they haven't done

## Key Principles

- **Orchestration, not reimplementation** — guide calls existing functions
- **Adaptive** — detects current state, skips what's already done
- **Non-gatekeeper** — helps fix problems rather than refusing to proceed
- **Dual-mode** — interactive for humans, JSON for agents
- **Reusable UI** — sync progress model designed for use beyond the guide

## Out of Scope

- `scribe add` implementation (separate spec — the guide ends with a pointer to it)
- Full TUI overhaul of other commands (sync progress model is reusable but adoption is follow-up work)
- First-run auto-detection (the guide is always explicit via `scribe guide`, not triggered automatically)
