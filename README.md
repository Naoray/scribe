# scribe

```
 ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|
```

> Agent-first skill manager for any AI coding agent. Built-in support for Claude Code, Codex, Cursor, and Gemini; register any other agent with `scribe tools add`. Your team's AI coding skills, always in sync — one manifest, one command.

<!-- TODO: hero terminal screenshot or asciinema GIF of `scribe list` TUI -->

## What it does

AI coding agents work better when you teach them how your team works — code review style, deployment checklists, framework patterns. These [SKILL.md](https://agentskills.io) files live in `~/.claude/skills/`, `~/.agents/skills/` (Codex), and similar directories. Sharing them used to mean Slack links and manual copying. Skills got stale; new teammates had no idea what existed.

Scribe makes the skill set declarative.

- **One source of truth.** Put your team's skills in a GitHub repo with a `scribe.yaml` manifest. Teammates run `scribe registry connect` once.
- **Cross-tool projection.** One canonical store under `~/.scribe/skills/` projects to whatever agent you use — Claude Code, Codex, Cursor, and Gemini ship as built-ins; register others (Aider, Cline, Roo, your own tool) with `scribe tools add`.
- **Project-scoped loadouts.** A `.scribe.yaml` per repo declares the **kits** (named skill bundles), **snippets** (rules injected into `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` / `.cursor/rules/*.mdc`), and **MCP server names** that project wants. Skills land in `<project>/.claude/skills/` and `<project>/.agents/skills/` — not machine-globally — so each repo sees only what it asked for.
- **Agent-first, scriptable.** Every migrated command emits a versioned JSON envelope (`{status, format_version, data, meta}`) with exit codes, partial-success semantics, and JSON Schema introspection.

## Install

**Homebrew (recommended):**

```bash
brew install Naoray/tap/scribe
```

**Binary (macOS / Linux):**

```bash
# macOS Apple Silicon
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_arm64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/

# macOS Intel
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_amd64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/

# Linux amd64
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_linux_amd64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/
```

**Go:**

```bash
go install github.com/Naoray/scribe/cmd/scribe@latest
```

Verify: `scribe --version`. Update via `brew upgrade scribe`, the same `go install` line, or replacing the binary from the releases page.

### Install via your agent

Paste this into your AI coding agent (Claude Code, Codex, Cursor, Gemini, or any agent with shell access) — it installs scribe and registers the agent skill so future sessions pick it up automatically:

```
I want to use Scribe to manage my AI coding-agent skills on this machine.
Repo: https://github.com/Naoray/scribe (setup steps: /blob/main/SKILL.md)

Please set it up for me:
  1. If `scribe --version` fails, install it (prefer brew, fall back to release binary, last resort `go install`).
  2. Register Scribe's own agent-facing skill: `scribe add Naoray/scribe:scribe --no-interaction --json`
  3. Show me `scribe list --json` to confirm.
```

## Quick start

```bash
scribe list           # see skills already available across tools
scribe adopt          # claim hand-rolled skills from Claude/Codex/Cursor
scribe sync           # project managed skills, kits, snippets, and MCP names into the current project
scribe show           # show the resolved project skill set and per-agent budgets
```

That is enough to start managing existing local skills between tools. Use `scribe tools` to see detected agents, and `scribe skill tools <name>` to enable, disable, or reset projection for one skill.

Drop a `.scribe.yaml` at the repo root to declare which kits, snippets, extra skills, or MCP server names this project wants — `scribe sync` then projects exactly that set into `<project>/.claude/skills/` + `<project>/.agents/skills/`, writes snippet blocks into `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` plus `.cursor/rules/<name>.mdc`, and scopes selected MCP definitions from `.mcp.json` into Claude, Codex, and Cursor project config. Scribe does not start MCP processes. See [`docs/projects-and-kits.md`](docs/projects-and-kits.md).

Registries are for adding shared/upstream skills. Connect one when you want more than your local set:

```bash
scribe registry connect anthropics/skills
scribe sync
```

## What you get

`scribe list` opens an interactive TUI on a terminal. Piped or in CI, it emits the JSON envelope for skills already available on the machine:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "packages": [],
    "skills": [
      {
        "name": "review-checklist",
        "description": "Apply the team's review checklist before opening a PR.",
        "revision": 1,
        "content_hash": "e42bc8ef",
        "targets": ["claude", "codex"],
        "managed": true,
        "origin": "local"
      }
    ]
  },
  "meta": { "duration_ms": 478, "command": "scribe list", "scribe_version": "dev" }
}
```

`scribe sync` adopts clean local skills, reconciles projections, and reports a structured envelope per run:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "adoption": { "adopted": 1, "skipped": 0, "conflicted": 0 },
    "reconcile": { "installed": 1, "relinked": 2, "removed": 0, "conflicts_count": 0 },
    "summary": { "failed": 0, "installed": 0, "skipped": 0, "updated": 0 }
  }
}
```

## Why scribe?

- **Agents-first**: versioned JSON envelope, JSON Schema for every migrated command, distinct exit codes, machine-readable error remediation. Drops cleanly into any agent loop — Claude Code, Codex, Cursor, Gemini, or a custom tool you registered with `scribe tools add`.
- **Project-local projection**: scopes skill availability to the project you're in, instead of dumping every installed skill into every session. Keeps Codex inside its 5440-byte description budget by construction.
- **Adoption, not migration**: claims hand-rolled skills already in `~/.claude/skills/` etc. via symlink — nothing moves, nothing breaks, scribe just starts managing them.
- **One manifest, every tool**: `scribe.yaml` works across every supported agent — built-ins (Claude Code, Codex, Cursor, Gemini) plus any custom tool you register. No per-tool maintenance.

## Documentation

- [Comparison](docs/comparison.md) — how scribe compares with skills.sh, Superpowers, Anthropic skills, Cursor rules, Cline/Roo, and MCP
- [Commands reference](docs/commands.md) — every subcommand grouped by use
- [JSON envelope + agent contract](docs/json-envelope.md) — envelope shape, exit codes, `--fields`, schema introspection
- [Projects, kits, and snippets](docs/projects-and-kits.md) — `.scribe.yaml`, kits, snippet rules
- [Adoption](docs/adoption.md) — claim hand-rolled skills already on the machine
- [Troubleshooting](docs/troubleshooting.md) — `scribe doctor`, repair flows, common issues

Skill format follows [agentskills.io](https://agentskills.io). Anything that works with `skills.sh` or Paks works with scribe.

## Requirements

- macOS or Linux
- GitHub account with access to your team's skills repo
- `gh` CLI recommended for auth (not required for public repos)

## Contributing

See the [open issues](https://github.com/Naoray/scribe/issues) — each one has enough context to pick up and run with.

```bash
git clone https://github.com/Naoray/scribe
cd scribe
go build ./cmd/scribe
go run ./cmd/scribe --help
go test ./...
```

## License

MIT
