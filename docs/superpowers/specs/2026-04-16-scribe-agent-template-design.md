# Scribe Agent Template + Daily Binary Upgrade Nudge

**Date:** 2026-04-16
**Status:** Proposed

## Problem

The embedded `scribe-agent` skill is currently shipped as one static `SKILL.md` file. That creates two issues:

1. The skill always carries a large first-run bootstrap block, even after `scribe` and `scribe-agent` are already installed. This wastes tokens on the steady-state path.
2. The skill cannot adapt its instructions based on local state, so it cannot nudge the agent to keep the `scribe` binary up to date.

We want the embedded skill to render from a template at install/refresh time. On first install it should include the bootstrap instructions. After bootstrap succeeds, it should omit that section and instead teach the agent to ask once per day for permission to run `scribe upgrade`, then run it on approval.

## Goals

1. Replace the static embedded `scribe-agent` markdown payload with a templated render step.
2. Include the one-time bootstrap section only when `scribe` is missing or `scribe-agent` is not yet installed locally.
3. Add a daily-throttled `scribe upgrade` prompt policy to the rendered skill.
4. Render instructions that tell the agent to ask for permission and run `scribe upgrade` itself on approval.
5. Keep `scribe upgrade` as the only binary self-update implementation.

## Non-goals

- No new interactive update prompt inside the `scribe` CLI itself.
- No automatic background upgrade without agent/user approval.
- No generalized user-authored template engine for third-party skills in this change.
- No change to how third-party registries render or install skills.

## Design

### 1. Store a template, render concrete markdown

Replace `internal/agent/scribe_agent/SKILL.md` with `internal/agent/scribe_agent/SKILL.md.tmpl`.

The embedded bootstrap code in `internal/agent` should render the template into concrete markdown before writing:

- `~/.scribe/skills/scribe-agent/SKILL.md`
- `~/.scribe/skills/scribe-agent/.scribe-base.md`

This keeps all downstream behavior unchanged. Tool projection, `scribe explain`, sync, and on-disk skill layout still see a normal `SKILL.md`.

The renderer should use Go's standard `text/template` package. This is enough for simple conditional sections and avoids introducing a second templating language or external runtime.

### 2. Render context

Add a typed render context owned by `internal/agent`, for example:

```go
type SkillTemplateData struct {
    NeedsBootstrap        bool
    HasScribeBinary       bool
    HasScribeAgentInstalled bool

    LastSucceededAt       time.Time
    ShouldCheckForUpdates bool
}
```

Rendering rules:

- `NeedsBootstrap` is true when either the `scribe` binary is missing from `PATH` or the local canonical store/state does not yet contain `scribe-agent`.
- The bootstrap block is rendered only when `NeedsBootstrap` is true.
- The daily update-nudge block is rendered only when `NeedsBootstrap` is false.
- The steady-state section tells the agent to ask once per 24 hours for permission to run `scribe upgrade`.
- A successful `scribe upgrade` run, including a no-op "Already up to date" outcome, resets the 24h TTL.
- A failed `scribe upgrade` run does not reset the TTL.

Important: the rendered markdown is guidance for the external agent. It is not executable policy inside `scribe`. The CLI only persists enough local state to let the markdown stay short and accurate.

### 3. Persist upgrade cooldown in state

Persist the `scribe` binary upgrade cooldown in `~/.scribe/state.json`, not in `config.yaml`.

Reasoning:

- This is operational cache, not user preference.
- It belongs with other runtime reconciliation state.
- It can be updated opportunistically during bootstrap refreshes without turning config saves into side-effect writes.

Add a top-level field to `state.State`:

```go
type BinaryUpdateCheck struct {
    LastSucceededAt time.Time `json:"last_succeeded_at,omitempty"`
}
```

```go
type State struct {
    ...
    BinaryUpdateChecks map[string]BinaryUpdateCheck `json:"binary_update_checks,omitempty"`
}
```

Use key `"scribe"` for this feature. A map keeps the schema extensible without forcing another migration if similar first-party tool nudges are added later.

Behavior:

- If no entry exists, the rendered skill behaves as "ask allowed now".
- If `LastSucceededAt` is less than 24 hours old, `ShouldCheckForUpdates` is false.
- If `LastSucceededAt` is 24 hours old or older, `ShouldCheckForUpdates` is true.
- Failed `scribe upgrade` runs do not update the cached entry.

### 4. How the daily prompt works

Bootstrap/render code should not fetch GitHub release data.

Instead:

1. Determine whether the 24-hour cooldown has expired by reading `state.BinaryUpdateChecks["scribe"]`.
2. If expired, render instructions telling the external agent to ask for permission to run `scribe upgrade`.
3. The agent then runs `scribe upgrade`, and that command remains the single source of truth for "is there actually an update?"

Design constraints:

- The bootstrap/render path remains local-only.
- The skill does not duplicate release-resolution logic in markdown.
- Future human-facing TUI paths may still warm cache or show separate update notices, but that is outside the bootstrap renderer's responsibility.

### 5. Template content changes

Split the current long "First-run bootstrap" section into two conditional sections.

#### Bootstrap section

Rendered only when `NeedsBootstrap` is true.

It keeps the current install-and-register instructions, but should be tightened:

- Check `scribe --version`.
- If missing, install `scribe`.
- Then confirm `scribe-agent` is present locally via `scribe list --json`.
- If missing, install/refresh the skill.
- Once bootstrap succeeds, continue with the user's actual request.

This section remains explicit because it exists to rescue first-run sessions where the agent may not yet know how to use `scribe`.

#### Steady-state section

Rendered only when `NeedsBootstrap` is false.

Replace the bootstrap block with a compact rule set:

- At most once per 24 hours, ask the user for permission to run `scribe upgrade`.
- If the user approves, run `scribe upgrade`.
- If `scribe upgrade` succeeds, even if it reports "Already up to date", treat the daily prompt as satisfied.
- If `scribe upgrade` fails, continue with the user's actual request and allow asking again next invocation.

Example tone:

> If you have not run `scribe upgrade` successfully in the last 24 hours, ask whether to run it before proceeding. If the user says yes, run it, then continue.

This replaces the current `refresh bootstrap skill` emphasis with the more important binary update path.

### 6. Bootstrap/install flow changes

Current behavior:

- `EnsureScribeAgent` installs `EmbeddedSkillMD` directly.

New behavior:

- `EnsureScribeAgent` gathers local render inputs.
- It renders `SKILL.md.tmpl` into concrete markdown.
- It writes the rendered bytes to the canonical store and `.scribe-base.md`.

Suggested package shape:

- `internal/agent/embed.go`
  - embed `SKILL.md.tmpl`
- `internal/agent/render.go`
  - parse template
  - build `SkillTemplateData`
  - render bytes
- `internal/agent/bootstrap.go`
  - call render, then install

Versioning rule:

- `EmbeddedVersion` should hash the template bytes plus a renderer format version string, not just the raw template file.
- This ensures template or renderer changes can trigger a refresh even if the rendered output happens to match on one machine.

### 7. Upgrade execution seam

Do not add release-checking network I/O to `EnsureScribeAgent`.

Instead:

- `EnsureScribeAgent` consumes only local state to decide whether the daily `scribe upgrade` prompt should be rendered.
- The external agent, following the rendered skill, runs `scribe upgrade` when the user approves.
- After a successful `scribe upgrade` invocation, `scribe` should update `state.BinaryUpdateChecks["scribe"].LastSucceededAt`.

This preserves the current "bootstrap is scoped, not root-global" design and keeps upgrade resolution inside the existing `scribe upgrade` command.

### 8. Testing

Add focused tests around rendering and caching.

#### `internal/agent/render_test.go`

- `TestRenderBootstrapWhenBinaryMissing`
- `TestRenderBootstrapWhenScribeAgentMissing`
- `TestRenderSteadyStateWhenInstalled`
- `TestRenderShowsDailyUpgradePromptWhenCooldownExpired`
- `TestRenderOmitsDailyUpgradePromptWhenCooldownFresh`

#### `internal/agent/bootstrap_test.go`

- Replace byte-equality assertions against `EmbeddedSkillMD` with assertions about rendered content sections.
- Verify a steady-state install writes the non-bootstrap version.
- Verify expired cooldown writes the daily-upgrade wording.
- Verify fresh cooldown writes the non-prompt steady-state wording.

#### `internal/state/state_test.go`

- Verify the new `BinaryUpdateChecks` field round-trips and preserves old state files.

## Open question resolved

The user chose:

- update target: the `scribe` binary itself
- frequency: once per day, on the first skill invocation after 24 hours
- action on approval: run the upgrade from inside the skill flow via `scribe upgrade`
- success semantics: a successful no-op `scribe upgrade` still resets the cooldown
- failure semantics: a failed `scribe upgrade` does not reset the cooldown
- bootstrap semantics: the bootstrap section remains until both the `scribe` binary exists and `scribe-agent` is installed locally

## Recommended implementation order

1. Add template file and render helper with pure unit tests.
2. Extend state schema with the successful-upgrade cooldown cache.
3. Wire cooldown-based rendering into `EnsureScribeAgent`.
4. Rewrite bootstrap tests around rendered sections instead of static bytes.
5. Update `scribe upgrade` to record a successful run in state.
6. Update the embedded `scribe-agent` copy to use the new compact steady-state wording.
