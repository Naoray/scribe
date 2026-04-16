# Scribe Agent Template + Daily Binary Upgrade Nudge

**Date:** 2026-04-16
**Status:** Proposed

## Problem

The embedded `scribe-agent` skill is currently shipped as one static `SKILL.md` file. That creates two issues:

1. The skill always carries a large first-run bootstrap block, even after `scribe` and `scribe-agent` are already installed. This wastes tokens on the steady-state path.
2. The skill cannot adapt its instructions based on local state, so it cannot nudge the agent to keep the `scribe` binary up to date.

We want the embedded skill to render from a template at install/refresh time. On first install it should include the bootstrap instructions. After bootstrap succeeds, it should omit that section and instead teach the agent to check for `scribe` binary updates at most once per day, ask the user for permission, and run `scribe upgrade` on approval.

## Goals

1. Replace the static embedded `scribe-agent` markdown payload with a templated render step.
2. Include the one-time bootstrap section only when `scribe` is missing or `scribe-agent` is not yet installed locally.
3. Add a daily-throttled `scribe` binary update check policy to the rendered skill.
4. When a newer `scribe` version is known, render instructions that tell the agent to ask for permission and run `scribe upgrade` itself on approval.
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

    CurrentVersion        string
    LastCheckedAt         time.Time
    ShouldCheckForUpdates bool
    LatestVersion         string
    UpdateAvailable       bool
}
```

Rendering rules:

- `NeedsBootstrap` is true when either the `scribe` binary is missing from `PATH` or the local canonical store/state does not yet contain `scribe-agent`.
- The bootstrap block is rendered only when `NeedsBootstrap` is true.
- The daily update-nudge block is rendered only when `NeedsBootstrap` is false.
- The "update available" wording appears only when `UpdateAvailable` is true.
- When no recent check result exists or the last check failed, the steady-state section should tell the agent to perform the daily version check before normal `scribe` operations when the 24h TTL has expired.

Important: the rendered markdown is guidance for the external agent. It is not executable policy inside `scribe`. The CLI only persists enough local state to let the markdown stay short and accurate.

### 3. Persist update-check cache in state

Persist the `scribe` binary update-check cache in `~/.scribe/state.json`, not in `config.yaml`.

Reasoning:

- This is operational cache, not user preference.
- It belongs with other runtime reconciliation state.
- It can be updated opportunistically during bootstrap refreshes without turning config saves into side-effect writes.

Add a top-level field to `state.State`:

```go
type BinaryUpdateCheck struct {
    CurrentVersion string    `json:"current_version,omitempty"`
    LatestVersion  string    `json:"latest_version,omitempty"`
    CheckedAt      time.Time `json:"checked_at,omitempty"`
}
```

```go
type State struct {
    ...
    BinaryUpdateChecks map[string]BinaryUpdateCheck `json:"binary_update_checks,omitempty"`
}
```

Use key `"scribe"` for this feature. A map keeps the schema extensible without forcing another migration if similar first-party binary nudges are added later.

Behavior:

- If no entry exists, the rendered skill behaves as "check allowed now".
- If `CheckedAt` is less than 24 hours old, `ShouldCheckForUpdates` is false.
- If `CheckedAt` is 24 hours old or older, `ShouldCheckForUpdates` is true.
- Failed checks do not block normal skill use. They simply do not update the cached entry.

### 4. How version data is obtained

Bootstrap/render code should compute template data using two sources:

1. Local binary version:
   - Use the running `cmd.Version` string when available in command paths that already know it.
   - Fall back to `"dev"` or empty as "do not suggest upgrade" for development builds.

2. Latest released version:
   - Reuse the existing GitHub release lookup path already exercised by `scribe upgrade`.
   - Compare via `internal/upgrade.NeedsUpgrade`.

Design constraint:

- The bootstrap refresh path must remain safe when GitHub is unavailable.
- If the release check fails, preserve the previous cached result and render the steady-state instructions without an "update available" nudge.

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

- At most once per 24 hours, check whether a newer `scribe` release exists.
- If a newer version exists, ask the user for permission to update.
- If the user approves, run `scribe upgrade`.
- If the check fails, continue with the user's actual request and do not guess.

When `UpdateAvailable` is already known from cached or freshly-computed data, the rendered text should mention the current and latest versions so the agent can ask a concrete permission question.

Example tone:

> `scribe` update available (`vX -> vY`). Before proceeding, ask whether to run `scribe upgrade`. If the user says yes, run it, then continue.

This replaces the current `refresh bootstrap skill` emphasis with the more important binary update path.

### 6. Bootstrap/install flow changes

Current behavior:

- `EnsureScribeAgent` installs `EmbeddedSkillMD` directly.

New behavior:

- `EnsureScribeAgent` gathers render inputs.
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

### 7. Upgrade-check execution seam

Do not add network I/O to every command path.

Instead:

- Commands that already call `EnsureScribeAgent` may opportunistically refresh the cached `"scribe"` binary update entry when the TTL is expired and GitHub is reachable.
- The refresh must be best-effort. On error, skip silently and keep going.

This preserves the current "bootstrap is scoped, not root-global" design while keeping the skill content fresh enough to guide the agent.

### 8. Testing

Add focused tests around rendering and caching.

#### `internal/agent/render_test.go`

- `TestRenderBootstrapWhenBinaryMissing`
- `TestRenderBootstrapWhenScribeAgentMissing`
- `TestRenderSteadyStateWhenInstalled`
- `TestRenderShowsUpdateAvailableMessage`
- `TestRenderOmitsUpdateMessageForFreshCurrentVersion`

#### `internal/agent/bootstrap_test.go`

- Replace byte-equality assertions against `EmbeddedSkillMD` with assertions about rendered content sections.
- Verify a steady-state install writes the non-bootstrap version.
- Verify stale TTL causes a best-effort check and state cache update when a newer version is returned.
- Verify failed release lookup leaves the cached value unchanged and still installs the skill.

#### `internal/state/state_test.go`

- Verify the new `BinaryUpdateChecks` field round-trips and preserves old state files.

## Open question resolved

The user chose:

- update target: the `scribe` binary itself
- frequency: once per day
- action on approval: run the upgrade from inside the skill flow via `scribe upgrade`

## Recommended implementation order

1. Add template file and render helper with pure unit tests.
2. Extend state schema with binary update cache.
3. Wire best-effort daily update refresh into `EnsureScribeAgent`.
4. Rewrite bootstrap tests around rendered sections instead of static bytes.
5. Update the embedded `scribe-agent` copy to use the new compact steady-state wording.
