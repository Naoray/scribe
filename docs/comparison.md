# Scribe vs alternatives

Scribe is for teams that want a reproducible, cross-agent way to publish, install, scope, and update coding-agent skills from registries. The closest direct peers are **xingkongliang/skills-manager**, **pivoshenko/Kasetto**, and **803/skills-supply** — three projects that, like Scribe, treat `SKILL.md` as the unit and project it across multiple agents. **GitHub Copilot custom instructions** sit nearby as the most-used per-repo rule renderer. Scribe is _not_ a replacement for an IDE agent, MCP servers, or coding methodologies (see [Different layer](#different-layer-not-an-alternative)).

## Feature comparison

| Feature | Scribe | xingkongliang/skills-manager | pivoshenko/Kasetto | 803/skills-supply | GitHub Copilot custom instructions |
|---|---|---|---|---|---|
| Primary job | Skill manager: publish, install, project, sync | Desktop GUI for cross-tool `SKILL.md` library | Declarative agent environment manager (Rust CLI) | One-manifest skill installer for multiple agents | Repo-scoped instructions for Copilot Chat, code review, agent |
| Where instructions live | `~/.scribe/` (canonical), projected to `.claude/`, `.cursor/`, `.codex/`, `.gemini/` | `~/.skills-manager/` (default), GUI-managed | `kasetto.yaml` + lockfile, projected to agent dirs | `agents.toml` per project or global | `.github/copilot-instructions.md`, `.github/instructions/*.instructions.md`, `AGENTS.md` |
| Cross-tool support | Claude Code, Codex, Cursor, Gemini, custom | Cursor, Claude Code, Codex, OpenCode, Amp, Roo Code, Gemini CLI, Copilot, Windsurf | 21 built-in agent presets (Claude Code, Cursor, Codex, Windsurf, Copilot, Gemini CLI, …) | Claude Code, Amp, Codex, OpenCode, Factory | Copilot (Chat, code review, cloud agent). `AGENTS.md` is also read by Claude (`CLAUDE.md`) and Gemini (`GEMINI.md`). |
| Path/file scoping | `.scribe.yaml` per project, kits, snippets | "Scenarios" + "Project Workspaces" | Per-skill selection + project/global scope | `agents.toml` (project or global) | `applyTo: "src/**/*.ts"` glob frontmatter on `*.instructions.md` |
| Reproducibility | `scribe.lock` lockfile, `scribe sync`, `scribe doctor` | Git backup/restore, snapshot tags, multi-machine sync (no lockfile) | Lockfile + pinned refs/tags/commits + `doctor` | Tracked state + reconciliation, no user-facing lockfile | Plain markdown in git, no lockfile or sync command |
| Discovery | Connected registries (Naoray, anthropics, custom) | Git repos, local folders, archives, skills.sh marketplace | GitHub / GitLab / Bitbucket / Codeberg / Gitea / self-hosted, local paths, remote team config URLs | GitHub, arbitrary Git remotes, local paths, Claude plugin marketplaces | None — write your own, or borrow from `awesome-copilot` |
| Existing skill adoption | `scribe adopt` claims unmanaged local skills via symlink | Project Workspaces compare project-local skill folders with the central library and sync either direction | Limited — sync from local folders, no one-shot claim | Refuses to overwrite manually-added skills; can reset state to mark them "unmanaged" | N/A — instructions are author-only |
| Per-project state | `.scribe.yaml`, `.ai/scribe.lock`, projection map | Project Workspaces with per-agent assignment | Project-scope install + selectable subset | `agents.toml` per project | Repo-scoped instructions in `.github/`, but no manifest or lockfile |
| Budget enforcement | Codex skill-description guardrail in `scribe doctor` | Not documented | Not documented | Not documented | Not documented |
| Schema / introspection | JSON envelopes, `scribe schema <command>`, exit codes | `--json` output for scripts and agents | `sync`, `list`, `doctor`, `clean`, self-update all expose `--json` | Scriptable via `--non-interactive`, no JSON output | Not applicable — natural-language markdown |
| Best at | Multi-agent skill lifecycle and team sync | Desktop-first cross-tool skill library with backup/sync | Declarative reproducible agent environments (CLI) | Manifest-driven skill sync across agents | Tightly-scoped repo instructions for Copilot users |

## When to pick Scribe

- You maintain team skills in Git and want one manifest-driven install/update flow across multiple agent tools.
- You need reproducible sync, lockfiles, repair commands, and machine-readable output for agent automation.
- You want project-scoped skill projection so one repository does not inherit every skill installed on the machine.
- You already have hand-written skills and want to adopt them without moving or rewriting them first.

## When to pick an alternative

- Pick **xingkongliang/skills-manager** if you want a polished desktop GUI as the primary surface, and value built-in backup/restore + multi-machine sync over a CLI-first lockfile model.
- Pick **pivoshenko/Kasetto** if you want a declarative, Rust-based CLI with a strong lockfile + `--json` everywhere, and don't yet need an `adopt`-style claim flow.
- Pick **803/skills-supply** if a single `agents.toml` per project covers your needs and you don't need a registry/lockfile/adopt bundle.
- Pick **GitHub Copilot custom instructions** if your team is all-in on Copilot and a path-scoped `*.instructions.md` plus a repo `copilot-instructions.md` cover what you need. Lighter than Scribe, but no lockfile, no project state, no cross-tool projection.
- Use **skills.sh / Agent Skills** as the format underneath any of the above. Scribe consumes `SKILL.md` directly; most peers do too. It is a spec, not a tool.

## Related projects (partial overlap, broader category)

These solve a slice of the same problem but sit in adjacent categories:

- **[pr-pm/prpm](https://github.com/pr-pm/prpm)** — universal registry that auto-converts one package into vendor-native formats (Cursor rules, Claude skills, Copilot instructions, …). Strong registry/conversion, weaker on canonical local storage and adoption.
- **[enulus/OpenPackage](https://github.com/enulus/OpenPackage)** — package manager for agent configs (rules, commands, agents, skills, MCP) with project/global scope and a wide platform matrix. Broader than Scribe; not `SKILL.md`-first.
- **[frmlabz/omnidev](https://github.com/frmlabz/omnidev)** — capability format (`omni.toml` / `OMNI.md`) with provider-specific generation and profile switching. Alpha-stage; provider-renderer rather than skill manager.
- **[dennishavermans/agentfile](https://github.com/dennishavermans/agentfile)** — single committed `ai/contract.yaml` rendered into `CLAUDE.md`, `.cursor/rules/*.mdc`, `.github/copilot-instructions.md`, `AGENTS.md`. Cross-tool contract renderer; no registry, no central skill store.
- **[KrystianJonca/lnai](https://github.com/KrystianJonca/lnai)** — define once in `.ai/`, sync everywhere. One-config renderer; no registry, no lockfile.

## Different layer, not an alternative

- **MCP servers** solve runtime tool/data access (calling GitHub, reading a database, connecting to internal services). Scribe and Copilot instructions ship _prompts and skills_; MCP ships _capabilities_. Most teams want both.
- **IDE agents** (Cline, Roo Code, Cursor agent mode, Claude Code) are the runtime that _executes_ skills. Scribe is the source-of-truth and delivery system; the agent is the consumer.
- **Coding methodologies** (TDD-first, agentic plan/build/review loops) are _how_ an agent works. Scribe is _what_ the agent reads.

## References

- [scribe README](../README.md)
- [scribe commands reference](commands.md)
- [scribe projects, kits, and snippets](projects-and-kits.md)
- [xingkongliang/skills-manager](https://github.com/xingkongliang/skills-manager)
- [pivoshenko/Kasetto](https://github.com/pivoshenko/Kasetto)
- [803/skills-supply](https://github.com/803/skills-supply)
- [GitHub Copilot custom instructions](https://docs.github.com/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot)
- [Path-scoped Copilot instructions changelog](https://github.blog/changelog/2025-09-03-copilot-code-review-path-scoped-custom-instruction-file-support/)
- [skills.sh](https://skills.sh)
- [Agent Skills specification](https://agentskills.io/specification)
- [Model Context Protocol introduction](https://modelcontextprotocol.io/docs/getting-started/intro)
