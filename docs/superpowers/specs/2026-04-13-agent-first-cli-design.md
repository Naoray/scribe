# Making Scribe agent-first: a complete design guide

**AI coding agents — not humans — should be Scribe's primary audience.** This means every command must produce structured, parseable output; every failure must communicate what went wrong, whether it's retryable, and what to do next; and every operation must be safe to run twice. The shift from human-first to agent-first isn't cosmetic — it requires rethinking output formats, exit codes, error contracts, discoverability, and the fundamental command vocabulary. The good news: proven patterns exist across `gh`, `kubectl`, `terraform`, `ripgrep`, and package managers like `npm` and `brew`. The emerging Model Context Protocol (MCP) and tool-use specifications from Anthropic and OpenAI provide concrete schemas. Scribe's position as a skill manager — sitting between registries, local storage, and tool-specific directories — makes it a natural fit for the plan/apply, declarative-state patterns that agents handle best.

---

## 1. Executive summary: ten decisions that define agent-first Scribe

These ten architectural decisions will determine whether AI agents can reliably orchestrate Scribe or whether they'll fumble through stderr parsing and ambiguous exit codes.

**Decision 1: Structured JSON output on every command.** Not "has a `--json` flag" but truly designed for programmatic consumption — JSON to stdout, human decoration to stderr, consistent types (ISO 8601 timestamps, integers not strings), and field selection via `--json name,version,status`. Follow `gh`'s model where `--json` with no arguments lists available fields.

**Decision 2: Semantic exit codes beyond 0/1.** Adopt a taxonomy: **0** success, **1** general error, **2** usage/input error, **3** not found, **4** permission denied, **5** conflict/already exists, **6** network error (retryable), **7** dependency error, **8** validation error, **10** partial success. An agent that gets exit code 6 retries with backoff. An agent that gets exit code 1 for everything must parse stderr — and will sometimes get it wrong.

**Decision 3: Idempotent by default.** `scribe install skill-x` when skill-x is already installed at the right version should exit 0 with `{"status":"already_installed"}`. Use declarative verbs (`ensure`, `sync`, `apply`) over imperative ones (`create`, `delete`). Follow `kubectl apply`'s desired-state convergence.

**Decision 4: Plan/apply separation for mutating operations.** Steal terraform's two-phase model: `scribe plan install skill-x --json` shows exactly what would change; `scribe apply` executes. This gives agents a safe preview before committing.

**Decision 5: Errors as structured data.** Every error must include `code` (machine-readable), `message` (human-readable), `resource` (what failed), `retryable` (boolean), and `remediation` (what to do next). An agent seeing `{"code":"REGISTRY_UNAVAILABLE","retryable":true,"retry_after_seconds":30}` can act immediately.

**Decision 6: TTY-aware automatic mode switching.** If stdout is a terminal, show colors, progress bars, and interactive prompts. If stdout is a pipe (how agents call CLIs), output clean JSON, suppress prompts, and fail with clear errors listing missing required flags. Respect `CI=true` and `NO_COLOR=1`.

**Decision 7: Self-describing commands.** Implement `scribe schema <command>` that returns JSON Schema for inputs and outputs. Ship a `CLAUDE.md` file documenting all commands for agent discovery. Consider an MCP server mode (`scribe mcp serve`) for structured protocol consumers.

**Decision 8: Lockfile and state management.** Maintain `scribe.lock` with exact versions and SHA checksums. Support `scribe ci` that fails if lockfile drifts from manifest. Track installation state in `scribe.state.json` with per-skill phases (staged, installed, linked).

**Decision 9: Graduated intervention model.** Default to prompting on conflicts; `--yes` auto-accepts defaults; `--force` overrides safety checks; `--dry-run` previews changes. TTY detection auto-selects the right mode. Read operations need zero intervention; write operations need approval; destructive operations need explicit confirmation.

**Decision 10: Dual-surface architecture.** One binary serving CLI (human/agent via bash) and MCP (structured protocol via stdio). Use the official Go MCP SDK. The CLI path is more token-efficient for simple operations; MCP wins for dynamic discovery across many tools.

---

## 2. Pattern library: proven designs for agent-friendly CLIs

### The structured output contract

The gap between "has `--json`" and "designed for programmatic consumption" is enormous. Tools truly designed for agents follow five rules:

**JSON to stdout, everything else to stderr.** Progress bars, warnings, spinners, and human-friendly messages go to stderr. The stdout stream is a clean, parseable data channel. This is how `gh`, `kubectl`, and `ripgrep` work in their structured modes. An agent piping `scribe list --json` to `jq` should never encounter a "Fetching..." message corrupting the JSON stream.

**Consistent types across all commands.** A timestamp is always ISO 8601, never "3 days ago." A count is always an integer, never "~100." A boolean is always `true`/`false`, never "yes"/"no." When types are consistent, agents don't need per-command parsing logic.

**Field selection controls output size.** Context window tokens are currency. `scribe list --json name,version` returns only requested fields. `scribe list --json` with no arguments prints available field names (the `gh` pattern). Consider a `--jq` flag for inline filtering without requiring external `jq`.

**JSON Lines (NDJSON) for streaming.** `ripgrep`'s `--json` mode emits one JSON object per line with type tags: `{"type":"match","data":{...}}`. For `scribe search`, streaming results as NDJSON means agents can process results incrementally rather than waiting for a full buffered response. Each line is independently parseable.

**Versioned schema with stability guarantees.** Include a `format_version` field in JSON output. Never remove or rename fields without a major version bump. Terraform includes format versions in all JSON output for exactly this reason. Breaking a JSON schema silently breaks every agent workflow that depends on it.

### Exit codes as control flow

The `grep` three-state model is the minimum: **0** = success with results, **1** = success but nothing found, **2+** = actual error. This pattern lets agents distinguish "the search worked but found nothing" from "the search failed" — a distinction lost when everything non-zero means error.

For a package manager, richer semantics pay for themselves immediately. `curl`'s ~100 distinct exit codes enable targeted retry logic without stderr parsing. A practical taxonomy for Scribe:

| Code | Meaning | Agent behavior |
|------|---------|---------------|
| 0 | Success | Continue |
| 1 | General failure | Log and escalate |
| 2 | Invalid input/usage | Fix arguments, don't retry |
| 3 | Resource not found | Try different name or registry |
| 4 | Permission denied | Check auth, don't retry |
| 5 | Conflict (already exists) | Skip or use `--force` |
| 6 | Network error | Retry with backoff |
| 7 | Dependency error | Resolve dependency first |
| 8 | Validation error | Fix manifest/schema |
| 10 | Partial success | Check which operations failed |

### Idempotency through declarative verbs

`terraform apply` is idempotent because it compares desired state against current state and only changes what differs. `ansible` reports `ok` (already correct) vs `changed` (modification made) vs `failed`. `brew install` exits 0 when a package is already installed with a "already installed" warning.

The pattern: **use declarative verbs that describe the desired end state, not the action to take.** `ensure` over `create`. `sync` over `install + update + remove`. `apply` over individual mutations. When an agent runs `scribe ensure skill-x` twice, the second run should be a fast no-op returning `{"status":"no_changes","skill":"skill-x"}`.

For operations that can't be fully idempotent (like `scribe add` to a registry), use distinct exit codes: exit 5 for "already exists" lets agents proceed without parsing error messages.

### The preview-then-commit pattern

Terraform's `plan`/`apply` separation is the gold standard. The plan phase is read-only and safe. The apply phase executes exactly what was planned. For agents, this means:

```
scribe plan install skill-x --json    →  shows files to add, symlinks to create
scribe plan sync --json               →  shows full diff (adds, updates, removes)
scribe apply                          →  executes the last plan
scribe apply --auto-approve           →  executes without confirmation
```

`kubectl` extends this with `--dry-run=client` (local validation) and `--dry-run=server` (server-side validation). For Scribe, `--dry-run` should resolve registries and check dependencies without actually installing — the agent needs to know if the operation *would* succeed.

### Structured errors with remediation

Cloud CLIs set the standard. AWS CLI v2.34+ supports `--cli-error-format json` returning `Code`, `Message`, and service-specific fields. Google APIs return `ErrorInfo` with `reason`, `domain`, and `metadata`. The critical field most CLIs miss: **remediation** — a concrete next step the agent can take.

```json
{
  "error": {
    "code": "SKILL_CONFLICT",
    "message": "Skill 'code-review' exists in both 'github.com/org/skills' and 'github.com/team/skills'",
    "retryable": false,
    "resources": ["github.com/org/skills/code-review", "github.com/team/skills/code-review"],
    "remediation": "Specify registry: scribe install code-review --registry github.com/org/skills",
    "exit_code": 5
  }
}
```

The `retryable` boolean is the single most important field for agents. It instantly partitions the error space into "try again" and "escalate."

---

## 3. Real-world examples and their lessons for Scribe

### gh (GitHub CLI) — field selection and built-in filtering

`gh` is the closest analog to what Scribe should become. Its `--json` flag accepts a comma-separated list of fields: `gh pr list --json number,title,author`. Running `--json` with no arguments prints all available fields — a discovery mechanism agents use constantly. The `--jq` flag embeds jq filtering without requiring external dependencies: `gh pr list --json author --jq '.[].author.login'`. The `gh api` command provides raw API access with built-in auth and pagination, covering operations that don't have dedicated subcommands.

**Exit code 4 means "not found"** — not just generic failure. This lets agents distinguish between "that PR doesn't exist" and "GitHub is down."

**Lesson for Scribe:** Implement `--json` with field selection on every command. Bundle `--jq` for in-tool filtering. Provide `scribe api` for raw registry operations. Use semantic exit codes matching specific failure modes.

### ripgrep — type-tagged JSON Lines for streaming

ripgrep's `--json` mode produces NDJSON where each line has a `type` field: `begin` (file start), `match` (result), `context` (surrounding lines), `end` (file end), `summary` (statistics). This lets consumers route messages by type — matches go to results, summaries go to metrics. VS Code's search integration consumes this format directly.

**The binary safety pattern is subtle but important:** ripgrep uses `{"text":"..."}` for valid UTF-8 and `{"bytes":"base64..."}` for binary content. Skill descriptions may contain Unicode, making this pattern relevant.

**Lesson for Scribe:** Use type-tagged NDJSON for `scribe search` and `scribe sync` output. Include timing/statistics in a `summary` message. Adopt the three-way exit code model: 0 = found, 1 = not found, 2 = error.

### terraform — state management and plan/apply

Terraform maintains a JSON state file mapping configuration to actual resources. On each `apply`, it diffs desired vs. current state and only changes what differs. The state file includes a `format_version` for forward compatibility. `terraform show -json` produces machine-readable plan output with `actions` arrays (`["create"]`, `["update"]`, `["delete"]`) per resource. When `apply` fails mid-way, it writes `errored.tfstate` for recovery.

**Terraform has no native rollback** — by design. Recovery means reverting the config in git and re-applying. The config (code) is the source of truth, not the state file.

**Lesson for Scribe:** Maintain `scribe.state.json` tracking installed skills with version, source, and phase. Implement plan/apply. Include `format_version` in state files. Make config manifests (`scribe.yaml`) the source of truth, with state tracking the current reality.

### kubectl — declarative convergence and dry-run levels

`kubectl apply -f deployment.yaml` converges toward desired state idempotently. `kubectl create` fails if the resource exists — a crucial distinction. The `--dry-run=server` flag sends the request to the API server for full validation (including webhooks and admission controllers) without persisting. `kubectl diff` shows what would change. Multiple output formats (`-o json`, `-o yaml`, `-o jsonpath`, `-o name`) cover every consumption pattern. **Fully qualified resource references** (`jobs.v1.batch/myjob`) prevent version ambiguity.

**Lesson for Scribe:** Support declarative management via `scribe apply -f skills.yaml`. Implement `scribe diff` showing what would change. Offer `--dry-run=client` (local manifest validation) and `--dry-run=server` (resolve against registries).

### npm/pnpm — lockfiles, CI mode, and conflict resolution

`npm ci` installs from lockfile only, fails if lockfile and `package.json` are out of sync, and deletes `node_modules` first — the definitive reproducible install. `pnpm`'s content-addressable store deduplicates packages globally. npm v7+ uses staging directories for atomic moves: packages unpack to temporary locations, then move atomically into `node_modules`. Conflict resolution uses `overrides` (npm) and `resolutions` (yarn) as escape hatches for impossible version conflicts. `npm why <package>` explains dependency chains.

**Lesson for Scribe:** Implement `scribe.lock` with exact versions and SHA checksums. Add `scribe ci` for frozen-lockfile installs. Use a staging directory pattern (`.scribe/staging/`) for atomic installs. Support `scribe why <skill>` to explain why a skill is installed and where it came from.

### brew — tap-based registry management

Homebrew's "tap" system maps directly to Scribe's registry model. A tap is a git repo added via `brew tap user/repo`. Ambiguous names resolve through priority: pinned taps → core → other taps. Fully qualified names (`user/repo/formula`) bypass resolution. Taps include `formula_renames.json` and `tap_migrations.json` for handling renamed or moved packages.

**Lesson for Scribe:** Model registries as taps with priority ordering. Support fully qualified skill names (`@registry/skill-name`) for disambiguation. Include rename/migration manifests in registries for handling skill moves.

---

## 4. Design recommendations specific to Scribe

### Command vocabulary redesign

Shift from imperative to declarative command names. The current human-oriented vocabulary should map to agent-friendly equivalents:

| Current (human-first) | Recommended (agent-first) | Rationale |
|----------------------|--------------------------|-----------|
| `scribe add <skill>` | `scribe install <skill>` | Standard package manager verb |
| `scribe remove <skill>` | `scribe uninstall <skill>` | Paired with `install` |
| — | `scribe ensure <skill>` | Idempotent: install if missing, no-op if present |
| — | `scribe sync` | Declarative: converge to manifest state |
| — | `scribe plan <operation>` | Preview any mutation |
| — | `scribe apply` | Execute a saved plan |
| `scribe list` | `scribe list` (unchanged) | Already agent-friendly |
| — | `scribe status` | Drift detection, broken symlinks, pending updates |
| — | `scribe doctor` | Health check across registries, symlinks, state |
| — | `scribe diff` | Show what would change vs. manifest |
| — | `scribe why <skill>` | Explain provenance and dependencies |
| — | `scribe schema <command>` | JSON Schema for command input/output |
| — | `scribe log --json` | Structured operation history |

### Output format architecture

Implement a Go `OutputWriter` interface that switches between human and machine modes:

```go
type OutputWriter interface {
    WriteResult(v interface{}) error
    WriteError(err *StructuredError) error
    WriteProgress(msg string) error   // stderr only
}
```

Three implementations: `JSONOutputWriter` (JSON to stdout, progress to stderr), `TableOutputWriter` (colored tables, spinners, prompts), and `QuietOutputWriter` (minimal output, bare values for piping). Selection logic: explicit `--json` flag → forced JSON; explicit `--quiet` → bare values; otherwise, TTY detection chooses automatically.

**Standard JSON envelope for all responses:**

```json
{
  "status": "ok",
  "format_version": "1",
  "data": { ... },
  "meta": {
    "duration_ms": 234,
    "command": "install",
    "scribe_version": "0.12.0"
  }
}
```

### Registry and disambiguation strategy

When multiple registries contain a skill with the same name, Scribe must never silently pick one. The resolution strategy:

1. Check if a default registry is configured for this skill name (via `scribe.yaml` overrides)
2. Check registry priority order (user-configured, like Homebrew's pinned taps)
3. If still ambiguous, exit with code 5 and structured error listing all options with their registries
4. Support `@registry/skill-name` syntax for explicit disambiguation
5. In `--yes` mode, use the highest-priority registry; document this behavior

### MCP server integration

Using the official Go MCP SDK, Scribe should serve as an MCP server via `scribe mcp serve`, exposing tools like:

```json
{
  "tools": [
    {
      "name": "scribe_install",
      "description": "Install a coding skill from a registry",
      "inputSchema": {"type":"object","properties":{"skill":{"type":"string"},"registry":{"type":"string"}},"required":["skill"]},
      "annotations": {"destructiveHint": false, "idempotentHint": true}
    },
    {
      "name": "scribe_list",
      "description": "List installed coding skills with version and source",
      "inputSchema": {"type":"object","properties":{"fields":{"type":"array","items":{"type":"string"}}}},
      "annotations": {"readOnlyHint": true}
    }
  ]
}
```

MCP tool annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`) communicate safety characteristics to agents, enabling automated approval policies. A 2026 study found CLI invocations are **10–32× cheaper in tokens** than MCP for simple operations, so both surfaces should coexist — CLI for cost-efficient bash-based agents, MCP for protocol-native environments.

---

## 5. The 90/10 framework: autonomy tiers for every operation

### Level 5 operations — fully autonomous, zero intervention

These operations are read-only or convergent with no destructive potential. An agent should execute them without asking:

- **`scribe list`** — Pure read, returns installed skills
- **`scribe search <query>`** — Registry query, no side effects
- **`scribe info <skill>`** — Metadata display
- **`scribe status`** — Drift detection, broken symlink check
- **`scribe doctor`** — Health diagnostics
- **`scribe why <skill>`** — Provenance explanation
- **`scribe schema <command>`** — Self-description
- **`scribe diff`** — Shows pending changes without applying
- **`scribe plan <operation> --json`** — Previews mutation safely

**Design constraint:** These commands must never prompt, never write to the filesystem (except logs), and never require `--yes`. If run in non-TTY mode, they produce clean JSON with no decorations.

### Level 4 operations — agent drives, approval recommended

These operations mutate filesystem state but are recoverable. An agent running with `--yes` can execute autonomously; without it, confirmation is appropriate:

- **`scribe install <skill>`** — Writes files and creates symlinks. Idempotent: exit 0 if already installed
- **`scribe update <skill>`** — Replaces skill version. Safe if semver minor/patch
- **`scribe sync`** — Converges to manifest state (may install, update, and remove)
- **`scribe apply`** — Executes a saved plan

**Decision criteria for autonomous execution:** The agent should auto-approve if (a) the plan shows no removals, (b) no major version bumps are involved, and (c) no conflicts require disambiguation. The `--dry-run --json` output should include a `safe_to_auto_approve` boolean based on these criteria.

### Level 3 operations — require judgment

These operations involve ambiguity or irreversible consequences that warrant deliberation:

- **Conflict resolution** — Local modifications to a skill diverge from upstream. Agent should present both versions and ask the orchestrating agent or user to choose: `{"conflict":"local_modified","local_version":"1.2.0-modified","upstream_version":"1.3.0","options":["keep_local","accept_upstream","merge"]}`
- **Ambiguous intent** — Same skill name in multiple registries. Exit with code 5 and list options
- **Breaking changes** — Major version bump detected. Include changelog excerpt in output
- **First-time setup** — Registry authentication, tool directory configuration

### Level 1–2 operations — human confirmation required

- **`scribe uninstall <skill>`** — Destructive, removes files. Always confirm unless `--force`
- **`scribe reset`** — Wipes all state. Require `--force --yes` for non-interactive execution
- **`scribe registry remove`** — May orphan installed skills. Show impact analysis first
- **Authentication setup** — API tokens, SSH keys for private registries

### Implementing the graduated flag model

```
scribe sync                          # Interactive: prompts on conflicts
scribe sync --yes                    # Auto-accept: uses defaults for all prompts
scribe sync --yes --json             # Agent mode: auto-accept + structured output
scribe sync --force                  # Override: ignores conflicts, forces upstream
scribe sync --dry-run                # Preview: shows what would change
scribe sync --dry-run --json         # Agent preview: structured change plan
```

The `--yes` flag answers "are you sure?" questions. The `--force` flag overrides safety checks. These are distinct — `--yes` with a conflict should exit with an error explaining the conflict, while `--force` should resolve conflicts using a documented default strategy.

---

## 6. Error recovery architecture

### Three-phase install with checkpoint tracking

Every skill installation proceeds through three phases, each recorded in `scribe.state.json`:

1. **Stage** — Download/clone skill to `.scribe/staging/<skill>/`. State: `staging`
2. **Install** — Move from staging to skills directory. State: `installed`
3. **Link** — Create symlinks into tool directories (`.claude/`, `.cursor/`, etc.). State: `linked`

If Scribe crashes or is interrupted, re-running the same install command checks state and resumes from the last incomplete phase. Staged-but-not-installed skills just need moving. Installed-but-not-linked skills just need symlinking. This is the `rsync` pattern applied to package management.

### Batch semantics: best-effort with structured reporting

For `scribe sync` processing multiple skills, use best-effort semantics — each skill installs independently. Failures in one skill don't block others. The final output reports per-skill status:

```json
{
  "status": "partial_success",
  "results": [
    {"skill": "code-review", "action": "installed", "version": "1.2.0", "status": "ok"},
    {"skill": "test-gen", "action": "install", "version": "2.0.0", "status": "failed", "error": {"code": "NETWORK_ERROR", "retryable": true}},
    {"skill": "pr-summary", "action": "no_change", "version": "1.0.0", "status": "ok"}
  ],
  "summary": {"succeeded": 2, "failed": 1, "unchanged": 1}
}
```

Exit code **10** (partial success) signals that the agent should inspect results and retry failed operations.

### Retry communication

Every error in structured output includes explicit retryability:

```json
{
  "error": {
    "code": "REGISTRY_TIMEOUT",
    "retryable": true,
    "retry_after_seconds": 30,
    "max_retries_suggested": 3,
    "message": "Registry 'github.com/org/skills' timed out after 10s"
  }
}
```

For network operations, Scribe should support `--max-retries N` with exponential backoff built in. Transient errors (network timeout, HTTP 429/500/502/503) are retryable. Permanent errors (invalid manifest, permission denied, skill not found) are not. The exit code taxonomy encodes this: **exit 6** (network) is always retryable; **exit 2** (usage error) never is.

### Rollback via paired operations

Every install records enough state to fully reverse. `scribe uninstall <skill>` removes files from the skills directory, deletes symlinks from all tool directories, and updates `scribe.state.json`. For batch rollback after a failed sync, `scribe rollback` reverts to the previous state snapshot:

```
scribe rollback                # Revert to pre-last-sync state
scribe rollback --to <timestamp>  # Revert to specific checkpoint
scribe log --json              # View operation history for rollback targets
```

State snapshots are maintained using a simple versioning scheme: before each sync, copy `scribe.state.json` to `.scribe/snapshots/<timestamp>.state.json`. Rotation policy keeps the last 10 snapshots.

### State introspection commands

Agents need to understand current state before making decisions:

- **`scribe status --json`** — Returns drift analysis: local vs. registry versions, broken symlinks, orphaned skills, pending updates
- **`scribe doctor --json`** — Returns health check results: registry reachability, filesystem permissions, state consistency, tool directory validity
- **`scribe log --json --since 1h`** — Returns recent operations for context reconstruction

The `doctor` command follows `brew doctor`'s model: checks everything, returns structured results with severity levels and actionable remediation for each issue found.

---

## 7. Anti-patterns: what not to do when designing for agents

**Never use a pager in non-TTY mode.** AWS CLI v2 changed its default pager to `less`, breaking thousands of CI jobs when the pager waited for keyboard input in headless environments. Always check TTY before enabling pagination. Scribe should never invoke a pager when stdout is piped.

**Never mix data and decoration on stdout.** "Fetching skills... done ✓" followed by JSON on the same stream is unparseable. Progress messages go to stderr. Always. The stdout stream is sacred data territory.

**Never return inconsistent types.** If `scribe list --json` returns `"version": "1.2.0"` (string) on one skill and `"version": 1.2` (number) on another, agents will crash. Define types once, enforce them everywhere.

**Never change JSON field names without a version bump.** Renaming `skill_name` to `name` in a patch release silently breaks every agent workflow. Treat `--json` output as a versioned API contract with backward compatibility guarantees.

**Never require interactive input in programmatic mode.** If a required flag is missing in non-TTY mode, fail immediately with a structured error listing exactly which flags are required. Never block waiting for stdin that will never arrive.

**Never return exit code 0 on failure.** Some tools return 0 but include error information in stdout/stderr. Agents check exit codes first — a false 0 means the agent proceeds with corrupted state. If anything went wrong, the exit code must be non-zero.

**Never embed state in environment that isn't documented.** If Scribe reads `SCRIBE_REGISTRY_URL`, `SCRIBE_SKILLS_DIR`, or `SCRIBE_AUTH_TOKEN`, these must be documented in `--help` output and `schema` output. Ambient state that agents can't discover causes mysterious failures.

**Never use human-readable time formats in JSON.** "3 days ago" or "last Tuesday" require NLP to parse. Always use ISO 8601 (`2026-04-13T10:30:00Z`) or Unix timestamps.

**Never silently resolve ambiguity.** If "code-review" exists in two registries and Scribe picks one without telling the agent, the wrong skill gets installed. Ambiguity must produce an error with explicit options.

**Never design commands that only work in sequence without documenting the sequence.** If `scribe init` must precede `scribe install`, the error from running `install` before `init` must say "Run 'scribe init' first," not "Error: state file not found."

---

## 8. Implementation priorities: quick wins versus strategic investments

### Week 1–2: Foundation (highest ROI, lowest effort)

**Semantic exit codes.** Define the exit code taxonomy (0–10) and implement it consistently. This is a code-level change with no UX impact on humans but massive impact on agent reliability. Every `os.Exit(1)` becomes a specific code.

**Structured error responses.** Wrap all error returns in a `StructuredError` struct with `code`, `message`, `retryable`, and `remediation` fields. When `--json` is active, errors serialize to JSON on stderr. Without `--json`, they render as human-readable messages. This is the single highest-leverage change.

**`--json` flag on list and status commands.** Start with read-only commands: `scribe list --json`, `scribe search --json`, `scribe info --json`. These are the commands agents call most frequently. Define the output schema and commit to it.

**TTY detection.** Use `isatty` to auto-detect pipe vs. terminal. In pipe mode: no colors, no prompts, no progress bars. If stdin is not a terminal and a required value is missing, fail with a clear error listing missing flags.

### Week 3–4: Core agent experience

**`--yes` and `--dry-run` flags.** Add to all mutating commands. `--yes` skips confirmation prompts. `--dry-run` shows what would change without acting. `--dry-run --json` is the agent's primary decision-making input.

**Idempotent install.** `scribe install <skill>` with an already-installed skill should exit 0 with `{"status":"already_installed"}` instead of failing. Add `--if-not-exists` as a fallback for backward compatibility.

**State file initialization.** Create `scribe.state.json` tracking installed skills, their versions, source registries, and installation timestamps. This becomes the foundation for `status`, `diff`, and `rollback`.

**`CLAUDE.md` skill file.** Ship a markdown file documenting all commands, flags, exit codes, and examples. Claude Code reads this file to learn how to use Scribe. Include realistic examples — agents learn from examples faster than descriptions.

### Month 2: Advanced patterns

**Plan/apply workflow.** Implement `scribe plan <operation> --json` producing a structured change plan, and `scribe apply` executing the last plan. This is the terraform pattern.

**`scribe doctor` command.** Check registry reachability, symlink validity, state consistency, tool directory existence. Output structured results with severity and remediation. Agents call this after failures to diagnose issues.

**`scribe schema <command>` introspection.** Return JSON Schema for every command's input (flags/args) and output format. This enables dynamic tool discovery by agents without reading documentation.

**Structured operation log.** Write JSON Lines to `~/.scribe/operations.log` recording every mutation: timestamp, command, skill, status, duration, actor (cli/mcp/ci). Support `scribe log --json --since 24h`.

### Month 3+: Strategic capabilities

**MCP server mode.** `scribe mcp serve` runs as a stdio-based MCP server using the official Go SDK. Same binary, dual surface. Expose all commands as MCP tools with proper annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`).

**Lockfile management.** Implement `scribe.lock` with exact versions and integrity checksums. Add `scribe ci` for frozen-lockfile installs. Support `scribe why <skill>` for provenance tracking.

**Rollback and snapshots.** State snapshots before mutations. `scribe rollback` to revert. Keep last 10 snapshots with rotation.

**Session and rate limit management.** Include `_meta` in JSON responses with rate limit status. Implement `--max-retries` with exponential backoff for network operations.

---

## 9. Measurement plan: tracking whether agents succeed

### Primary metrics

**Agent task completion rate** — the percentage of agent-initiated Scribe operations that complete without human intervention. Measure by tracking invocations where `actor=agent` (detectable via non-TTY, `CI=true`, or `AGENT=true` environment variable) and correlating with error rates. Target: **>90%** of agent-initiated operations succeed on first attempt.

**Human intervention rate** — how often a human must step in to fix a Scribe-related issue during agent workflows. Track by counting operations that exit with intervention-required codes (exit 5 for conflicts, errors requiring human judgment). Target: **<10%** of all operations. Measure weekly, trend over time.

**Parse error rate** — how often agents fail to parse Scribe's output. This should be **0%** with proper `--json` implementation. Detect by monitoring agent retry patterns: if an agent calls `scribe list --json` then immediately calls it again with different flags, the first call likely produced unparseable output.

**Retry rate per command** — the average number of retries before success. High retry rates indicate unclear errors or missing idempotency. Track retries by comparing sequential identical commands in the operation log. Target: **<1.2 average retries** per successful operation (meaning most succeed first try).

### Implementation approach

**Structured telemetry in the operation log.** Every invocation records:

```json
{
  "timestamp": "2026-04-13T10:30:00Z",
  "command": "install",
  "args": ["code-review"],
  "flags": {"json": true, "yes": true},
  "exit_code": 0,
  "duration_ms": 1250,
  "actor": "agent",
  "tty": false,
  "output_bytes": 342,
  "scribe_version": "0.12.0"
}
```

**`scribe metrics` command** aggregates telemetry: success rate by command, error distribution by code, retry frequency, average duration. Output as JSON for dashboard ingestion.

**Golden dataset for regression.** Maintain 20–50 scripted agent scenarios: install a skill, sync from manifest, handle a conflict, recover from network error. Run these as CI tests on every release. Any regression in success rate blocks the release.

**Context window impact tracking.** Measure output token count per command. If `scribe list --json` returns 50KB for 10 skills, agents are paying unnecessarily. Track average output size and set budgets per command. The `--fields` flag enables agents to request only what they need.

### Dogfooding signals

Use Scribe from Claude Code daily. Track friction points: how often Claude Code misinterprets output, fails to parse errors, or retries unnecessarily. Each friction point is a bug. Maintain a friction log with severity ratings and resolution status. The team's own experience as Scribe-via-agent users is the most sensitive signal.

---

## 10. Resources: standards, articles, and implementations

### Essential reading

The three defining articles on agent-first CLI design emerged in early 2026. Justin Poehnelt's "You Need to Rewrite Your CLI for AI Agents" provides the comprehensive blueprint including schema introspection, context window discipline, and multi-surface architecture. Ugo Enyioha's "Writing CLI Tools That AI Agents Actually Want to Use" distills eight practical rules covering structured output, exit codes, idempotency, and composability. The InfoQ piece "Patterns for AI Agent Driven CLIs" (August 2025) provides the cautionary tales including the AWS pager incident.

### Protocol specifications

The **Model Context Protocol specification** (version 2025-11-25) at `modelcontextprotocol.io` defines tool schemas, transport mechanisms, error handling, and the new Tasks abstraction for async operations. The specification was donated to the Linux Foundation's Agentic AI Foundation in December 2025. Claude's tool-use documentation at `platform.claude.com` details the `tool_use`/`tool_result` lifecycle, `strict` mode for schema enforcement, and `input_examples` for improved accuracy. OpenAI's function calling guide covers the parallel Responses API approach.

### Go implementations

The official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) — maintained in collaboration with Google — is the recommended implementation for adding MCP support to a Go CLI. The community `mcp-go` library (`github.com/mark3labs/mcp-go`) offers backward compatibility across all spec versions. Go's own `gopls` language server includes an experimental MCP server, demonstrating the dual-surface (LSP + MCP) pattern from a single binary.

### Reference implementations

Google's Workspace CLI (`gws`) implements the exact architecture recommended here: one binary serving CLI, MCP, and Gemini extension surfaces, with schema introspection via `gws schema <method>`. The `mcptools` CLI demonstrates MCP server interaction patterns (`mcp call`, `mcp list`, `mcp shell`). Homebrew's `brew bundle` shows declarative package management via manifest files. The `awesome-claude-code-toolkit` repository (`github.com/rohitg00/awesome-claude-code-toolkit`) collects agent, skill, plugin, and hook examples for Claude Code.

### Emerging standards

The `agents.md` specification (GitHub issue #136) proposes a standard environment variable for AI agent runtime detection, analogous to `CI=true`. The JSON Agents specification at `jsonagents.org/cli` defines a portable agent manifest format with CLI validation tools. Google's Agent2Agent (A2A) protocol uses AgentCard (JSON) for cross-agent discovery. OpenTelemetry's GenAI semantic conventions provide standardized attributes for agent operation telemetry.

---

## Conclusion: the agent-first inversion

The shift from human-first to agent-first isn't adding `--json` flags — it's inverting the design hierarchy. In human-first design, the pretty output is the primary path and JSON is an afterthought. In agent-first design, **structured JSON is the primary output** and human formatting is a rendering layer on top. Exit codes aren't status indicators — they're control flow primitives. Error messages aren't explanations — they're data structures carrying codes, affected resources, and executable remediation steps.

Scribe's strongest advantage is timing. The agentic CLI design patterns crystallized in 2025–2026 with MCP, tool-use protocols, and reference implementations like `gws`. Building on these patterns rather than inventing from scratch means Scribe can implement proven designs. The three-phase plan — foundation work on exit codes and structured errors (weeks 1–2), core agent experience with dry-run and idempotency (weeks 3–4), then strategic capabilities like MCP and lockfiles (months 2–3) — delivers compounding value at each stage. Every week of the foundation work immediately reduces agent failure rates.

The measurement discipline — tracking completion rates, retry rates, and parse errors from day one — creates a feedback loop that makes every subsequent decision data-driven rather than intuition-driven. The target is clear: **>90% of agent-initiated operations succeed on first attempt, <10% require human intervention.** These numbers are achievable with the patterns described here, because the failure modes are well-understood and the solutions are proven across `gh`, `kubectl`, `terraform`, and the package manager ecosystem.
