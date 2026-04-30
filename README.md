# scribe

```
 ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|
```

> Agent-first skill manager for Claude Code, Codex, and Cursor. Your team's AI coding skills, always in sync — one manifest, one command.

<!-- TODO: hero terminal screenshot or asciinema GIF of `scribe list` TUI -->

## What it does

AI coding agents work better when you teach them how your team works — code review style, deployment checklists, framework patterns. These [SKILL.md](https://agentskills.io) files live in `~/.claude/skills/`, `~/.codex/skills/`, and similar directories. Sharing them used to mean Slack links and manual copying. Skills got stale; new teammates had no idea what existed.

Scribe makes the skill set declarative.

- **One source of truth.** Put your team's skills in a GitHub repo with a `scribe.yaml` manifest. Teammates run `scribe registry connect` once.
- **Cross-tool projection.** One canonical store under `~/.scribe/skills/` projects to Claude Code, Codex, and Cursor at the right paths.
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

Paste this into Claude Code, Cursor, or Codex with shell access — it installs scribe and registers the agent skill so future sessions pick it up automatically:

```
I want to use Scribe to manage my AI coding-agent skills on this machine.
Repo: https://github.com/Naoray/scribe (setup steps: /blob/main/SKILL.md)

Please set it up for me:
  1. If `scribe --version` fails, install it (prefer brew, fall back to release binary, last resort `go install`).
  2. Register Scribe's own agent-facing skill: `scribe add Naoray/scribe:scribe-agent --yes --json`
  3. Show me `scribe list --json` to confirm.
```

## 60-second start

```bash
scribe registry connect ArtistfyHQ/team-skills   # connect once
scribe sync                                       # install everything
scribe list                                       # verify
```

Run `scribe sync` again anytime to pick up new skills. Setting up a registry from scratch? `scribe registry create` scaffolds the repo, `scribe.yaml`, and connection in one prompt.

For a default starter set, connect `Naoray/scribe-skills-essentials` and run `scribe sync --registry Naoray/scribe-skills-essentials`.

## What you get

`scribe list` opens an interactive TUI on a terminal. Piped or in CI, it emits the JSON envelope:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "packages": [
      { "name": "superpowers", "revision": 1, "sources": ["obra/superpowers"] }
    ],
    "skills": [
      {
        "name": "add-init",
        "description": "Create a new /init-* command.",
        "revision": 1,
        "content_hash": "e42bc8ef",
        "targets": ["claude", "codex", "cursor", "gemini"],
        "managed": true
      }
    ]
  },
  "meta": { "duration_ms": 478, "command": "scribe list", "scribe_version": "dev" }
}
```

`scribe sync` reports a structured envelope per run, with `partial_success` + exit code `10` when any item failed:

```json
{
  "status": "partial_success",
  "format_version": "1",
  "data": {
    "reconcile": { "installed": 2, "relinked": 0, "removed": 2, "conflicts_count": 0 },
    "summary":   { "failed": 1, "installed": 0, "skipped": 73, "updated": 0 }
  }
}
```

## Why scribe?

- **Agents-first**: versioned JSON envelope, JSON Schema for every migrated command, distinct exit codes, machine-readable error remediation. Drops cleanly into Claude Code / Codex / Cursor agent loops.
- **Project-local projection**: scopes skill availability to the project you're in, instead of dumping every installed skill into every session. Keeps Codex inside its 5440-byte description budget by construction.
- **Adoption, not migration**: claims hand-rolled skills already in `~/.claude/skills/` etc. via symlink — nothing moves, nothing breaks, scribe just starts managing them.
- **One manifest, every tool**: `scribe.yaml` works across Claude Code, Codex, and Cursor. No per-tool maintenance.

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
