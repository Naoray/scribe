# Commands reference

Every scribe subcommand, grouped by what you reach for it for.

For machine-readable details (input flags, output schema, exit codes), pair this page with [`docs/json-envelope.md`](json-envelope.md) and `scribe schema <command> --json`.

## Daily use

The commands you'll reach for every session — install, list, sync, and keep skills tidy. Most readers can stay in this section.

| Command | What it does |
|---|---|
| `scribe` | Open the local skill manager (interactive TUI when stdout is a TTY) |
| `scribe list` | Show all skills on this machine (managed + unmanaged) |
| `scribe browse` | Discover and install skills from connected registries. Accepts typed sources via `--source github\|git\|local`, `--repo`, `--url`, `--ref`, `--path`, `--id`. Pass `--resync` to overwrite local edits with the upstream version for modified skills. |
| `scribe add [query]` | Find and install skills from registries (legacy; prefer `browse`). Accepts typed sources via `--source github\|git\|local`, `--repo`, `--url`, `--ref`, `--path`, `--id`. Pass `--resync` to overwrite local edits with the upstream version for modified skills. |
| `scribe install --all` | Install every catalog entry from a registry in one shot |
| `scribe adopt [name]` | Import hand-rolled skills from `~/.claude/skills` etc. into the canonical store |
| `scribe remove <skill>` | Remove a skill from this machine (records a deny-list entry so it does not come back on the next sync) |
| `scribe sync` | Reconcile the current project: resolve `.scribe.yaml` (kits, snippets, MCP, add/remove), project skills into `<project>/.claude/skills/` and `<project>/.agents/skills/`, write snippet blocks into `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` / `.cursor/rules/*.mdc`, and scope selected `.mcp.json` definitions into Claude, Codex, and Cursor project config |
| `scribe project sync` | Publish the current repo's shareable kits and skills into `.ai/` (`.ai/kits`, normalized `.ai/skills`, `.ai/scribe.lock`) so teammates can reproduce the loadout |
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
| `scribe mcp list` | Inspect declared MCP servers in `.scribe.yaml` and how they project into Claude, Codex, and Cursor (`--json` supported) |
| `scribe upgrade-agent` | Refresh the embedded scribe bootstrap skill |

## Registry management

Connect, create, share, and audit the skill registries this machine pulls from. A registry is a GitHub repo with a `scribe.yaml` and a `skills/` directory.

| Command | What it does |
|---|---|
| `scribe registry connect <repo>` | Connect to a skill registry (alias: `scribe connect`). Accepts typed sources via `--source github\|git\|local`, `--repo`, `--url`, `--ref`, `--path`, `--id` alongside the positional `owner/repo`. Pass `--force-kits` to overwrite existing kit files with the same name when the registry publishes kits. |
| `scribe registry create` | Scaffold a new registry repo on GitHub interactively |
| `scribe registry add [name]` | Share a local skill into a connected registry |
| `scribe registry list` | Show connected registries with skill counts |
| `scribe registry index` | Show the local cache of public registries under `~/.scribe/index/registries.json` |
| `scribe registry enable <repo>` / `disable <repo>` | Toggle a connected registry without forgetting it |
| `scribe registry forget <repo>` | Disconnect a registry (does not remove already-installed skills) |
| `scribe registry resync <repo>` | Force-fetch a registry's catalog from upstream. Pass `--refresh-kits` to also re-fetch registry-published kit definitions and write source stamps (becomes the default in the next minor release; a stderr deprecation banner fires otherwise). Combine with `--force-kits` to overwrite hand-authored or other-registry kit files. |
| `scribe registry migrate` | Convert a `scribe.toml` registry to `scribe.yaml` |

## Skills and tools

Tune individual skills, project-author new ones, and decide which AI tools each skill projects to. Use these once your basic install and sync flow is working.

| Command | What it does |
|---|---|
| `scribe skill edit <name>` | Edit per-skill metadata (`--add`, `--remove`, `--inherit`, `--pin`, `--tools`) |
| `scribe skill repair <name>` | Re-write a tool-facing projection from the canonical store |
| `scribe skill tools <name>` | Per-skill tool projection controls (`--enable`, `--disable`, `--reset`) |
| `scribe project skill create <name>` | Create a project-authored local skill and mark it for vendoring on the next `scribe project sync` |
| `scribe project skill claim <name>` | Convert an existing local-origin skill into a project-authored skill |
| `scribe tools` | List, enable, or disable detected tools machine-wide |
| `scribe tools add` | Register a custom tool integration (`--detect`, `--install`, `--path`, `--uninstall`) |
| `scribe kit create <name>` | Create a local kit — a named list of skills and MCP servers scoped to a project (saved to `~/.scribe/kits/<name>.yaml`). Use `--skills`, `--mcp-servers`, and `--registry` to populate it. Reference the kit by name in a project's `.scribe.yaml` under `kits:`. |
| `scribe kit list` | List local kits from `~/.scribe/kits/*.yaml` *and* registry-published kits from every connected registry by default (per-registry skip-and-warn when a registry has no `scribe.yaml` kit manifest). Pass `--local` to skip the network, `--remote` to hide local kits, or `--registry <owner/repo>` to filter both views to one registry. Supports `--fields` and `--json` for agent-readable output. |
| `scribe kit show <name>` | Show one local kit, including skills and source metadata. Use `scribe kit show <owner/repo>:<kit>` to inspect a registry-published kit without installing it. |
| `scribe kit install <registry>:<kit>` | Install a registry-published kit into `~/.scribe/kits/`, stamping the source registry. |
| `scribe kit sync` | Reconcile installed registry-published kits against their upstream registries. |
| `scribe kit push <name>` | Push a local kit back to its source registry. |
| `scribe project init` | Create a committed project `.scribe.yaml` for repo-local loadouts. The picker discovers local kits and kits available from connected registries (selecting a remote kit installs it first). `--kits a,b,owner/repo:c` accepts both local kit names and `owner/repo:name` remote refs for non-interactive setup. |

## Conflicts and recovery

When the canonical store and a tool-local copy disagree, these commands let you pick a winner or recover after manual edits. Pair with `scribe doctor` to find what needs attention.

| Command | What it does |
|---|---|
| `scribe resolve <skill>` | Resolve a sync conflict with `--ours` (keep canonical) or `--theirs` (accept tool-local) |
| `scribe restore <skill>` | Restore a skill from the canonical store after manual deletion |

## Configuration

Inspect or change persistent settings stored in `~/.scribe/config.yaml`. These affect every project on this machine.

| Command | What it does |
|---|---|
| `scribe config` | Show current `~/.scribe/config.yaml` |
| `scribe config set <key> <value>` | Set a config value (e.g. `editor`) |
| `scribe config adoption` | Show adoption settings (`--mode`, `--add-path`, `--remove-path`) |

## Other

Schema introspection for agents, version checks, and the interactive setup guide.

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
