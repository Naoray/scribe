# Scribe vs alternatives

Scribe is for teams that want a reproducible, cross-agent way to publish, install, scope, and update coding-agent skills from registries. The closest neighbours are **GitHub Copilot custom instructions** and the open **Agent Skills** format — both ship instructions to coding agents, but with different goals. Scribe is _not_ a replacement for an IDE agent, a coding methodology, or MCP servers — those solve different problems (see [Different layer](#different-layer-not-an-alternative)).

## Feature comparison

| Feature | Scribe | GitHub Copilot custom instructions | skills.sh / Agent Skills |
|---|---|---|---|
| Primary job | Skill manager: publish, install, project, sync | Repo-scoped instructions for Copilot Chat, code review, agent | Open `SKILL.md` format and public directory |
| Where instructions live | `~/.scribe/` (canonical), projected to `.claude/`, `.cursor/`, `.codex/`, `.gemini/` | `.github/copilot-instructions.md`, `.github/instructions/*.instructions.md`, `AGENTS.md` | Per-skill `SKILL.md` (frontmatter) anywhere a tool can read |
| Cross-tool support | Claude Code, Codex, Cursor, Gemini, custom tools | Copilot (Chat, code review, agent, cloud agent). `AGENTS.md` is also read by Claude (`CLAUDE.md`) and Gemini (`GEMINI.md`). | Any agent that consumes the spec |
| Path/file scoping | `.scribe.yaml` per project, kits, snippets | `applyTo: "src/**/*.ts"` glob frontmatter on `*.instructions.md` | Not part of spec |
| Reproducibility | `scribe.lock` lockfile, `scribe sync`, `scribe doctor` | None — markdown files in git, no lockfile or sync command | None — repo or directory listing |
| Discovery | Connected registries (Naoray, anthropics, custom) | None — author your own, or borrow from `awesome-copilot` | Public directory and leaderboard |
| Existing skill adoption | `scribe adopt` claims unmanaged local skills via symlink | N/A — instructions are author-only | Compatible `SKILL.md` format (cross-import) |
| Per-project state | `.scribe.yaml`, `.ai/scribe.lock`, projection map | Repo-scoped instructions in `.github/`, but no manifest or lockfile | Per-skill, not project state |
| Budget enforcement | Codex skill-description guardrail in `scribe doctor` | Not documented | Spec gives description limits, not session budgets |
| Schema / introspection | JSON envelopes, `scribe schema <command>`, exit codes | Not applicable — natural-language markdown | Not part of spec |
| Best at | Multi-agent skill lifecycle and team sync | Tightly-scoped repo instructions for Copilot users | Portable skill authoring format |

## When to pick Scribe

- You maintain team skills in Git and want one manifest-driven install/update flow across multiple agent tools.
- You need reproducible sync, lockfiles, repair commands, and machine-readable output for agent automation.
- You want project-scoped skill projection so one repository does not inherit every skill installed on the machine.
- You already have hand-written skills and want to adopt them without moving or rewriting them first.

## When to pick an alternative

- Pick **GitHub Copilot custom instructions** when your team is all-in on Copilot and a path-scoped `*.instructions.md` plus a repo `copilot-instructions.md` cover what you need. Lighter than Scribe, but no lockfile, no project state, no cross-tool projection.
- Pick **skills.sh / Agent Skills** when you mainly need the open `SKILL.md` format, public discovery, or compatibility with tools that already consume Agent Skills directly. (Scribe consumes the same `SKILL.md` format — you can use both.)

## Different layer, not an alternative

- **MCP servers** solve runtime tool/data access (calling GitHub, reading a database, connecting to internal services). Scribe and Copilot instructions ship _prompts and skills_; MCP ships _capabilities_. Most teams want both.
- **IDE agents** (Cline, Roo Code, Cursor agent mode, Claude Code) are the runtime that _executes_ skills. Scribe is the source-of-truth and delivery system; the agent is the consumer.
- **Coding methodologies** (TDD-first, agentic plan/build/review loops) are _how_ an agent works. Scribe is _what_ the agent reads.

## References

- [scribe README](../README.md)
- [scribe commands reference](commands.md)
- [scribe projects, kits, and snippets](projects-and-kits.md)
- [GitHub Copilot custom instructions](https://docs.github.com/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot)
- [Path-scoped Copilot instructions changelog](https://github.blog/changelog/2025-09-03-copilot-code-review-path-scoped-custom-instruction-file-support/)
- [skills.sh](https://skills.sh)
- [Agent Skills specification](https://agentskills.io/specification)
- [Model Context Protocol introduction](https://modelcontextprotocol.io/docs/getting-started/intro)
