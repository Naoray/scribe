# Scribe vs alternatives

Scribe is for teams that want a reproducible, cross-agent way to publish, install, scope, and update coding-agent skills from registries. It is not a replacement for agent-specific workflow systems, IDE agents, or MCP servers: those are often better when you want a richer coding methodology, editor automation, or runtime access to external tools and data.

## Feature comparison

| Feature | scribe | skills.sh / Agent Skills | superpowers | anthropics/skills | Cursor MDC | Cline / Roo | MCP servers |
|---|---|---|---|---|---|---|---|
| Primary job | ✓ skill manager | ✓ open skill format and directory | ✓ coding methodology and plugin | ✓ Claude skill examples and reference | ✓ scoped Cursor instructions | ✓ IDE workflows, skills, and commands | ✓ runtime tools, data, and workflows |
| Reproducibility | ✓ `scribe.lock`, manifests, sync | partial spec validation, no lockfile | partial plugin/extension installs, no skill lockfile | partial git repo / plugin install | partial versioned if committed | partial project files if committed | N/A protocol integration |
| Cross-tool support | ✓ Claude Code, Codex, Cursor, Gemini/custom tools | ✓ broad agent ecosystem | ✓ many coding-agent/plugin surfaces | partial Claude surfaces | ✗ Cursor only | partial Cline/Roo and `.agents` paths | ✓ broad clients and servers |
| Discovery model | ✓ connected registries plus browse/list | ✓ public directory and leaderboard | partial plugin marketplaces and repo | partial curated example repo | partial local rule list | partial local menus/marketplaces | ✓ registries and server catalogs |
| Update flow | ✓ explicit `sync`, `status`, `doctor`, schemas | partial CLI install/update behavior | partial agent-dependent, often automatic | partial plugin/repo updates | ✗ manual/project workflow | partial extension and local-file flows | partial client/server dependent |
| Author publishing | ✓ `registry create/add`, `init`, `push` flows | partial publish by repository/directory listing | partial PR/plugin contribution | partial PR-based examples/templates | ✗ commit files in one repo | partial commit local project files | N/A build and publish servers |
| Per-project state | ✓ `.scribe.yaml`, kits, snippets, projections | ✗ format is per skill, not project state | partial methodology can use worktrees | ✗ repository of skills, not project state | ✓ `.cursor/rules` and nested rules | ✓ project skills/workflows/commands | N/A client configuration |
| Budget enforcement | ✓ Codex skill-description guardrail | ✗ spec gives description limits, not session budgets | ✗ not documented | ✗ not documented | ✗ not documented | ✗ not documented | N/A |
| Existing skill adoption | ✓ adopt unmanaged local skills | partial compatible format | partial consumes skills/plugins | partial examples/template | N/A | partial can read local skill dirs | N/A |
| Frontmatter-only skill format | ✓ uses `SKILL.md` frontmatter | ✓ spec is `SKILL.md` frontmatter | partial skills plus plugin instructions | ✓ `SKILL.md` frontmatter | partial MDC frontmatter, not skills | ✓ `SKILL.md` for skills | N/A |
| Command/schema introspection | ✓ JSON envelopes and `scribe schema` | ✗ not part of spec | ✗ not documented | ✗ not documented | ✗ not documented | partial command UIs/CLI, no shared schema | ✓ tool schemas in protocol |
| Best at | ✓ skill lifecycle and team sync | ✓ portable skill authoring format | ✓ disciplined development workflow | ✓ Claude-native examples | ✓ Cursor prompt policy | ✓ IDE automation and task workflows | ✓ connecting agents to external systems |

## When to pick scribe

- You maintain team skills in Git and want one manifest-driven install/update flow across multiple agent tools.
- You need reproducible sync, lockfiles, repair commands, and machine-readable output for agent automation.
- You want project-scoped skill projection so one repository does not inherit every skill installed on the machine.
- You already have hand-written skills and want to adopt them without moving or rewriting them first.

## When to pick an alternative

- Pick **skills.sh / Agent Skills** when you mainly need the open `SKILL.md` format, public discovery, or compatibility with tools that already consume Agent Skills directly.
- Pick **superpowers** when you want a complete agentic software-development methodology with planning, TDD, review, worktree, and subagent workflows.
- Pick **anthropics/skills** when you want Claude-native examples, templates, or document skills that demonstrate Anthropic's skill patterns.
- Pick **Cursor MDC** when your only target is Cursor and you want lightweight, version-controlled rules tied to file globs or manual `@ruleName` attachment.
- Pick **Cline or Roo Code** when you want a VS Code agent with editor automation, slash-command workflows, skills, checkpoints, modes, browser tools, or human-in-the-loop command approval.
- Pick **MCP servers** when the problem is tool/data access at runtime, such as calling GitHub, reading a database, or connecting an agent to an internal service.

## References

- [scribe README](../README.md)
- [scribe commands reference](commands.md)
- [scribe projects, kits, and snippets](projects-and-kits.md)
- [skills.sh](https://skills.sh)
- [Agent Skills specification](https://agentskills.io/specification)
- [obra/superpowers README](https://github.com/obra/superpowers)
- [anthropics/skills README](https://github.com/anthropics/skills)
- [Cursor rules documentation](https://docs.cursor.com/en/context)
- [Cline workflows documentation](https://docs.cline.bot/customization/workflows)
- [Cline skills documentation](https://docs.cline.bot/customization/skills)
- [Roo Code skills documentation](https://docs.roocode.com/features/skills)
- [Roo Code slash commands documentation](https://docs.roocode.com/features/slash-commands)
- [Model Context Protocol introduction](https://modelcontextprotocol.io/docs/getting-started/intro)
