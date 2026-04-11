# Scribe Agentic Skill + Single-File Skill Support

**Date:** 2026-04-11
**Status:** Design approved, pending implementation plan

## Problem

AI coding agents using scribe today have no standard way to learn the CLI. When a user says "install the react skill" or "what scribe packages do I have installed", the agent has to improvise — guessing at flag names, falling into blocking interactive browsers (`scribe add` with no query hangs), parsing styled TUI output instead of JSON, or worse, hand-editing `~/.claude/skills/`.

We want scribe itself to ship a skill that teaches agents how to drive the CLI efficiently, and we want that skill auto-discoverable on first install — not something users have to hunt down.

Secondary wins: this forces us to fix a latent broken path in scribe's own tree-scan discovery (root-level `SKILL.md` emits `Path: "."` which then causes `Fetch` to pull the entire repo), and it establishes the pattern "drop a SKILL.md at your repo root and scribe picks it up" — a strong dogfooding story.

## Goals

1. Ship a `scribe` skill at the scribe repo root that teaches agents how to use scribe.
2. Make `Naoray/scribe` a default public registry, applied to both first-time and existing users.
3. Fix tree-scan + fetch so root-level `SKILL.md` actually works end-to-end (single-file skill support).
4. No regressions for existing registries (`anthropic/skills`, `openai/codex-skills`, `expo/skills`).

## Non-goals

- Local registry index / catalog cache (see `project_local_registry_index.md` memory — separate spec).
- Refactoring `RegistryTypeTeam`/`RegistryTypeCommunity` into a private/public model (see `feedback_registry_private_public.md` — future debt).
- Single-file skill advanced features (multi-file but single-dir skills, references/ subdirs at root) — out of scope; current scope is "one file, one SKILL.md at repo root".
- Any new command. This spec only touches existing commands' wiring + adds one skill file.

## Design

### 1. Skill payload — SKILL.md at scribe repo root

A single file, `SKILL.md`, at the root of the `Naoray/scribe` repository. No `scribe.yaml`, no `.claude-plugin/marketplace.json`, no dedicated skill subdirectory. One file.

This makes scribe its own marketing: the simplest way to ship a skill is drop one `SKILL.md` at your repo root.

**Frontmatter:**

```yaml
---
name: scribe
description: Use when the user wants to install, list, sync, remove, or manage AI coding-agent skills on this machine. Scribe is the CLI that manages ~/.claude/skills and symlinks skills into Claude Code, Cursor, Codex, and other AI tools.
---
```

**Body structure** (all sections combined, target ~150-220 lines):

1. **What scribe is** — 2-3 sentences on the tool's purpose: canonical store, symlink-based installs into per-tool dirs, registry discovery.

2. **Trigger phrases → command map** — dense lookup table. Maps user utterances to the scribe commands an agent should invoke. Example rows:
   - "install the X skill" → `scribe add X --yes` (or search first with `scribe add X --json`)
   - "install X from Y/Z" → `scribe add Y/Z:X --yes`
   - "what skills do I have" → `scribe list --json`
   - "what skills are available" → `scribe list --remote --json`
   - "remove X" → `scribe remove X`
   - "sync my skills" → `scribe sync --json`
   - "import my existing skills" → `scribe adopt --json`
   - "what does X do" → `scribe explain X`
   - "add Y/Z as a registry" → `scribe registry add Y/Z`
   - "show scribe status" → `scribe status`

3. **Non-negotiable rules** — the block that prevents agent-session disasters:
   - Always pass `--json` when parsing output.
   - Never run `scribe add` without a query or direct target — bare `scribe add` opens an interactive browser that blocks on stdin.
   - Always pass `--yes` for install/remove to skip confirmation prompts.
   - Prefer `owner/repo:skill` direct form when the target is known — deterministic, auto-connects the registry.
   - Don't hand-edit `~/.scribe/state.json`.
   - Don't `cp`/`mv` skills into `~/.claude/skills/` — use `scribe adopt`.
   - Don't use `scribe sync` to install one skill — sync is reconcile-only; use `scribe add`.

4. **JSON output shapes** — one line per command naming the fields the agent should parse. Concrete list to verify during implementation (all commands already support `--json` as of 2026-04-11):
   - `scribe list --json`
   - `scribe list --remote --json`
   - `scribe add <q> --json`
   - `scribe sync --json`
   - `scribe adopt --json`
   - `scribe remove <name> --json`
   - `scribe status --json`
   - `scribe explain <name> --json`

   **Verification subtask during implementation:** capture the actual JSON shape of each by running the command against a test fixture, and paste the resulting keys into the SKILL.md so the skill is accurate at ship time. If shapes differ from what the skill documents, fix the skill, not the command.

5. **Common flows** — 3-5 copy-paste recipes the agent can adapt:
   - Install one skill by name: `scribe add <query> --yes --json`
   - Install from a specific registry: `scribe add owner/repo:skill --yes --json`
   - Check what's installed before modifying: `scribe list --json | jq '.[].name'`
   - Bring a local-edited skill back under management: `scribe adopt --json`
   - Refresh everything from connected registries: `scribe sync --json`

6. **Fallback to `--help`** — one line covering commands not in the hot path (`registry`, `config`, `tools`, `skill edit`, `resolve`, `restore`, `create`, `guide`, `migrate`, `upgrade`). Agent should run `scribe <cmd> --help` for these.

7. **Anti-patterns** — short reinforcement of the rules block.

**Target length:** 150-220 lines of markdown. Lean enough to load every session.

### 2. Single-file skill support

Tree-scan discovery already emits catalog entries for root-level `SKILL.md` files (`internal/provider/treescan.go:30-38`), but the path it records (`"."`) is wrong: `Fetch` then calls `FetchDirectory(..., ".", ...)` and pulls the entire repo into the skill store. Two small code changes fix it.

**Change 1 — `internal/provider/treescan.go`:** root-level branch sets `skillPath = "SKILL.md"` instead of `"."`. The catalog entry now records the actual file path.

**Change 2 — `internal/provider/github.go` Fetch method:** branch on `strings.HasSuffix(entry.Path, ".md")`:

```go
func (p *GitHubProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]File, error) {
    src, err := manifest.ParseSource(entry.Source)
    if err != nil {
        return nil, fmt.Errorf("parse source for %s: %w", entry.Name, err)
    }
    skillPath := entry.Path
    if skillPath == "" {
        skillPath = entry.Name
    }
    if strings.HasSuffix(skillPath, ".md") {
        data, err := p.client.FetchFile(ctx, src.Owner, src.Repo, skillPath, src.Ref)
        if err != nil {
            return nil, err
        }
        return []File{{Path: "SKILL.md", Content: data}}, nil
    }
    ghFiles, err := p.client.FetchDirectory(ctx, src.Owner, src.Repo, skillPath, src.Ref)
    if err != nil {
        return nil, err
    }
    files := make([]File, len(ghFiles))
    for i, f := range ghFiles {
        files[i] = File{Path: f.Path, Content: f.Content}
    }
    return files, nil
}
```

Key invariant: the file is always stored as `SKILL.md` in the on-disk skill directory regardless of the remote filename. This normalizes the layout so the store writer + tool-install adapters don't need to know whether a skill originated as a single file or a directory.

No changes to `internal/manifest/manifest.go` — `Entry.Path` is already a free-form string. Optionally add a doc comment noting "directory or a single `.md` file".

**Downstream impact:** none for existing multi-file skills. The single-file branch is additive.

### 3. Default registry wiring — `Naoray/scribe`

Goal: `Naoray/scribe` is auto-connected on first run AND backfilled for existing users — without re-adding registries a user has explicitly removed.

**Change 1 — `internal/firstrun/firstrun.go:20`:** add `"Naoray/scribe"` at the top of `builtinRepos`:

```go
var builtinRepos = []string{
    "Naoray/scribe",
    "anthropic/skills",
    "openai/codex-skills",
    "expo/skills",
}
```

**Change 2 — `internal/config/config.go`:** add `SeenBuiltins []string` to `Config`:

```go
type Config struct {
    Registries   []RegistryConfig `yaml:"registries,omitempty"`
    Token        string           `yaml:"token,omitempty"`
    Tools        []ToolConfig     `yaml:"tools,omitempty"`
    Editor       string           `yaml:"editor,omitempty"`
    Adoption     AdoptionConfig   `yaml:"adoption,omitempty"`
    SeenBuiltins []string         `yaml:"seen_builtins,omitempty"`
}
```

Tracks which builtins scribe has already offered the user. Empty slice for existing configs = all pre-existing builtins will be marked seen on next run without being re-added (idempotent for their existing registry entries).

**Change 3 — `internal/firstrun/firstrun.go` `ApplyBuiltins`:** change semantics to "apply any un-seen builtin, return the net-new additions". Respects user deletions (seen + absent means the user removed it — do not re-add).

```go
func ApplyBuiltins(cfg *config.Config) (added []string) {
    seen := map[string]bool{}
    for _, s := range cfg.SeenBuiltins {
        seen[s] = true
    }
    for _, builtin := range BuiltinRegistries() {
        if seen[builtin.Repo] {
            continue // user has seen this — respect their decision to remove/disable
        }
        if cfg.FindRegistry(builtin.Repo) == nil {
            cfg.AddRegistry(builtin)
            added = append(added, builtin.Repo)
        }
        cfg.SeenBuiltins = append(cfg.SeenBuiltins, builtin.Repo)
    }
    return added
}
```

Behavior matrix:

| Scenario | Before | After | Net action |
|---|---|---|---|
| First-run user | no config | builtins added, all marked seen | install all 4 |
| Existing user (pre-`Naoray/scribe`) | 3 builtins present, SeenBuiltins empty | all 4 marked seen; only `Naoray/scribe` actually added | install 1 |
| User removed a builtin earlier | registry absent, seen | no change | nothing |
| User disabled a builtin | registry present, `Enabled: false`, seen | no change | nothing |

**Change 4 — `cmd/root.go:44-53`:** move `ApplyBuiltins` call OUT of the `if !firstrun.IsFirstRun()` gate. Run on every invocation (cheap — it's a map diff), but only act when new builtins are added.

```go
cfg, err := factory.Config()
if err != nil {
    return err
}

added := firstrun.ApplyBuiltins(cfg)
if len(added) > 0 {
    if isatty.IsTerminal(os.Stdout.Fd()) {
        if firstrun.IsFirstRun() {
            fmt.Println("Welcome to Scribe! Adding built-in registries...")
        } else {
            fmt.Println("Scribe: new built-in registries available:")
        }
        for _, repo := range added {
            fmt.Printf("  + %s\n", repo)
        }
        fmt.Println()
    }
    if err := cfg.Save(); err != nil {
        return err
    }
}

if firstrun.IsFirstRun() {
    // existing adoption prompt — unchanged
}
```

Note: existing users get a one-shot banner the first time they run scribe after upgrading, listing `Naoray/scribe` as the new builtin. No banner on subsequent runs (SeenBuiltins is now populated). No banner in piped/non-TTY contexts.

## Testing

### New tests

**`internal/provider/treescan_test.go`:**
- Root-level `SKILL.md` emits `Path: "SKILL.md"` (not `"."`).
- Regression guard: assert no catalog entry has `Path == "."`.

**`internal/provider/github_test.go`:**
- `Fetch` with `entry.Path = "SKILL.md"` uses `FetchFile` and returns a single-file slice with `Path: "SKILL.md"`.
- Existing `Fetch` cases with directory paths unchanged.

**`internal/firstrun/firstrun_test.go`:**
- Existing `ApplyBuiltins` tests updated to assert new return value and `SeenBuiltins` population.
- New: user removed a builtin earlier → `ApplyBuiltins` does not re-add. (Seed: empty registries, seen contains the removed repo.)
- New: existing user with 3 pre-existing registries, empty `SeenBuiltins` → `added` returns only `["Naoray/scribe"]`, all 4 marked seen.
- New: user with `Enabled: false` on a builtin → `ApplyBuiltins` leaves enabled flag untouched.

**Integration-ish:**
- End-to-end: seed a fake GitHub client that returns a root `SKILL.md`, call `scribe add Naoray/scribe:scribe` with it, assert `~/.scribe/skills/scribe/SKILL.md` exists and nothing else in that directory.

### Manual verification steps (captured in plan)

1. Fresh `~/.scribe/` → run `scribe status` → expect 4 builtins listed including `Naoray/scribe`.
2. Existing config with only the 3 old builtins → run any scribe command → expect banner announcing `Naoray/scribe` added; re-run → no banner.
3. `scribe registry remove Naoray/scribe` → re-run any scribe command → expect no re-add (respects user deletion).
4. `scribe add Naoray/scribe:scribe --yes --json` → expect success, `scribe list --json` shows the skill, `~/.claude/skills/scribe` symlink exists.

## Migration / backwards compatibility

- Existing `config.yaml` files without `seen_builtins` key load cleanly (omitempty + empty slice).
- After first run against the new binary, `seen_builtins` appears in the persisted config.
- Downgrading scribe after seeing the new key is safe — old scribe versions ignore unknown YAML keys per `yaml.v3` defaults.
- No state migration, no on-disk store migration, no breaking change to existing commands.

## Implementation subtasks (for the plan)

1. Verify exact JSON output shapes of each command listed in the skill body.
2. Write `SKILL.md` at repo root with the finalized content from Section 1.
3. `internal/provider/treescan.go`: fix root-level `skillPath`.
4. `internal/provider/github.go`: add single-file fetch branch.
5. `internal/config/config.go`: add `SeenBuiltins` field.
6. `internal/firstrun/firstrun.go`: rewrite `ApplyBuiltins` signature + semantics.
7. `cmd/root.go`: move `ApplyBuiltins` out of first-run gate, add net-new banner branch.
8. All tests from Testing section.
9. Manual verification walkthrough.
10. Commit + PR (squash merge to main per project convention).

## Open questions resolved during brainstorming

- **Skill location:** SKILL.md at repo root, no scribe.yaml / marketplace.json. Forces fix of latent single-file-skill bug.
- **Default registry mechanism:** builtin list + `SeenBuiltins` diff — backfills existing users, respects deletions.
- **Team/community type:** out of scope; reject the distinction in principle (see `feedback_registry_private_public.md`) but don't refactor in this spec.
- **Local registry index:** out of scope, separate future spec (`project_local_registry_index.md`).
