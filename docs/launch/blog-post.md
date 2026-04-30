# Scribe 1.0: One manifest for every agent's skills  **AWAITING-VOICE-REVIEW**

> *Voice + technical claims need user review before publication. Do not publish as-is.*

AI coding agents work better when you teach them how your team works — code review style, deployment checklists, the framework patterns you actually use. Lately every agent has its own way to load that knowledge: `~/.claude/skills/`, `~/.codex/skills/`, `.cursor/rules/`, plugin folders, dotfiles. Sharing it has meant Slack links and copy-paste. New teammates have no idea what exists. Skills go stale silently. Three agents on one laptop means three separate stores, none of them in sync.

Scribe 1.0 is the skill manager that fixes this. One manifest, one command, every agent.

## The problem

Skill files are simple — a `SKILL.md` with frontmatter, often a folder of supporting scripts. The hard part is everything around them: distribution, versioning, scoping, attribution, and keeping a fleet of agents on the same page.

Today, most teams handle that manually:

- Someone writes a `tdd.md` and pastes it in the team channel.
- Half the team copies it into `~/.claude/skills/`. The other half forgets.
- Two weeks later, a small fix lands in the original. Nobody resyncs.
- A teammate switches from Claude Code to Codex and starts from scratch.
- A new project inherits every skill on the laptop, blowing past Codex's 5440-byte session-description budget.
- A reviewer asks "where did this skill come from?" and there's no answer in the file.

There is no `package.json` for agent skills. No lockfile. No "what's installed and why." No reproducible setup for a new hire. The Agent Skills spec gave us a portable file format; what's missing is the thing that ships, pins, projects, and adopts those files across tools.

## What scribe does differently

Scribe treats skills like packages and treats agents like first-class consumers.

**One source of truth.** Put your team's skills in a GitHub repo with a `scribe.yaml` manifest. Teammates run `scribe registry connect <owner>/<repo>` once. Updates land via `scribe sync`. The same manifest works for everyone, no per-tool maintenance.

**Reproducible installs.** `scribe.lock` pins each entry by `commit_sha` and `content_hash`. `scribe sync` refuses to run when the lockfile diverges from what's on disk; `scribe check` lists what would change; `scribe update` advances the lock after review. This is the workflow you already have for npm, Cargo, or Go modules — applied to the layer above the language runtime.

**Cross-tool projection.** One canonical store under `~/.scribe/skills/` projects to Claude Code, Codex, Cursor, and Gemini at the right paths. Add a tool integration once and every existing skill picks it up. No duplicated copies in three different folders that drift on the next edit.

**Project-local, not global.** A `.scribe.yaml` in the project directory declares which kits and skills belong to *that* project. The agent only sees what's relevant. Cross-cutting skills (review, TDD) stay global; per-stack skills (Laravel patterns, Tauri integration) stay scoped. The Codex description budget gets enforced by construction, not by guesswork.

**Adoption, not migration.** You probably already have hand-rolled skills in `~/.claude/skills/`. `scribe adopt` claims them via symlink — nothing moves, nothing breaks, scribe just starts managing them. You can roll back at any time.

**Author publishing.** `scribe init` scaffolds a package manifest by discovering the `SKILL.md` files in the current directory. `scribe registry create` scaffolds a registry repo on GitHub. `scribe push` ships local skill edits back upstream. The path from "I just wrote a skill" to "the team has it" is two commands.

**Agent-first contract.** Every migrated command emits a versioned JSON envelope:

```json
{ "status": "ok", "format_version": "1", "data": { ... }, "meta": { ... } }
```

Mutators use semantic exit codes — `2` for usage errors, `3` not-found, `4` permission, `5` conflict, `6` network, `7` dependency, `8` validation, `9` user-canceled, `10` partial success. `scribe schema <command> --json` returns JSON Schema 2020-12 for inputs and outputs, so an agent can compose calls without guessing flags. `--fields name,version` projects tabular output gh-style. Designed for `jq` and for Claude Code / Codex / Cursor agent loops.

## Walkthrough

The 60-second story, from a fresh machine to a synced project:

```bash
brew install Naoray/tap/scribe

scribe registry connect Naoray/scribe-skills-essentials
scribe sync --all
scribe list
```

That's it. The first command installs scribe. The second connects a curated starter registry. The third walks the registry catalog, writes `scribe.lock`, projects every skill into the right tool directories, and reports a structured summary. The fourth opens the interactive TUI (or emits the JSON envelope when piped) so you can see what landed and where.

For team setup, swap the registry for your own:

```bash
scribe registry connect ArtistfyHQ/team-skills
scribe sync
```

Commit the resulting `scribe.lock` to your project. The next teammate runs `scribe sync` and gets the exact same skill set, byte-for-byte.

For authoring:

```bash
cd my-skills-repo
scribe init           # scaffolds scribe.yaml from existing SKILL.md files
git add scribe.yaml && git commit -m "scribe manifest"
git push

scribe registry add Naoray/scribe-skills-essentials   # if you have access
scribe push my-skill                                   # PRs the local edit upstream
```

The asciinema cast linked at the top walks through the same flow in 90 seconds.

## What v1.0 commits to

Tagging 1.0 means scribe stops moving the floor under integrators. Specifically:

- The JSON envelope is `format_version: "1"` and stable. Breaking changes bump `format_version`.
- Exit codes 0-10 are stable. New conditions get new codes, not reassignments.
- `scribe schema <command> --json` is the source of truth for flags and payloads. If a flag is in the schema, scripts can rely on it.
- `scribe.yaml` and `scribe.lock` follow SemVer. `format_version` in each makes future migrations explicit.
- `scribe.yaml` (registries) and `.scribe.yaml` (projects) are forward-compatible: unknown fields are ignored.

A handful of older mutator commands (`install`, `remove`, `resolve`, `restore`, `skill`, `tools`, `config`, `create`, `registry*`, `upgrade`, `migrate`, `browse`) still emit pre-envelope output and reject `--json` with `JSON_NOT_SUPPORTED`. They migrate in the next minor release without changing argument semantics. `scribe schema --all --json` is the live source of truth for which commands are migrated.

Deprecations get a one-minor-release warning window with `DEPRECATED:` prefixed in stderr and the schema, before removal in the following minor.

## What's next

A few of the things on deck for 1.x:

- **Kits and snippets, end to end.** The schema and resolver are in; the user-facing `scribe kit` and `scribe snippet` commands are next so a project can pull in `laravel-baseline` or `react-typescript` as one entry instead of seven.
- **Hooks, opt-in.** `scribe-hook.sh` and the installer are shipped; the next step is wiring `sync` and `doctor` into Claude Code session lifecycle hooks so a project's skill set is always current when an agent starts.
- **Public registry index.** Today, you discover registries by URL. Next, an opt-in directory makes "what skill registries are out there" answerable from the CLI.
- **Per-skill tool overrides.** `scribe skill edit --tools` ships; a richer tool-affinity language for "this skill only matters in Claude Code" is on deck.

Some of this depends on what teams actually need. The [open issues](https://github.com/Naoray/scribe/issues) are the working list.

## Get started

```bash
brew install Naoray/tap/scribe
scribe registry connect Naoray/scribe-skills-essentials
scribe sync --all
scribe list
```

Or paste this into Claude Code, Cursor, or Codex with shell access:

> I want to use Scribe to manage my AI coding-agent skills on this machine.
> Repo: https://github.com/Naoray/scribe (setup steps: /blob/main/SKILL.md)
>
> Please set it up: install if missing, run `scribe add Naoray/scribe:scribe-agent --yes --json`, then show me `scribe list --json`.

The agent picks it up from there. Future sessions register the bootstrap skill automatically.

Skill format follows [agentskills.io](https://agentskills.io) — anything that works with `skills.sh` or Paks works with scribe.

Compare with [skills.sh, Superpowers, Anthropic skills, Cursor rules, Cline/Roo, and MCP](../comparison.md) if you're picking a tool.

---

*AWAITING-VOICE-REVIEW: voice and technical claims need user review before publication. Cross-check the "What v1.0 commits to" section against the final SemVer policy. Confirm the `scribe push` and `scribe init` flows match the shipping CLI before publishing externally.*
