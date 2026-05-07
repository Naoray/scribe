# Projects, kits, and snippets

Scribe scopes skill availability to the project you're working in, instead of dumping every installed skill into every session. The pieces:

- `.scribe.yaml` — a per-repo declaration of which kits, snippets, or extra skills the project wants
- **Kits** — named, reusable bundles of skills, stackable per project
- **Snippets** — rules / behavior directives injected into agent rules files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`) and Cursor rules (`.cursor/rules/<name>.mdc`)

Full design rationale lives in [`docs/superpowers/specs/2026-04-29-kits-and-snippets-design.md`](superpowers/specs/2026-04-29-kits-and-snippets-design.md). This page documents the shipped surface.

## Why projection moved project-local

Earlier scribe symlinked `~/.scribe/skills/<name>` → `~/.claude/skills/<name>`, machine-globally. Every session saw every skill — bad for Codex's 5440-byte description budget (issue #114) and bad for routing quality once you accumulate hundreds of skills.

Now scribe symlinks into `<project>/.claude/skills/<name>` and `<project>/.agents/skills/<name>`. Both Claude Code and Codex already read project-local skill directories without any flag (Codex reads `.agents/skills/`). The canonical store at `~/.scribe/skills/` is unchanged.

Effect: a session's skill set is determined by `cwd`. Two repos see two different skill sets, even if you only ran `scribe sync` once. Parallel sessions in the same repo with different kits are handled by anvil worktrees.

## `.scribe.yaml`

A `.scribe.yaml` at a project root declares the project's intent. All keys are optional. Run `scribe project init` to scaffold the file, optionally with `--kits web,backend` for non-interactive setup.

```yaml
# .scribe.yaml — committed to the repo so the team shares the same skill set
kits:
  - laravel-baseline
snippets:
  - commit-discipline
mcp:
  - mempalace
add:
  - owner/repo:extra-skill
remove:
  - skill-this-project-doesnt-want
```

Empty or missing files are treated as "no project-level intent" — sync still runs, but nothing is added or removed at the project level.

The dotfile (`.scribe.yaml`) is distinct from `scribe.yaml` used inside skill **registry** repos as a manifest. Same format, different role, different name.

## Kits

A kit is a curated, stackable list of skills. Kits live under `~/.scribe/kits/<name>.yaml`.

```yaml
# ~/.scribe/kits/laravel-baseline.yaml
apiVersion: scribe/v1
kind: Kit
name: laravel-baseline
description: Default skill set for Laravel app work
skills:
  - init-laravel
  - init-livewire
  - init-filament
  - tdd
  - debugger
mcp_servers:
  - mempalace
  - laravel-boost
```

Projects list which kits they want via `kits:` in `.scribe.yaml`. Multiple kits union; the project may add or remove individual skills on top with `add:` / `remove:`. MCP servers can also be declared through kits or directly in `.scribe.yaml` with `mcp:` / `mcp_servers:`; `scribe sync` uses those names to select definitions from project `.mcp.json`. Claude gets enabled server names in `.claude/settings.json`, Codex gets selected definitions in `.codex/config.toml`, and Cursor gets selected definitions in `.cursor/mcp.json`. Existing unmanaged Codex/Cursor entries are preserved; Scribe only replaces entries it previously projected. Scribe does not start MCP server processes.

### Authoring kits and snippets (today)

Snippet **projection** ships in v1.1.0 — `scribe sync` writes snippet bodies into project agent rules files and Cursor rules. A user-facing snippet authoring CLI is still absent; agents (or you, by hand) write snippet markdown into `~/.scribe/snippets/<name>.md` and reference it from `.scribe.yaml`. The embedded scribe skill (installed automatically the first time you run scribe in any supported agent session — Claude Code, Codex, Cursor, Gemini, or a custom tool registered via `scribe tools add`) knows the snippet schema and will scaffold one for you. **Ask your AI agent.**

Examples:

```text
You: Create a kit called web-baseline with tdd, code-review, commit-message, and the mempalace MCP server.
Agent: <runs `scribe kit create web-baseline --skills tdd,code-review,commit-message --mcp-servers mempalace`, then `scribe sync`>

You: Add a snippet that enforces commit discipline, target Claude and Codex.
Agent: <writes ~/.scribe/snippets/commit-discipline.md, lists targets in frontmatter>

You: Wire web-baseline into this project.
Agent: <edits .scribe.yaml in the repo root, runs `scribe sync`>
```

The agent uses `scribe kit create` for kits, the snippet schema below (markdown frontmatter) for snippets, then runs `scribe sync` to apply changes. If `.scribe.yaml` changes or generated agent files drift from it, run `scribe sync` again before assuming the active loadout is current.

If you want to author by hand, the YAML files at `~/.scribe/kits/<name>.yaml` and `~/.scribe/snippets/<name>.md` are the source of truth — `scribe sync` picks them up on every run.

Kits are **not** personas. The forward path for role specialization (adversarial review, security/a11y experts, context isolation) is subagent routing metadata inside kits — not a separate primitive. That metadata is not in the v1 surface.

### Codex budget guardrail

Codex's 5440-byte skill-description budget is enforced as a real refusal, not a warning:

- below 70% of budget → silent
- 70%–100% → warn but proceed
- at or above 100% → refuse with the list of skills causing the overflow

You retry with `--force` or trim the kit. The structural guardrail makes the original Codex truncation bug impossible to reintroduce by accident.

## Snippets

A snippet is a Markdown body with frontmatter declaring which agents it targets. Scribe injects snippet bodies into the project's agent rules files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`) inside scribe-managed marker blocks and writes Cursor snippets to `.cursor/rules/<name>.mdc`.

```markdown
---
name: commit-discipline
description: Commit-message rules and agent commit discipline
targets: [claude, codex, cursor]
---

Commit after each logical phase of work, not just at the end.
Use `[agent]` prefix on every commit message...
```

Snippets stack like kits — `snippets:` in `.scribe.yaml` lists the order, and scribe writes one block per snippet. Content **outside** the marker blocks is preserved, so you can edit your `CLAUDE.md` freely; scribe only owns the marked region.

Snippets are deliberately plain text in v1: no variables, no conditionals, no liquid-style logic. They exist to share rules, not to template.

## Migration from global projection

Existing global-projection installs are not auto-migrated on upgrade. Scribe ships a compatibility mode that keeps `~/.claude/skills/` symlinks working with a deprecation banner; `scribe migrate global-to-projects` walks projects you select interactively to flip them onto project-local projection.

## Where to look next

- [`commands.md`](commands.md) — full command surface
- [`adoption.md`](adoption.md) — claim hand-rolled skills already on the machine
- [`docs/superpowers/specs/2026-04-29-kits-and-snippets-design.md`](superpowers/specs/2026-04-29-kits-and-snippets-design.md) — full design doc, non-goals, parking lot
