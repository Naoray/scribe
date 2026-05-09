# scribe

```
 ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|
```

**Website:** [usescribe.dev](https://usescribe.dev) · **Docs:** [usescribe.dev/docs](https://usescribe.dev/docs/)

A skill manager for AI coding agents. One canonical store of `SKILL.md` files, symlinked into `~/.claude/skills/`, `.cursor/rules/`, `~/.agents/skills/`, and Gemini. Project-scoped, lockfile-pinned, and scriptable through a versioned JSON envelope.

For engineers running Claude Code, Cursor, Codex, or Gemini who got tired of copying skill files between four directories.

## What it does

- **Single canonical store at `~/.scribe/skills/<name>`.** Tool directories get symlinks, never duplicates. Edit the source once; every agent sees the change.
- **Project loadout from `.scribe.yaml`.** Declare which kits (skill bundles), snippets (rules blocks), and MCP server names a repo wants. `scribe sync` projects exactly that set into `<project>/.claude/skills/`, `<project>/.agents/skills/`, `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and `.cursor/rules/<name>.mdc`. No global pollution.
- **Lockfile-pinned.** `scribe.lock` records resolved revisions per registry. Two engineers running `scribe sync` against the same manifest get the same skills.
- **Adopts existing skills via symlink.** `scribe adopt` claims hand-rolled `SKILL.md` files already in `~/.claude/skills/` etc. Files do not move; nothing breaks; scribe just starts managing them.
- **Codex 5440-byte description budget enforced.** Sync refuses at 100% (exit 5) and warns from 70%. You hear about overflow at sync time, not when Codex silently truncates.
- **Versioned JSON envelope on every migrated command.** Stable exit codes, JSON Schema introspection via `scribe schema <cmd> --json`, partial-success semantics. Drops cleanly into an agent loop.

Built-in tool support: Claude Code, Cursor, Codex, Gemini. Register others (Aider, Cline, Roo, your own) with `scribe tools add`.

## In 60 seconds

```console
$ brew install Naoray/tap/scribe
$ scribe init                                        # create ~/.scribe, detect tools
$ scribe registry connect anthropics/skills          # connect a public registry
$ scribe sync                                        # resolve, symlink, write rules blocks
$ scribe list --json --fields name,targets,managed
{
  "status": "ok",
  "format_version": "1",
  "data": [
    { "name": "code-review", "targets": ["claude","codex","cursor"], "managed": true },
    { "name": "tdd",         "targets": ["claude","codex"],          "managed": true }
  ],
  "meta": { "duration_ms": 41, "command": "scribe list" }
}
```

That set of skills now lives at `~/.scribe/skills/`. Your agents see it through symlinks. Re-run `scribe sync` after edits or registry updates.

## How it fits

Scribe is the projection layer between an agent skill format and the directories your tools actually read. It does **not** replace:

- **The `SKILL.md` format.** Scribe consumes the [agentskills.io](https://agentskills.io) spec; it does not invent a new one. Anything that works with `skills.sh` or Paks works here.
- **Your agent.** Claude Code, Cursor, Codex, and Gemini still own loading, prompting, and execution. Scribe puts the right files in the right place.
- **MCP.** Scribe writes server *names* into project files (Claude approvals, `.codex/config.toml`, `.cursor/mcp.json`). It does not start MCP processes — that is the agent's job.

What scribe adds: a canonical store, manifest-driven projection, lockfile, project scoping, and a JSON-envelope CLI so the whole thing stays reproducible and agent-readable.

## Why this exists

Teams started writing internal skills — code review style, deployment checklists, framework patterns — and three problems showed up immediately. Skills drifted across tools: the Claude Code copy got edited, the Cursor copy didn't. Teammates ran different revisions because there was no shared lockfile. `CLAUDE.md` and `AGENTS.md` filled with copy-pasted rules blocks that no one trusted to be current.

Scribe's bet is that the store should be the truth and tool directories should be projections. One file at `~/.scribe/skills/code-review/SKILL.md`. Symlinks into every agent's directory. A manifest in the repo that says which skills, kits, and snippets this project wants. A lockfile so the resolution is reproducible. After that, every agent reads the same thing.

Most coding-agent tooling fights over the agent itself. Scribe manages the skills the agent actually loads.

## Agent contract

Migrated commands emit this on stdout:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": { },
  "meta": {
    "duration_ms": 12,
    "bootstrap_ms": 3,
    "command": "scribe list",
    "scribe_version": "1.1.0"
  }
}
```

Errors use the same shape on stderr. Payload always lives under `data` — `jq '.data.foo'`, never `jq '.foo'`.

| Exit | Meaning |
|---:|---|
| 0 | success |
| 2 | usage / invalid flags |
| 3 | not found (command, registry, skill, schema) |
| 4 | permission or auth failure |
| 5 | conflict requiring user action |
| 6 | network failure |
| 7 | local dependency unavailable |
| 8 | validation failure |
| 9 | user canceled |
| 10 | partial success — inspect `data.summary.failed` |

Discover any command's input/output schema:

```bash
scribe schema sync --json
scribe schema --all --json   # every migrated command
```

Full contract in [CLAUDE.md](CLAUDE.md). Envelope spec in [docs/json-envelope.md](docs/json-envelope.md). Trigger-phrase mapping for agent loops in [SKILL.md](SKILL.md).

## Project file (`.scribe.yaml`)

Run `scribe project init` to scaffold one, or write it by hand:

```yaml
kits:
  - laravel-baseline
snippets:
  - commit-discipline
mcp:
  - mempalace
add:
  - anthropics/skills:pdf
remove:
  - skill-this-project-doesnt-want
```

`scribe sync` resolves the kits, applies `add` / `remove`, writes the snippets into the agent rule files (with markers — your unmanaged content is preserved), and projects every selected `.mcp.json` definition into Claude, Codex, and Cursor project config.

### Team-share

For projects shared with teammates, the author runs `scribe project sync` and commits `.scribe.yaml` plus `.ai/kits/`, `.ai/skills/`, and `.ai/scribe.lock`. Teammates clone the repo, connect any registries named in `.ai/scribe.lock`, then run `scribe sync` — they get the same kits, the same skills, at the same pinned revisions. Use `scribe project sync --check` in CI to fail when committed `.ai/` artifacts drift from `.scribe.yaml`. See [docs/projects-and-kits.md](docs/projects-and-kits.md).

## Install

**Homebrew:**

```bash
brew install Naoray/tap/scribe
```

**Binary:**

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

Verify with `scribe --version`. Update via `brew upgrade scribe`, the same `go install` line, or replacing the binary from [releases](https://github.com/Naoray/scribe/releases).

### Install via your agent

Paste this into Claude Code, Codex, Cursor, or Gemini. It bootstraps scribe and registers scribe's own agent-facing skill so future sessions pick it up automatically:

```
I want to use Scribe to manage my AI coding-agent skills on this machine.
Repo: https://github.com/Naoray/scribe (setup steps: /blob/main/SKILL.md)

Please set it up:
  1. If `scribe --version` fails, install it (prefer brew, fall back to release binary, last resort `go install`).
  2. Register Scribe's own agent skill: `scribe add Naoray/scribe:scribe --no-interaction --json`
  3. Show me `scribe list --json` to confirm.
```

## Quick reference

```bash
scribe list                  # local skills (TUI on a terminal, JSON when piped)
scribe adopt                 # claim hand-rolled skills already in tool dirs
scribe sync                  # apply project loadout
scribe project sync          # publish shareable .ai/ artifacts for teammates
scribe show                  # resolved project skill set + per-agent budgets
scribe doctor                # audit and repair managed skill health
scribe tools                 # detected agents (claude, codex, cursor, gemini, custom)
scribe skill tools <name>    # enable / disable / reset projection per skill per tool
scribe registry connect ...  # add a registry
scribe browse --query ...    # search registries before installing
```

## Documentation

- [Comparison](docs/comparison.md) — scribe vs. skills.sh, Superpowers, anthropics/skills, Cursor MDC, Cline/Roo, MCP
- [Commands reference](docs/commands.md) — every subcommand grouped by use
- [JSON envelope + agent contract](docs/json-envelope.md) — envelope shape, exit codes, `--fields`, schema introspection
- [Projects, kits, and snippets](docs/projects-and-kits.md) — `.scribe.yaml`, kits, snippet rules
- [Adoption](docs/adoption.md) — claim hand-rolled skills already on the machine
- [Troubleshooting](docs/troubleshooting.md) — `scribe doctor`, repair flows, common issues

## Status

- v1.1.0 — active development.
- Requires macOS or Linux. Windows install via the PowerShell snippet in [SKILL.md](SKILL.md).
- `gh` CLI recommended for auth (not required for public repos).
- Issues: [github.com/Naoray/scribe/issues](https://github.com/Naoray/scribe/issues) — each one has enough context to pick up.

## Contributing

```bash
git clone https://github.com/Naoray/scribe
cd scribe
go build ./cmd/scribe
go run ./cmd/scribe --help
go test ./...
```

## License

MIT
