# Commands reference

Every scribe subcommand, grouped by what you reach for it for.

For machine-readable details (input flags, output schema, exit codes), pair this page with [`docs/json-envelope.md`](json-envelope.md) and `scribe schema <command> --json`.

## Daily use

| Command | What it does |
|---|---|
| `scribe` | Open the local skill manager (interactive TUI when stdout is a TTY) |
| `scribe list` | Show all skills on this machine (managed + unmanaged) |
| `scribe browse` | Discover and install skills from connected registries |
| `scribe add [query]` | Find and install skills from registries (legacy; prefer `browse`) |
| `scribe install --all` | Install every catalog entry from a registry in one shot |
| `scribe adopt [name]` | Import hand-rolled skills from `~/.claude/skills` etc. into the canonical store |
| `scribe remove <skill>` | Remove a skill from this machine (records a deny-list entry so it does not come back on the next sync) |
| `scribe sync` | Reconcile the current project: resolve `.scribe.yaml` (kits, snippets, MCP, add/remove), project skills into `<project>/.claude/skills/` and `<project>/.agents/skills/`, write snippet blocks into `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` / `.cursor/rules/*.mdc`, and scope selected `.mcp.json` definitions into Claude, Codex, and Cursor project config |
| `scribe show` | Show the resolved project skill set and per-agent budgets |
| `scribe check` | Check connected registries for lockfile updates without modifying anything |
| `scribe update --apply` | Refresh registry lockfiles after review (omit `--apply` for a dry-run report) |
| `scribe push <skill>` | Push local skill edits back to their source registry |
| `scribe init` | Scaffold a `scribe.yaml` package manifest from `SKILL.md` files in the current directory |
| `scribe doctor` | Inspect managed skills and projections (skills + snippets) for repairable issues |
| `scribe doctor --fix` | Normalize canonical skill metadata and repair affected projections |
| `scribe doctor --skill <name> --fix` | Repair a single managed skill and its projections |
| `scribe skill repair <skill> --tool <tool>` | Resolve drift when a tool-local copy diverges from the canonical store |
| `scribe status` | Show connected registries, installed count, and last sync |
| `scribe tools` | List detected AI tools on this machine; enable, disable, or register custom ones |
| `scribe explain <skill-or-snippet>` | AI-powered explanation for an installed skill or snippet (or `--raw` for the rendered body) |
| `scribe upgrade-agent` | Refresh the embedded scribe bootstrap skill |

## Registry management

| Command | What it does |
|---|---|
| `scribe registry connect <repo>` | Connect to a skill registry (alias: `scribe connect`) |
| `scribe registry create` | Scaffold a new registry repo on GitHub interactively |
| `scribe registry add [name]` | Share a local skill into a connected registry |
| `scribe registry list` | Show connected registries with skill counts |
| `scribe registry enable <repo>` / `disable <repo>` | Toggle a connected registry without forgetting it |
| `scribe registry forget <repo>` | Disconnect a registry (does not remove already-installed skills) |
| `scribe registry resync <repo>` | Force-fetch a registry's catalog from upstream |
| `scribe registry migrate` | Convert a `scribe.toml` registry to `scribe.yaml` |

## Skills and tools

| Command | What it does |
|---|---|
| `scribe skill edit <name>` | Edit per-skill metadata (`--add`, `--remove`, `--inherit`, `--pin`, `--tools`) |
| `scribe skill repair <name>` | Re-write a tool-facing projection from the canonical store |
| `scribe skill tools <name>` | Per-skill tool projection controls (`--enable`, `--disable`, `--reset`) |
| `scribe tools` | List, enable, or disable detected tools machine-wide |
| `scribe tools add` | Register a custom tool integration (`--detect`, `--install`, `--path`, `--uninstall`) |
| `scribe kit create <name>` | Create a local kit — a named list of skills and MCP servers scoped to a project (saved to `~/.scribe/kits/<name>.yaml`). Use `--skills`, `--mcp-servers`, and `--registry` to populate it. Reference the kit by name in a project's `.scribe.yaml` under `kits:`. |
| `scribe kit list` | List local kits from `~/.scribe/kits/*.yaml`. Supports `--fields` and `--json` for agent-readable output. |
| `scribe kit show <name>` | Show one local kit, including skills and source metadata. |
| `scribe project init` | Create a committed project `.scribe.yaml` for repo-local loadouts. Use `--kits a,b` for non-interactive setup or pick from local kits interactively. |

## Conflicts and recovery

| Command | What it does |
|---|---|
| `scribe resolve <skill>` | Resolve a sync conflict with `--ours` (keep canonical) or `--theirs` (accept tool-local) |
| `scribe restore <skill>` | Restore a skill from the canonical store after manual deletion |

## Configuration

| Command | What it does |
|---|---|
| `scribe config` | Show current `~/.scribe/config.yaml` |
| `scribe config set <key> <value>` | Set a config value (e.g. `editor`) |
| `scribe config adoption` | Show adoption settings (`--mode`, `--add-path`, `--remove-path`) |

## Other

| Command | What it does |
|---|---|
| `scribe guide` | Interactive setup guide |
| `scribe schema <command> --json` | Print the JSON Schema for a command's `--json` envelope (input + output) |
| `scribe schema --all --json` | List every command that has a published schema |
| `scribe upgrade --check` | Check whether a newer scribe release is available |
| `scribe --version` | Show the installed version |

## Discovery tips

- `scribe schema --all --json | jq 'keys'` — list every command whose `--json` output is migrated to the v1 envelope. Anything not in that list still emits its pre-envelope shape.
- `scribe schema <command> --json | jq '.data.input_schema.properties'` — see the flag schema before composing a call.
- `scribe list --json --fields name,managed,targets` — project specific columns from a tabular command.
- `scribe doctor --json` — health report without running any fixes.

## Where to look next

- Agent-friendly envelope, exit codes, partial success: [`json-envelope.md`](json-envelope.md)
- Project-level config (`.scribe.yaml`), kits, snippets: [`projects-and-kits.md`](projects-and-kits.md)
- Adopting hand-rolled skills already on the machine: [`adoption.md`](adoption.md)
- Diagnosing drift with `scribe doctor`: [`troubleshooting.md`](troubleshooting.md)
