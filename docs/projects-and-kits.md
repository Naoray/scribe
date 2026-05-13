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

A `.scribe.yaml` at a project root declares the project's intent. All keys are optional. Run `scribe project init` to scaffold the file, optionally with `--kits web,backend` for non-interactive setup. The picker lists local kits *and* kits available from connected registries (shown as `owner/repo:name (remote)`); selecting a remote kit installs it first, then writes the local name into `.scribe.yaml`. The flag form accepts the same `owner/repo:name` syntax — useful for non-interactive setup that needs a registry kit you have not installed yet.

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

## Team-sharing a project

`.scribe.yaml` shares intent. To share the artifacts behind that intent, run:

```bash
scribe project sync
```

That writes project-owned artifacts under `.ai/`:

```text
.ai/kits/<name>.yaml
.ai/skills/<project-skill>/SKILL.md
.ai/skills/<project-skill>/.scribe-base.md
.ai/skills/<project-skill>/.scribe-content-hash
.ai/scribe.lock
```

Commit `.scribe.yaml` and `.ai/` together. Do not commit generated tool projections such as `.agents/skills/`, `.claude/skills/`, or `.cursor/rules/` unless your project already manages those files for another reason.

Teammates clone the repo, connect any registries named in `.ai/scribe.lock`, then run `scribe sync`. Project-vendored skills win over global skills with the same name. Registry skills are fetched at the pinned commit from `.ai/scribe.lock`. Project-vendored `SKILL.md` files are normalized with Scribe metadata before hashing, so teammate sync can project them for Codex without dirtying committed `.ai/` files.

Use `scribe project sync --check` in CI to fail when committed `.ai/` artifacts drift from the current `.scribe.yaml` and local author state. Use `--force` only after reviewing project-side changes; it overwrites changed `.ai/` artifacts.

Project-authored skills are explicit:

```bash
scribe project skill create review-guidelines
scribe project sync
```

To promote an existing local skill into the project, run `scribe project skill claim <name>` first. Registry and bootstrap skills cannot be claimed; this prevents silently detaching shared registry content or binary-managed skills.

Laravel Boost projects are supported. When `composer.json` contains `laravel/boost` and a skill is vendored in `.ai/skills`, Scribe leaves Claude's `.claude/skills/<name>` real directory to Boost and projects that skill only to other active tools. The usual run order is:

```bash
php artisan boost:update
scribe sync
```

Snippets and MCP server definitions are still machine/project-local in this release. In team-share mode, missing snippet files or `.mcp.json` definitions warn and skip instead of blocking skill projection.

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

`scribe kit list` shows local kits *and* kits from every connected registry by default, marking remote-only entries so you can spot them in the merged view. Pass `--local` to skip the network call (offline / fast iteration), `--remote` to hide local kits, and `--registry <owner/repo>` to filter both views to a single registry. Registries that lack a `scribe.yaml` kit manifest are skipped with a stderr warning instead of aborting the listing. Use `scribe kit show <owner/repo>:<kit> --json` to inspect a remote kit body without installing it; remote show classifies each skill ref as same-registry, cross-registry, or local and reports whether referenced registries are connected.

### Kits from registries

A registry can ship kits beside its skill catalog. The registry's `scribe.yaml` declares a top-level `kits:` block, each entry pointing at a repo-relative kit YAML file:

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: example
catalog: [...]
kits:
  - name: daily-workflow
    description: Plan, capture, and close the day
    path: kits/daily-workflow.yaml
  - name: release-pipeline
    path: kits/release-pipeline.yaml
```

Omit `path` to use `kits/<name>.yaml`. Manifest validation rejects duplicate kit names, kit names that collide with skill catalog entries, and paths outside the registry repo. Each referenced file is a normal Kit YAML (`kind: Kit`, `name:`, `skills:`, `mcp_servers:`).

`scribe registry connect <repo>` fetches every referenced kit body, validates the body name against the manifest ref, and writes it to `~/.scribe/kits/<name>.yaml` with `source.registry` stamped. From there, `scribe kit list`, `scribe kit show`, `scribe project init --kits`, and `.scribe.yaml` project `kits:` all see it like any local kit.

`scribe registry resync <repo>` keeps its legacy behavior this release: it only clears mute state. Pass `--refresh-kits` to re-fetch registry kit definitions now. The next minor release will refresh kits by default. Use `--force-kits` with connect or resync when you intentionally want registry content to overwrite an existing kit file.

Resolution precedence:

- Project-local kits in `<project>/.scribe/kits/<name>.yaml` win over global kits in `~/.scribe/kits/<name>.yaml`.
- Hand-authored global kits with no `source.registry` are protected by default. Scribe refuses to overwrite them unless you pass `--force-kits`.
- If another registry already owns the same global kit name, Scribe skips that kit. Pass `--force-kits` to overwrite the existing kit, or rename one side by hand in `~/.scribe/kits/<name>.yaml` to keep both.

Legacy `scribe.toml` registry manifests do not support `kits:`. Any such block is ignored by the legacy path; migrate the registry to `scribe.yaml` to publish kits.

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
