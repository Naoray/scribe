# Kits and Snippets Design

**Date:** 2026-04-29
**Status:** Proposed
**Author:** brainstorming session

## Problem

Scribe currently projects every installed skill into a single global directory per agent (`~/.claude/skills/`, `~/.codex/skills/`). This causes two related problems:

1. **Codex skill budget overflow.** Issue [#114](https://github.com/Naoray/scribe/issues/114): 153 top-level skills + 211 nested entries vs. Codex's 5440-byte description budget. Codex truncates every skill's description by ~256 chars, degrading skill discovery and routing for the rest of the session. Same pressure exists for Claude Code at higher counts.
2. **One project, many sessions, different needs.** A coding session does not need `plan-my-day`. A planning session does not need `tdd`. Today every session sees every skill the user has ever installed. There is no way to scope what an agent loads to the work at hand.

A side problem also lives in this neighborhood: scribe has no first-class home for **rules / behavior directives** — the things users put in `CLAUDE.md`, `AGENTS.md`, `.cursorrules` to steer voice, conventions, do/don't lists. These are not skills (they add no Skill-tool capability) and they should not be skills.

This spec introduces two new primitives — **kits** and **snippets** — and changes how scribe projects skills onto disk.

## Goals

- Let users scope skill availability to the current project, not the whole machine.
- Let users compose curated skill bundles (`kits`) and stack multiple bundles per project.
- Let users author and share rules (`snippets`) injected into agent rules files.
- Keep Codex within its skill description budget by default, refuse to silently overflow it.
- Preserve parallel-session safety: two sessions running with different kits must not stomp each other.
- Stay opt-in for existing users — no surprise migrations on upgrade.

## Non-Goals

- **Persona primitive.** Research concluded: do not ship as a top-level Scribe primitive. Frontier coding agents in 2026 capture most of the historical multi-agent gains with stronger base models, planning/verification harnesses, repo rules, and on-demand subagent delegation. The forward path for the few cases where role specialization still wins (adversarial review, security/a11y experts, context isolation) is **subagent routing metadata inside kits**, not a new primitive. See the Companion Research section.
- **Kit inheritance / composition.** Kits do not import other kits in v1. Stack at the project level instead.
- **Snippet templating.** No variables, no conditionals, no liquid-style logic in snippet bodies. Plain text.
- **MCP server management.** Out of scope. Layer later if and when a real cross-cutting need appears.
- **Cross-machine sync.** Project files travel with the repo; user-defined kits/snippets do not auto-sync between machines yet.
- **Subagent routing metadata inside kits.** The eventual home for "fork a security-reviewer subagent on PR diffs" lives inside kit definitions, not in a persona object. Out of scope for this PR; sketched in Companion Research as the v2 direction.

## Key Decisions

### 1. Projection moves from global to project-local

Today scribe symlinks `~/.scribe/skills/<name>` → `~/.claude/skills/<name>`. After this change, scribe symlinks `~/.scribe/skills/<name>` → `<project>/.claude/skills/<name>`. The canonical store under `~/.scribe/skills/` is unchanged.

Both Claude Code and Codex already read project-local skill directories — `<cwd>/.claude/skills/` and `<cwd>/.codex/skills/` — without any flag. This is not a new agent feature; it is a switch in where scribe writes.

Consequence: a session's skill set is determined by `cwd`. Two sessions in two different repos see two different skill sets, even if the user only ever ran one `scribe use` command. Parallel sessions in the same repo with different kits are handled by anvil worktrees (already standard practice).

### 2. Kit is a curated, stackable skill bundle

A kit is a named list of skills. Kits live under `~/.scribe/kits/<name>.yaml`. A project lists which kits it wants in `.scribe.yaml`. Kits union; the project may add or remove individual skills on top.

Kits are not personas. Research (see Companion Research) concluded that a separate persona primitive is not justified by 2026's coding-agent landscape. If kits eventually grow optional **subagent routing metadata** ("on PR diffs, fork a security-reviewer with these tools"), that lives inside the kit, not in a sibling primitive.

### 3. Snippet is a rules block injected into agent rules files

A snippet is a Markdown body with frontmatter declaring which agents it targets. Scribe injects snippet bodies into the project's `CLAUDE.md`, `AGENTS.md`, `.cursorrules` (per target) inside scribe-managed marker blocks. User-authored content outside the markers is preserved.

Snippets stack like kits: project lists `snippets:` and scribe writes one block per snippet, in order.

### 4. Project file is `.scribe.yaml`

A new dotfile at the project root, distinct from `scribe.yaml` used inside skill repos as a manifest. Same format (YAML), different role, different name. The dotfile is committed to the repo so the team shares the same skill set.

### 5. Budget guardrail is a real refusal, not a soft warning

Codex's 5440-byte budget is a hard limit. Scribe estimates the resolved skill set's description bytes before projection:

- < 70% → silent
- 70%–100% → warn but proceed
- ≥ 100% → refuse with the list of skills causing the overflow; user retries with `--force` or trims the kit

This makes the Codex bug structurally impossible to reintroduce by accident.

### 6. State tracks per-project projection sets

Today `state.InstalledSkill.Tools` lists which tools a skill is projected onto, machine-globally. We add `Projections []ProjectionEntry` recording which projects currently link this skill. Sync uses this to clean up stale links when a kit changes or a project is deleted.

### 7. Existing global-projection installs are not auto-migrated

Upgrade does not silently delete `~/.claude/skills/` symlinks. A new `scribe migrate global-to-projects` command interactively walks projects the user picks. Until run, scribe operates in a compatibility mode that keeps global projection working with a deprecation banner. Compat mode removed at v1.0.

## User Experience

### Activating a kit in a project

```
$ cd ~/code/my-laravel-app
$ scribe use coding reviewing

  ✓ resolved 2 kits → 18 skills
  ✓ projected to .claude/skills/, .codex/skills/
  ✓ wrote .scribe.yaml
  • Codex budget: 64% (3470 / 5440 bytes)

  Active kits in this project: coding, reviewing
```

### Inspecting current state

```
$ scribe kit list

  Active in this project (.scribe.yaml):
    coding       12 skills
    reviewing     6 skills

  All defined kits:
    coding       Skills for code authoring, debugging, review (12)
    reviewing    PR/code review workflow                       (6)
    planning     Planning, scheduling, weekly cadence         (8)
    writing      Long-form writing and editing                 (5)
```

### Creating a kit

```
$ scribe kit create laravel
  Pick skills (space to toggle, enter to confirm):
    [x] tdd
    [x] debugging
    [ ] init-laravel
    ...
  ✓ wrote ~/.scribe/kits/laravel.yaml
```

### Sharing a kit

Kits live under `~/.scribe/kits/`. Push to a registry the same way skills are pushed — `scribe kit push <name>` adds it to the connected team registry's `kits/` directory. `scribe kit install <ref>` pulls one from a registry.

### Activating a snippet

```
$ scribe snippet use laravel-conventions terse-output
  ✓ injected into CLAUDE.md, AGENTS.md, .cursorrules
  ✓ wrote .scribe.yaml
```

The target files now contain:

```markdown
<!-- scribe:start name=laravel-conventions hash=ab12cd34 -->
- Use Pest for tests, not PHPUnit
- Avoid Eloquent relationships in this codebase
- ...
<!-- scribe:end name=laravel-conventions -->

<!-- scribe:start name=terse-output hash=ef56gh78 -->
Respond terse. Drop articles, filler, hedging. Fragments OK.
<!-- scribe:end name=terse-output -->
```

User content above, between, and below the managed blocks is preserved on every resync.

### Project file shape

```yaml
# .scribe.yaml at project root
kits:
  - coding
  - reviewing

snippets:
  - laravel-conventions
  - terse-output

# optional fine-tuning on top of kits
add:
  - some-extra-skill
remove:
  - skill-i-dont-want-from-coding-kit
```

Resolution order: union of all kits → apply `add` → apply `remove` → final skill set.

### Budget refusal

```
$ scribe use coding writing planning generalist

  ✗ Codex skill budget exceeded
    estimated: 6210 / 5440 bytes (114%)
    overflow caused by:
      writing      adds 920 bytes (5 skills)
      generalist   adds 1340 bytes (8 skills)

  Try removing one kit, or run `scribe use ... --force` to project anyway.
```

## Data Model

### Kit definition (`~/.scribe/kits/<name>.yaml`)

```yaml
name: coding
description: Skills for code authoring, debugging, review
skills:
  - tdd
  - debugging
  - code-review
  - investigate
  - run-tests
  - "init-*"          # glob across registered skills
  - "audit-*"
source:
  registry: Naoray/scribe-kits   # optional, for kits installed from registries
  rev: 7
```

Globs match against installed skill names, evaluated at resolution time so adding a new `init-foo` skill auto-joins the kit.

### Snippet definition (`~/.scribe/snippets/<name>.md`)

```markdown
---
name: laravel-conventions
description: Laravel project conventions
targets: [claude, codex, cursor]   # or "all"
source:
  registry: Naoray/scribe-snippets
  rev: 3
---

- Use Pest for tests, not PHPUnit
- Avoid Eloquent relationships in this codebase
- Prefer single-action invokable controllers
```

`targets` controls which rules files receive the block. `all` is shorthand for every detected agent.

### Project file (`<project>/.scribe.yaml`)

```yaml
kits: [coding, reviewing]
snippets: [laravel-conventions, terse-output]
add: []
remove: []
```

All keys optional. Empty file = no scribe activity in this project.

### State additions (`~/.scribe/state.json`)

```json
{
  "installed": {
    "tdd": {
      "kind": "skill",
      "canonical_dir": "/Users/.../.scribe/skills/tdd",
      "projections": [
        { "project": "/Users/.../my-laravel-app", "tools": ["claude", "codex"] },
        { "project": "/Users/.../scribe",         "tools": ["claude"] }
      ]
    }
  },
  "kits": {
    "coding": { "source": "local", "skills": [...] }
  },
  "snippets": {
    "laravel-conventions": { "source": "local", "targets": [...] }
  }
}
```

`projections` replaces the existing single `paths` + `tools` pair. Migration converts old shape on first read.

## CLI Surface

### Top-level shortcuts

```
scribe use <kit> [<kit> ...]          # activate kit(s) in cwd
scribe drop <kit>                     # remove kit from cwd
scribe show                           # print resolved skill set + budget for cwd
```

### Kit subcommand

```
scribe kit list                       # active in cwd, then all defined
scribe kit list --all                 # everything, including remote-installable
scribe kit show <name>                # inspect kit contents
scribe kit create <name>              # interactive picker
scribe kit edit <name>                # edit existing kit
scribe kit remove <name>              # delete kit definition (refuses if any project uses it)
scribe kit install <ref>              # pull from registry
scribe kit push <name>                # push to connected registry
```

### Snippet subcommand

```
scribe snippet list
scribe snippet show <name>
scribe snippet create <name>
scribe snippet edit <name>
scribe snippet remove <name>
scribe snippet use <name> [<name> ...]
scribe snippet drop <name>
scribe snippet install <ref>
scribe snippet push <name>
```

### Migration command

```
scribe migrate global-to-projects
  > Found 153 globally projected skills.
  > Pick projects that should keep these (others will be dropped):
    [x] ~/code/my-laravel-app
    [x] ~/code/scribe
    [ ] ~/code/personal-vault
    ...
  ✓ wrote .scribe.yaml in 2 projects
  ✓ removed global symlinks in ~/.claude/skills/, ~/.codex/skills/
```

`scribe sync` continues to work; it now resolves per-project state instead of one global set.

## Implementation Notes

### Resolution algorithm

```go
func ResolveProject(projectDir string) (ResolvedSet, error) {
    cfg := readScribeYAML(projectDir)
    skills := map[string]bool{}

    for _, kitName := range cfg.Kits {
        kit := loadKit(kitName)
        for _, glob := range kit.Skills {
            for _, name := range matchInstalledSkills(glob) {
                skills[name] = true
            }
        }
    }
    for _, name := range cfg.Add {
        skills[name] = true
    }
    for _, name := range cfg.Remove {
        delete(skills, name)
    }

    return ResolvedSet{
        Skills: keys(skills),
        Snippets: cfg.Snippets,
        Budget: estimateBudget(skills),
    }, nil
}
```

### Projection mechanics

For each skill in the resolved set and each enabled tool with `Detect()=true`:
- Symlink `~/.scribe/skills/<name>` → `<project>/.<tool>/skills/<name>`
- Update state's `projections` entry

For each skill currently projected to `<project>` but no longer in the resolved set:
- Remove the symlink
- Update state

This is the same projection code used today, parameterized over the target dir.

### Snippet injection mechanics

For each snippet in the resolved set and each declared target:
- Determine target file (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`)
- Locate or create the scribe-managed block by markers
- Replace block content if `hash` differs, otherwise no-op
- Append a new block at the file's end if absent

Idempotent. Re-running `scribe sync` produces zero-diff churn when nothing changed.

### Budget estimation

Sum bytes of `frontmatter.description + "\n\n" + first-paragraph-of-body` per resolved skill. Compare to per-agent budget table (`codex: 5440`, `claude: TBD via probing`). Surface results in `scribe show` and any kit-changing command.

### Compatibility mode (pre-1.0)

If state shows global projections AND no `.scribe.yaml` exists in the cwd, scribe falls back to global-projection behavior with a once-per-day deprecation banner. After v1.0 release, compat mode is removed and orphaned global symlinks are recommended for removal via `scribe migrate global-to-projects`.

## Alternatives Considered

1. **Global projection + filtered runtime view.** Keep symlinking everything globally; have the agent filter what it loads. Rejected — Codex loads what's on disk, no runtime filter exists, and we cannot ship code into agents.
2. **Single active "mode" or "profile" per machine.** Switching mode in session B yanks skills out from under session A. Rejected — global mutable state breaks parallel sessions.
3. **Tags on skills + filtered sync.** `scribe sync --tags coding`. Composable but most existing skills lack tags; required upfront tagging effort across the registry. Kit gives explicit curation now and tags can layer later inside kit definitions.
4. **Personas as the only primitive.** Bundle skills + rules + role under one "persona" object. Rejected for v1 — locks naming on a concept whose value is unclear (research pending). Building kit and snippet first leaves room for persona to compose them later.
5. **Snippets-as-skills.** Treat rules as a special skill kind. Rejected — they have no `SKILL.md` semantics, no Skill-tool invocation. Forcing them into the skill abstraction adds confusion for both authors and agents.

## Open Questions

- **Snippet target detection**: how should scribe choose which rules file to inject for a generic `targets: all` snippet — only files that already exist, or always create them? Proposal: only existing files unless user passes `--create-targets`.
- **Kit-of-kits**: should kits be allowed to list other kits, even in v1? Risk of recursion + ordering bugs. Proposal: defer; project-level stacking covers this.
- **Snippet conflict policy**: two snippets that contradict (e.g. "use Pest" vs. "use PHPUnit"). Proposal: scribe does not police content; render both blocks in declared order.
- **Per-tool kit override**: project may want kit `coding` for Claude but `coding-light` for Codex. Defer to v2; current shape covers >90% case.
- **Where does scribe-agent itself live**: it currently auto-projects to global. After this change, scribe-agent should still be globally available (it is the bootstrap). Special-case: scribe-agent always projects globally, all other skills go project-local.

## Scope

In scope for this PR:

- Kit primitive: data model, storage, resolution, CLI subcommand
- Snippet primitive: data model, storage, injection mechanism, CLI subcommand
- Project file `.scribe.yaml` parsing
- Projection move: per-project skill linking
- Top-level `scribe use`, `scribe drop`, `scribe show`
- Budget guardrail (Codex first; Claude via probing later)
- State schema change with migration on read
- Migration command `scribe migrate global-to-projects`
- Compatibility mode for legacy global projection
- Tests for resolution, projection, injection, budget

Out of scope:

- Persona primitive (research pending — see prompt below)
- Kit-of-kits / inheritance
- Snippet templating
- MCP server management
- Per-tool kit override
- Cross-machine sync of user-defined kits/snippets

## Companion Research

A research pass evaluated whether a third primitive — **persona**, framed as multi-agent role specialization in the MetaGPT / ChatDev / self-collaboration tradition — earns a place beyond kits + snippets in 2026's coding-agent landscape.

**Verdict: no.** Do not ship persona as a top-level Scribe primitive.

### Why

- **Frontier single agents have closed the old role-pipeline gap.** Modern repo-level systems cluster in the 70–75% SWE-bench band regardless of architecture; mini-SWE-agent + Gemini 3 Pro Preview hits 74.2% as a single agent, comparable to multi-agent systems like Agyn at 72.2%. Architecture is now less predictive than model quality, token budget, search, and verification.
- **Post-MetaGPT literature drifts away from fixed SDLC personas** toward planner-worker-search-evaluator structures. SWE-Search and EvoMAC show that "specialization" still matters, but as search/verification structure, not standalone persona abstraction.
- **Frontier-lab guidance is consistent**: single agent first, multi-agent only when justified. OpenAI says maximize a single agent before adding more. Anthropic flags MAS as "powerful but over-applied" and notes most coding tasks are less parallelizable than research; reliable multi-agent wins are limited to context isolation, parallel execution, and specialization. Google's everyday Gemini Code Assist agent is single-agent with tools.
- **Production tools mostly avoid persona as a distinct primitive.** Claude Code, Codex, Cursor, Aider, Continue, Cline center on rules + skills + optional subagents. The clearest first-class persona feature — Roo Code's custom modes — is sunsetting on May 15, 2026. Devin moved from persona cards to playbooks + child sessions + managed Devins (dynamic orchestration, not static personas).

### Where role specialization still wins

Real edge cases exist: parallel and read-heavy work, adversarial review (judge/monitor, red/blue), domain-locked experts (security, accessibility), and strict context isolation. Anthropic's docs use a `security-reviewer` subagent as a canonical example; monitor-augmented multi-agent code generation improved LiveCodeBench in 2025.

These wins do not need a new primitive. They need a way for kits to declare "spawn this subagent on this trigger".

### Forward path: subagent routing metadata inside kits (deferred)

Future kit YAML may carry optional delegation rules:

```yaml
name: reviewing
skills: [code-review, run-tests, ...]
subagents:
  - name: security-reviewer
    trigger: pr-diff-touches /auth/, /sessions/, /crypto/
    model: claude-opus-4-7
    tools: [grep, read, web-search]
    isolation: child-session
  - name: a11y-reviewer
    trigger: pr-diff-touches *.tsx, *.jsx
    model: claude-sonnet-4-6
    tools: [grep, read]
```

This composes with v1: a kit is still a skill bundle by default; routing metadata is opt-in. No new primitive, no new file.

The signal that would prove the verdict wrong and force a real persona primitive: a repeatable result — same model, same tools, same compute budget — where explicit role-specialized orchestration beats single-agent + planning + ad-hoc subagents by a meaningful margin on **repo-level** benchmarks (not HumanEval-style competitive code). Current public evidence is too mixed and too confounded by model/compute differences to justify it today.

### Sources

- [PersonaLLM survey, software-development section](https://github.com/MiuLab/PersonaLLM-Survey#software-development) — arXiv [2406.01171](https://arxiv.org/abs/2406.01171)
- SWE-Search, EvoMAC, SWE-Dev (post-MetaGPT MAS literature)
- Agyn / mini-SWE-agent SWE-bench results
- Anthropic, OpenAI, Google DeepMind agent-design guidance (2025–2026)
- Roo Code custom modes deprecation (sunsets 2026-05-15)
