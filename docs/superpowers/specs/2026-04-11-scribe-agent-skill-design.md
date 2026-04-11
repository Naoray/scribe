# Scribe Agentic Skill + Single-File Skill Support

**Date:** 2026-04-11
**Status:** Design approved, pending implementation plan

## Problem

AI coding agents using scribe today have no standard way to learn the CLI. When a user says "install the react skill" or "what scribe packages do I have installed", the agent has to improvise — guessing at flag names, falling into blocking interactive browsers (`scribe add` with no query hangs), parsing styled TUI output instead of JSON, or worse, hand-editing `~/.claude/skills/`.

We want scribe itself to ship a skill that teaches agents how to drive the CLI efficiently, and we want that skill auto-discoverable on first install — not something users have to hunt down.

Secondary wins: this forces us to fix a latent broken path in scribe's own tree-scan discovery (root-level `SKILL.md` emits `Path: "."`, which then makes `FetchDirectory` compute `prefix = "./"`, match zero tree entries, and return `no files found under "."` — the catalog entry is effectively dead on arrival), and it establishes the pattern "drop a SKILL.md at your repo root and scribe picks it up" — a strong dogfooding story.

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
name: scribe-agent
description: Use when the user wants to install, list, sync, remove, or manage AI coding-agent skills on this machine. Scribe is the CLI that manages ~/.claude/skills and symlinks skills into Claude Code, Cursor, Codex, and other AI tools.
---
```

**Why `scribe-agent` and not `scribe`:** avoids name collision with any other registry that also happens to ship a skill named `scribe` (common name, likely to clash eventually). State.json keys skills by name; two skills named `scribe` from different registries would clobber each other. The bare-root-SKILL.md story is preserved — the file still lives at repo root — but the frontmatter `name:` field gives the skill a unique identity. Install reads `scribe add Naoray/scribe:scribe-agent --yes`, which is also easier on the eyes than the double-scribe.

**Tree-scan consequence:** tree-scan currently derives the skill name from the repo name for root-level SKILL.md (`name = repo` at `internal/provider/treescan.go:32`). The frontmatter `name:` field takes precedence downstream — discovery still works because the add command looks skills up by the frontmatter name once SKILL.md is fetched. Verify during implementation that `scribe add Naoray/scribe:scribe-agent` resolves correctly end-to-end; if not, tree-scan needs a preflight to read the frontmatter and use `name:` instead of the repo name.

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

Tree-scan discovery already emits catalog entries for root-level `SKILL.md` files (`internal/provider/treescan.go:30-38`), but the path it records (`"."`) is effectively dead on arrival: `Fetch` → `FetchDirectory(..., ".", ...)` → `prefix = "./"` → zero tree-entry matches → error `no files found under "."`. Three code changes fix it end-to-end: tree-scan emits a valid path, Fetch supports single-file retrieval, and sync's blob-SHA resolver recognizes single-file skills.

**Change 1 — `internal/provider/treescan.go:30-38`:** root-level branch sets `skillPath = "SKILL.md"` instead of `"."`. The catalog entry now records the actual file path.

**Change 2 — `internal/provider/github.go` Fetch method:** branch on `path.Base(skillPath) == "SKILL.md"` (exact match, not suffix). This avoids a directory literally named `foo.md` accidentally hitting the single-file branch, and it's self-documenting — the condition says "this is a single-file skill" rather than "this happens to end in .md".

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
    if path.Base(skillPath) == "SKILL.md" {
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

**Change 3 — `internal/sync/blobsha.go:19-35` `resolveSkillBlobSHA`:** current implementation assumes `skillPath` is a directory and builds `target = skillPath + "/SKILL.md"`. For a single-file skill with `entry.Path = "SKILL.md"` this produces `target = "SKILL.md/SKILL.md"` → never matches the tree entry → `resolveSkillBlobSHA` returns `("", false)` → sync misclassifies the skill as missing-upstream on every run. This is a silent regression for anyone installing a single-file skill and then running `scribe sync`. Fix:

```go
func resolveSkillBlobSHA(tree []provider.TreeEntry, entry manifest.Entry) (string, bool) {
    skillPath := entry.Path
    if skillPath == "" {
        skillPath = entry.Name
    }
    skillPath = strings.TrimSuffix(skillPath, "/")
    target := "SKILL.md"
    switch {
    case path.Base(skillPath) == "SKILL.md":
        target = skillPath              // entry already points at the file
    case skillPath != "" && skillPath != ".":
        target = skillPath + "/SKILL.md" // legacy directory-based entry
    }
    for _, e := range tree {
        if e.Type == "blob" && e.Path == target {
            return e.SHA, true
        }
    }
    return "", false
}
```

No changes to `internal/manifest/manifest.go` — `Entry.Path` is already a free-form string. Optionally add a doc comment noting "directory or a single `.md` file".

**Downstream impact:** none for existing multi-file skills. Both the Fetch branch and the `resolveSkillBlobSHA` switch are additive — only single-file skills (where `path.Base(skillPath) == "SKILL.md"`) take the new code path.

### 3. Default registry wiring — `Naoray/scribe`

Goal: `Naoray/scribe` is auto-connected on first run AND backfilled for existing users — without re-adding registries users have disabled.

**Design note on "respecting deletions":** the earlier design tried to track per-repo `SeenBuiltins []string` so a user who removes a builtin via `scribe registry remove` wouldn't see it re-added on upgrade. But `scribe registry remove` doesn't exist today — only `registry add` and `registry enable/disable`. The untestable case was dead code. The disable path is already respected because `ApplyBuiltins` calls `FindRegistry` before adding, and disabled registries are still present in the config. Dropping per-repo tracking in favor of a version-gated marker.

**Change 1 — `internal/firstrun/firstrun.go`:** add `"Naoray/scribe"` at the top of `builtinRepos` and introduce a version constant:

```go
// currentBuiltinsVersion bumps whenever builtinRepos changes. Used by ApplyBuiltins
// to detect that a config was written against an older scribe version and may be
// missing new builtins.
const currentBuiltinsVersion = 2

var builtinRepos = []string{
    "Naoray/scribe",
    "anthropic/skills",
    "openai/codex-skills",
    "expo/skills",
}
```

`currentBuiltinsVersion = 1` represents the pre-existing set (`anthropic/skills`, `openai/codex-skills`, `expo/skills`). `= 2` is this change, which adds `Naoray/scribe`. Future additions bump further.

**Change 2 — `internal/config/config.go`:** add `BuiltinsVersion int` to `Config`:

```go
type Config struct {
    Registries      []RegistryConfig `yaml:"registries,omitempty"`
    Token           string           `yaml:"token,omitempty"`
    Tools           []ToolConfig     `yaml:"tools,omitempty"`
    Editor          string           `yaml:"editor,omitempty"`
    Adoption        AdoptionConfig   `yaml:"adoption,omitempty"`
    BuiltinsVersion int              `yaml:"builtins_version,omitempty"`
}
```

Zero value (`0`) represents "no version marker yet" — either a brand-new config or a config from scribe versions before this change. Both cases trigger backfill.

**Change 3 — `internal/firstrun/firstrun.go` `ApplyBuiltins`:** version-gated. Runs the diff only when the config is behind. Returns net-new additions for UX output.

```go
func ApplyBuiltins(cfg *config.Config) (added []string) {
    if cfg.BuiltinsVersion >= currentBuiltinsVersion {
        return nil
    }
    for _, builtin := range BuiltinRegistries() {
        if cfg.FindRegistry(builtin.Repo) == nil {
            cfg.AddRegistry(builtin)
            added = append(added, builtin.Repo)
        }
    }
    cfg.BuiltinsVersion = currentBuiltinsVersion
    return added
}
```

Behavior matrix:

| Scenario | Before | After | Net action |
|---|---|---|---|
| First-run user | no config (version 0) | 4 builtins added, version = 2 | install 4 |
| Existing user (pre-`Naoray/scribe`) | 3 builtins present, version 0 | `Naoray/scribe` added, version = 2 | install 1 |
| User disabled a builtin | `Enabled: false`, version 0 | disabled flag untouched, only missing repo(s) added, version = 2 | possibly install 1 |
| Already up to date | version = 2 | no-op | nothing |
| Future upgrade (v3) | version = 2, new repo in list | diff adds new repo, version = 3 | install new |

No per-repo "seen" tracking, no "user removed" case — that's a future problem for when `scribe registry remove` actually exists.

**Change 4 — `cmd/root.go:44-53`:** move `ApplyBuiltins` call OUT of the `if !firstrun.IsFirstRun()` gate. Print the banner to **stderr** so it does not corrupt `--json` output on stdout (agents run in PTYs where `isatty(stdout)` returns true; the `os.Stdout` vs `os.Stderr` distinction is what keeps machine-readable output clean, not the TTY check).

```go
cfg, err := factory.Config()
if err != nil {
    return err
}

added := firstrun.ApplyBuiltins(cfg)
if len(added) > 0 {
    if isatty.IsTerminal(os.Stderr.Fd()) {
        if firstrun.IsFirstRun() {
            fmt.Fprintln(os.Stderr, "Welcome to Scribe! Adding built-in registries...")
        } else {
            fmt.Fprintln(os.Stderr, "Scribe: new built-in registries available:")
        }
        for _, repo := range added {
            fmt.Fprintf(os.Stderr, "  + %s\n", repo)
        }
        fmt.Fprintln(os.Stderr)
    }
    if err := cfg.Save(); err != nil {
        return err
    }
}

if firstrun.IsFirstRun() {
    // existing adoption prompt — unchanged
}
```

Key invariants: banner lives on stderr, stdout is reserved for command output (including `--json`). TTY check on stderr is purely to suppress noise in piped stderr redirections (rare but harmless). `cfg.Save()` is called only when `added` is non-empty, which means steady-state scribe invocations after the upgrade do not incur a write.

**Implicit contract note:** after this change, `PersistentPreRunE` may leave `cfg` dirty on disk before `RunE` starts. Any command that also mutates and saves `cfg` later must be aware that its pre-save baseline might differ from what it read originally. Today this is not a conflict because `ApplyBuiltins` only touches `Registries` and `BuiltinsVersion`, both of which are append-only in this flow. Document with a one-line code comment at the call site.

## Testing

### New tests

**`internal/provider/treescan_test.go`:**
- Root-level `SKILL.md` emits `Path: "SKILL.md"` (not `"."`).
- Regression guard: assert no catalog entry has `Path == "."`.
- Root-level SKILL.md at a non-`scribe` repo still derives the tree-scan default name from the repo (e.g. `acme/my-thing` → entry named `my-thing`); the frontmatter `name:` override happens downstream during install, not in tree-scan.

**`internal/provider/github_test.go`:**
- `Fetch` with `entry.Path = "SKILL.md"` uses `FetchFile` (not `FetchDirectory`) and returns a single-file slice with `Path: "SKILL.md"`.
- `Fetch` with `entry.Path = "foo.md"` (hypothetical directory literally named `foo.md`) does NOT hit the single-file branch — verifies `path.Base == "SKILL.md"` is the discriminator, not `HasSuffix(".md")`.
- Existing `Fetch` cases with directory paths unchanged.

**`internal/sync/blobsha_test.go`:**
- New test: `resolveSkillBlobSHA` with `entry.Path = "SKILL.md"` returns the root-level blob SHA (target path = `"SKILL.md"`, not `"SKILL.md/SKILL.md"`).
- Existing directory-path cases unchanged.

**`internal/firstrun/firstrun_test.go`:**
- Existing `ApplyBuiltins` tests updated to assert new return value and `BuiltinsVersion` advancement.
- New: `BuiltinsVersion` already at current → `ApplyBuiltins` returns nil and does not mutate config.
- New: existing user with 3 pre-existing registries, `BuiltinsVersion = 0` → `added` returns `["Naoray/scribe"]`, config ends with `BuiltinsVersion = currentBuiltinsVersion`.
- New: user with `Enabled: false` on a builtin → `ApplyBuiltins` does not flip the flag back to enabled.

**`cmd/root_test.go` (or equivalent):**
- New: banner output writes to stderr, not stdout. Run the command with stdout captured as JSON, assert stdout is clean JSON even when builtins backfill fires. (Black-box via `cmd.SetOut` / `cmd.SetErr`.)

**Integration-ish:**
- End-to-end: seed a fake GitHub client that returns a root `SKILL.md` with frontmatter `name: scribe-agent`, call `scribe add Naoray/scribe:scribe-agent` with it, assert `~/.scribe/skills/scribe-agent/SKILL.md` exists and nothing else in that directory.

### Manual verification steps (captured in plan)

1. Fresh `~/.scribe/` → run `scribe status` → expect 4 builtins listed including `Naoray/scribe`.
2. Existing config with only the 3 old builtins → run any scribe command → expect stderr banner announcing `Naoray/scribe` added; re-run → no banner.
3. `scribe add Naoray/scribe:scribe-agent --yes --json` → expect clean JSON on stdout, banner on stderr, `scribe list --json` shows the skill, `~/.claude/skills/scribe-agent` symlink exists.
4. `scribe sync --json` immediately after install → expect no-op (not a false "missing upstream" update), verifying the `blobsha.go` fix.

## Migration / backwards compatibility

- Existing `config.yaml` files without `builtins_version` key load cleanly (omitempty + zero value). Zero triggers backfill on next command.
- After first run against the new binary, `builtins_version: 2` appears in the persisted config.
- Downgrading scribe after `builtins_version` is written is safe — old scribe versions ignore unknown YAML keys per `yaml.v3` defaults. The only quirk is that downgraded scribe won't know it's behind if the user later upgrades to a version with `currentBuiltinsVersion = 3`; that's a non-issue because the upgraded binary will still see `builtins_version: 2 < 3` and backfill correctly.
- No state migration, no on-disk store migration, no breaking change to existing commands.

## Implementation subtasks (for the plan)

1. Verify exact JSON output shapes of each command listed in the skill body.
2. `internal/provider/treescan.go`: fix root-level `skillPath` (`"."` → `"SKILL.md"`).
3. `internal/provider/github.go`: add single-file fetch branch using `path.Base(skillPath) == "SKILL.md"` discriminator.
4. `internal/sync/blobsha.go`: fix `resolveSkillBlobSHA` for single-file entries.
5. `internal/config/config.go`: add `BuiltinsVersion int` field.
6. `internal/firstrun/firstrun.go`: add `currentBuiltinsVersion` const, add `Naoray/scribe` to `builtinRepos`, rewrite `ApplyBuiltins` as version-gated diff.
7. `cmd/root.go`: move `ApplyBuiltins` out of first-run gate; print banner to stderr.
8. Write `SKILL.md` at repo root with the finalized content from Section 1 (including `name: scribe-agent` frontmatter).
9. All tests from Testing section.
10. Manual verification walkthrough.
11. Commit in ordered phases per project commit discipline (one commit per logical phase: tree-scan fix, single-file fetch, blobsha fix, builtins version gate, banner move, skill content). PR squash-merged to main.

## Open questions resolved during brainstorming + counselors review

- **Skill location:** SKILL.md at repo root, no scribe.yaml / marketplace.json. Forces fix of latent single-file-skill bug.
- **Skill name:** `scribe-agent`, not `scribe`. Avoids state.json name collision risk and the awkward `scribe add Naoray/scribe:scribe` double-scribe install path.
- **Fetch discriminator:** `path.Base(skillPath) == "SKILL.md"`, not `HasSuffix(".md")`. Precise, avoids `foo.md` directory edge case.
- **Blob-SHA resolution:** `resolveSkillBlobSHA` needs a parallel branch for single-file entries — otherwise sync falsely classifies installed single-file skills as "missing upstream" every run.
- **Default registry mechanism:** version-gated marker (`BuiltinsVersion int`), not per-repo `SeenBuiltins`. Simpler schema, matches what the codebase can actually test today (no `registry remove` command exists).
- **Banner location:** stderr, not stdout. Prevents `--json` output corruption for agents running in PTYs.
- **Team/community type:** out of scope; reject the distinction in principle (see `feedback_registry_private_public.md`) but don't refactor in this spec.
- **Local registry index:** out of scope, separate future spec (`project_local_registry_index.md`).

## Counselors review

Reviewed by claude-opus + codex-5.3-high (gemini-3-flash quota-exhausted) on 2026-04-11. Outputs archived at `agents/counselors/1775931960-review-request-question-is-the-three-pa/`. Key findings incorporated into this revision:

- **Critical:** codex caught `internal/sync/blobsha.go:26` breaking for single-file skills. Fix added as Change 3 in Section 2.
- **High:** both agents flagged `HasSuffix(".md")` as fragile. Replaced with `path.Base() == "SKILL.md"`.
- **High:** both agents flagged `SeenBuiltins` as over-designed for untestable deletion semantics. Replaced with `BuiltinsVersion int`.
- **Medium:** both agents flagged banner-on-stdout as a JSON-corruption risk for agent callers. Moved to stderr.
- **Medium:** both agents flagged `scribe:scribe` name collision risk. Renamed to `scribe-agent`.
- **Factual correction:** my original narrative claimed `Path: "."` makes `FetchDirectory` "pull the whole repo". Wrong — it actually errors with `no files found under "."`. Corrected.
- **Rejected:** codex recommended splitting into three PRs. Siding with claude: the three changes are too tightly coupled to land usefully apart. Ship as one PR with ordered commits per project commit discipline.
