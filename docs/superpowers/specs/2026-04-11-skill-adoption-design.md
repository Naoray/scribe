# Skill Adoption

**Date:** 2026-04-11
**Status:** Design (revision 1)
**Author:** brainstorming session

## Problem

Scribe only manages skills it installed. Skills users acquired through other routes — hand-written, `git clone` into `~/.claude/skills/`, Claude Code plugins, Cursor rules checked in per-project, Laravel Boost's `.ai/skills/` — are invisible to Scribe. There is no single view of "what AI skills exist on this machine," no update surface for them, no way to enable/disable them per tool, and no way to sync them to additional tools the user adds later.

`internal/discovery.OnDisk` already walks `~/.claude/skills/` and `~/.codex/skills/` and surfaces the skills it finds, but those entries are read-only presentation. They are not in `state.Installed`, so `scribe sync`, `scribe tools loadout`, and the per-skill tool TUI (per `2026-04-11-per-skill-tool-management-design.md`) all skip them. The discovery layer is doing most of the "find" work already; what is missing is the "adopt" step that moves the skill into the canonical store and records it in state so the rest of the system can manage it.

This matches the project-memory vision that Scribe is the single skill manager for the machine, not just a registry-sync tool.

## Goals

- Detect skills that live in known tool-facing directories and are not yet in `state.Installed`.
- Import them into the canonical store `~/.scribe/skills/<name>/` and re-link each target via the existing `tools.Tool` symlink install path, so any tool that was already reading the skill keeps reading the same file content after adoption.
- Record adopted skills in `state.Installed` with a local source so the existing sync, listing, tool-management, and removal flows work on them unmodified.
- Let the user control adoption behavior through `config.yaml` with an `auto | prompt | off` mode, defaulting to auto when the field is unset.
- Keep adoption idempotent and reversible — re-running it is a no-op, and removing an adopted skill via `scribe remove` puts things back to a "not managed" state without leaving behind orphaned symlinks into the canonical store.
- Document the feature in `scribe guide`, the README, and the `scribe config` surface so users are never surprised by files moving.

## Non-Goals

- Adopting **project-scoped** skills (`<project>/.ai/skills/`, `<project>/.cursor/rules/`). Project-scoped content is per-checkout, not machine-global; adopting it into `~/.scribe/skills/` would break project ownership. v1 only adopts from machine-global directories. A follow-up spec can introduce project-scoped adoption with a per-project state file.
- Adopting from **Gemini**. Gemini's skill directory layout is not firm yet and `discovery.OnDisk` explicitly does not scan it. Adoption respects the same boundary.
- Reconstructing **upstream provenance**. An adopted skill's `Sources` field stays empty; its source is `local`. Future work may re-associate adopted skills with a registry entry once registries gain per-skill matching, but v1 treats provenance as unknown.
- Adopting **packages**. Packages have install/update commands and are not plain skill directories. Adoption skips anything that would write `state.InstalledSkill.Type == "package"`.
- Updating adopted skills **in place from their original location** after adoption. Once adopted, the canonical store is the source of truth — users editing the old tool-facing path are editing a symlink target, which is the canonical path. This is the same invariant Scribe already enforces for registry-synced skills.
- CLAUDE.md / snippet adoption. Tracked separately in the sharable-snippets project memory.

## Key Decisions

### 1. Adoption is a separate phase of `scribe sync`

Adoption runs at the start of `scribe sync`, before registry reconciliation. Rationale:

- `scribe sync` is the single entry point users already run. Putting adoption here means a freshly installed Scribe on an existing machine picks up the user's prior skills on first sync without a second command.
- Running adoption **before** registry reconciliation means a registry-sourced skill with the same name as an already-installed local skill wins the name: registry reconciliation overwrites the adopted entry and flips its source from local to the registry. This matches the user's intent when they connect a registry that happens to ship an identically named skill.
- `scribe list` also triggers discovery today but does not write state. List stays read-only; the discovery output already marks skills as "not managed" for visibility, which is the prompt that leads the user to run sync.

A new `scribe adopt` command is also added as an explicit trigger for users who want to adopt without running a full registry sync. It is the same codepath as the sync-prelude, just invoked directly.

### 2. Config gate: `adoption.mode` with `auto` default

```go
type AdoptionConfig struct {
    Mode  string   `yaml:"mode,omitempty"`  // "auto" | "prompt" | "off"
    Paths []string `yaml:"paths,omitempty"` // optional extra dirs; builtins always included
}

type Config struct {
    // ...existing fields
    Adoption AdoptionConfig `yaml:"adoption,omitempty"`
}
```

- Zero-value (`Mode == ""`) resolves to `auto`. A new helper `(c *Config) AdoptionMode() string` centralizes the default so every call site stays consistent. This is important because Go's zero-value-means-off idiom would otherwise invert the intended default.
- `auto` — adopt silently, print a one-line summary. Non-TTY-safe.
- `prompt` — show each candidate and ask `[y/N/all/none]`. On non-TTY, degrades to `off` with a one-line warning to stderr suggesting `--yes` or switching the mode.
- `off` — discovery still surfaces unmanaged skills in `scribe list` output, but sync does not touch them.

The `Paths` field is additive: the builtin scan list (`~/.claude/skills/`, `~/.codex/skills/`) is always walked; `Paths` extends it. Relative paths are resolved against the user's home directory. Paths outside the home directory are rejected at config-load time with a clear error (prevents `Paths: ["/"]` disasters).

### 3. Adoption is install-first via the existing tool pipeline

Once a candidate is confirmed for adoption:

1. Read the skill files from the on-disk location (`discovery.Skill.LocalPath`).
2. Call `tools.WriteToStore(name, files)` to materialize the canonical copy at `~/.scribe/skills/<name>/`. This is the same helper registry-sourced skills already use; it handles reserved names, path traversal rejection, and base-hash capture.
3. For each target the skill was originally found in (`claude`, `codex`): remove the old path, then re-run `tool.Install(name, canonicalDir)` to replace it with a symlink into the canonical store. This is the same `tools.Tool` interface the syncer uses; no new install codepath.
4. Record the skill in `state.Installed` with:
    - `Sources: nil` (no registry)
    - `Tools: [<targets discovered>]`
    - `ToolsMode: ToolsModeInherit` (per the per-skill tool mgmt spec; inherit means subsequent `tools enable/disable` reconciles adopted skills the same way as registry-sourced ones)
    - `InstalledHash: <SKILL.md blob sha computed during WriteToStore>`
    - `Revision: 1`
    - A new `Origin: "local"` field (see next decision)

Install-first ordering matters: if the symlink swap fails mid-flight (disk full, path exists as a non-symlink file, permission denied), the user's original file is still readable from the old path because we only remove it *after* the store write succeeds. On failure, we do not record the skill in state and we emit an error event. The canonical-store copy is still present but unreferenced — the next adoption run will see it and retry rather than duplicate.

### 4. New `InstalledSkill.Origin` field

```go
type Origin string

const (
    OriginRegistry Origin = ""       // default: zero value — came from a registry
    OriginLocal    Origin = "local"  // adopted or hand-written
)

type InstalledSkill struct {
    // ...existing fields
    Origin Origin `json:"origin,omitempty"`
}
```

Rationale: we cannot infer "adopted" from `Sources` alone. A registry-sourced skill has `Sources` populated, but a future feature might also populate `Sources` for local skills (e.g., re-linking to a registry match). An explicit field gives sync and list code a one-liner check — `if skill.Origin == state.OriginLocal` — instead of reasoning about combinations of empty fields.

`scribe list` renders local-origin skills with a subtle marker (e.g. `(local)` in the source column) so users can see at a glance which skills have no upstream. `scribe list --json` emits `origin` as a field.

### 5. Candidate detection reuses `discovery.OnDisk`

No new walker. `discovery.OnDisk` already:
- Scans `~/.scribe/skills/` first and marks those as `seen`.
- Scans `~/.claude/skills/` and `~/.codex/skills/`, deduplicating by name.
- Resolves symlinks so a tool-facing entry that already points into the canonical store is recognized as already managed.
- Falls through to state-tracked orphans.

Adoption adds one line of reasoning: a `Skill` returned by `OnDisk` is an adoption candidate iff:
- `skill.LocalPath != ""` (found on disk, not a state-only orphan)
- The path does **not** resolve into `~/.scribe/skills/` (so it is not already managed via the canonical store)
- `state.Installed[skill.Name]` is absent (not already tracked — if the name is present but the path is elsewhere, we have a conflict: see §6)
- `skill.Package == ""` (sub-skills of package skills are not independently adoptable — they move as a unit with their parent)

The new `internal/adopt` package wraps this filter in a `FindCandidates(st *state.State, cfg config.AdoptionConfig) ([]Candidate, error)` function. Because `discovery.OnDisk` already does the walk, adoption is cheap to run on every sync.

### 6. Conflict resolution: name clash

If `state.Installed` already has a skill named `commit` and the adopter finds an unmanaged `~/.claude/skills/commit/` whose SKILL.md differs from the canonical copy, we have a conflict. Resolution:

- **Auto mode:** skip silently and record in the sync summary as `1 skill skipped (name conflict)`. The user can see details with `scribe list --verbose`. Rationale: auto mode must never destroy user content.
- **Prompt mode:** ask `commit: unmanaged copy differs from managed version. [s]kip / [o]verwrite managed / [r]eplace unmanaged with managed / show [d]iff`. Overwrite imports the unmanaged copy into the store (replacing the canonical files) and bumps `Revision`. Replace just runs a normal `tool.Install` to swap the unmanaged path for a symlink to the canonical store (standard "fix drift" path). Diff pipes to `$PAGER` or `less`.

Conflicts are detected by comparing the `discovery.Skill.ContentHash` against the canonical store's content hash. Identical hashes are not conflicts — they are cleaned up by simply re-symlinking the tool-facing path to the store (an idempotent no-op if the symlink is already correct).

### 7. `scribe list` surfaces unmanaged skills differently

Today, `discovery.OnDisk` already returns unmanaged tool-facing skills and `scribe list` renders them. The change is a status column: managed skills show `managed`, unmanaged ones show `unmanaged (run: scribe adopt <name>)`. This keeps parity with the adoption flow and makes `off` mode discoverable without making the default output noisy.

### 8. Adoption in the first-run experience

`internal/firstrun` triggers on the first `scribe` invocation without state. It currently walks the user through registry setup. The revision adds: **after** registry setup, run adoption in `prompt` mode one-shot, regardless of the persisted config default. Rationale: the first-run experience is the only moment where asking "Scribe found 12 skills in ~/.claude/skills — adopt them?" is appropriate, because a first-time user cannot know they need to check. After first run, the config default takes over.

## User Experience

### A. Default sync output (auto mode)

```
$ scribe sync
→ Checking for unmanaged skills...
  ✓ adopted 3 skills (commit, audit-drift, review) into ~/.scribe/skills/
→ Syncing registries...
  ✓ 17 skills up to date
Done.
```

One line per adopted skill is fine; if more than ~5, collapse to `adopted 12 skills (run scribe list --origin=local to see them)`.

### B. Prompt mode

```
$ scribe sync
→ Checking for unmanaged skills...
Found 3 unmanaged skills:

  1. commit          ~/.claude/skills/commit/
     "Create clean commits with clear messages"

  2. audit-drift     ~/.claude/skills/audit-drift/
     "Check for drift between intended and actual state"

  3. review          ~/.codex/skills/review/
     "Review diffs for style and correctness"

Adopt all? [y/N/each]: each

commit (1/3)? [y/N/skip-all]: y
audit-drift (2/3)? [y/N/skip-all]: n
review (3/3)? [y/N/skip-all]: y

  ✓ adopted commit, review
  ⏭ skipped audit-drift
→ Syncing registries...
```

Non-TTY fallback:

```
$ scribe sync
Warning: adoption mode is "prompt" but stdin is not a terminal.
         Skipping adoption. Run `scribe adopt --yes` or set
         adoption.mode to auto/off in ~/.scribe/config.yaml.
→ Syncing registries...
```

### C. Standalone `scribe adopt`

```
scribe adopt                 # same as sync prelude — respects config mode
scribe adopt --yes           # force auto even if config says prompt
scribe adopt --dry-run       # show what would happen, write nothing
scribe adopt <name>          # adopt one skill by name (from any scanned path)
scribe adopt --json          # machine output for CI / agents
```

`--dry-run` prints the would-be summary line and, with `--verbose`, the full plan (read from, write to, link targets).

### D. `scribe config adoption`

Minimal config surface:

```
scribe config adoption                 # print current mode + paths
scribe config adoption --mode auto
scribe config adoption --mode prompt
scribe config adoption --mode off
scribe config adoption --add-path ~/src/my-skills
scribe config adoption --remove-path ~/src/my-skills
```

Writes back to `~/.scribe/config.yaml` atomically via the existing `Config.Save()` path.

### E. Conflict prompt (prompt mode only)

```
Conflict: commit
  managed:   ~/.scribe/skills/commit/          (rev 4, hash abc1234)
  unmanaged: ~/.claude/skills/commit/          (hash def5678)

[s]kip / [o]verwrite managed / [r]eplace unmanaged / show [d]iff: d

--- managed SKILL.md
+++ unmanaged SKILL.md
@@ -12,3 +12,4 @@
 ...diff...

[s]kip / [o]verwrite managed / [r]eplace unmanaged: s
  ⏭ skipped commit (conflict)
```

## Architecture

### New package: `internal/adopt`

```go
package adopt

import (
    "github.com/Naoray/scribe/internal/config"
    "github.com/Naoray/scribe/internal/discovery"
    "github.com/Naoray/scribe/internal/state"
)

// Candidate is an unmanaged skill that adoption can import.
type Candidate struct {
    Name      string
    LocalPath string   // source dir on disk
    Targets   []string // tools whose directories already contain this skill
    Hash      string   // content hash (matches discovery.Skill.ContentHash)
}

// Conflict describes a name collision between an unmanaged skill and a managed one.
type Conflict struct {
    Name      string
    Managed   state.InstalledSkill
    Unmanaged Candidate
}

// FindCandidates walks configured adoption paths and returns candidates and
// conflicts. Never mutates state. Cheap: one on-disk scan via discovery.OnDisk.
func FindCandidates(st *state.State, cfg config.AdoptionConfig) ([]Candidate, []Conflict, error)

// Plan describes what Adopt will do when Apply is called. Pure — no I/O.
type Plan struct {
    Adopt     []Candidate
    Conflicts []Conflict // unresolved
}

// Decision is the user's choice for a single conflict.
type Decision int
const (
    DecisionSkip            Decision = iota
    DecisionOverwriteManaged           // import unmanaged into store
    DecisionReplaceUnmanaged           // re-link unmanaged to the managed store
)

// Resolve folds user decisions into the plan, producing the final set of
// candidates to adopt.
func Resolve(p Plan, decisions map[string]Decision) []Candidate

// Apply adopts each candidate. Install-first ordering. Per-candidate failure
// is non-fatal; successful adoptions are committed to state. Emits *Msg events
// via the same callback pattern as internal/sync.
type Adopter struct {
    State *state.State
    Emit  func(any)
}

func (a *Adopter) Apply(candidates []Candidate) Result

type Result struct {
    Adopted  []string
    Skipped  []string
    Failed   map[string]error
}
```

### Event types

```go
type AdoptCandidateFoundMsg struct { Candidate Candidate }
type AdoptStartedMsg        struct { Name string }
type AdoptedMsg             struct { Name string; Tools []string }
type AdoptErrorMsg          struct { Name string; Err error }
type AdoptCompleteMsg       struct { Adopted, Skipped, Failed int }
```

Naming matches `internal/sync` conventions so the cmd/ layer can multiplex adoption and sync events through the same Bubble Tea program.

### Files read from disk

The adopter needs the raw file bytes to feed `tools.WriteToStore`. `discovery.Skill.LocalPath` gives the source directory. A new helper `adopt.readSkillFiles(localPath string) ([]tools.SkillFile, error)` walks the directory, skips reserved names (`versions/`, `.git/`), rejects path traversal, and returns the flat list `WriteToStore` expects. This mirrors how the syncer reads blobs from GitHub, just with `os.ReadFile` as the source.

### Config changes

`internal/config/config.go`:

```go
type AdoptionConfig struct {
    Mode  string   `yaml:"mode,omitempty"`
    Paths []string `yaml:"paths,omitempty"`
}

func (c *Config) AdoptionMode() string {
    switch c.Adoption.Mode {
    case "auto", "prompt", "off":
        return c.Adoption.Mode
    default:
        return "auto"
    }
}

func (c *Config) AdoptionPaths() ([]string, error) {
    home, _ := os.UserHomeDir()
    out := []string{
        filepath.Join(home, ".claude", "skills"),
        filepath.Join(home, ".codex", "skills"),
    }
    for _, p := range c.Adoption.Paths {
        abs := p
        if strings.HasPrefix(p, "~/") {
            abs = filepath.Join(home, p[2:])
        }
        clean := filepath.Clean(abs)
        if !strings.HasPrefix(clean, home) {
            return nil, fmt.Errorf("adoption.paths entry %q is outside home", p)
        }
        out = append(out, clean)
    }
    return out, nil
}
```

The validation in `AdoptionPaths` runs eagerly in `Load()` so a bad config fails fast instead of at adoption time.

### Syncer hook

`internal/sync/syncer.go`:

```go
func (s *Syncer) Run(ctx context.Context) error {
    if s.adoptionEnabled() {
        if err := s.runAdoption(ctx); err != nil {
            s.emit(SyncAdoptionFailedMsg{Err: err})
            // Non-fatal: continue with registry sync
        }
    }
    // ...existing registry reconciliation
}
```

Adoption is wired via a small adapter in `cmd/sync.go` that instantiates an `adopt.Adopter` with the same `Emit` callback as the syncer. The syncer itself does not depend on the `adopt` package directly — `cmd/` orchestrates both, same pattern as how `cmd/sync.go` wires the syncer to the UI today. Keeps the core/UI-agnostic contract intact (no new cross-package core coupling inside `internal/sync`).

### Discovery changes

`internal/discovery/discovery.go:OnDisk` gains a `managed bool` field on `Skill` (true iff `state.Installed[name]` is present and `LocalPath` resolves into `~/.scribe/skills/`). `cmd/list.go` uses this to render the managed/unmanaged column. This is a small additive change — no rename of existing fields.

### State changes

`internal/state/state.go`:

- Add `Origin` field (see §4). Zero-value is the registry default; adopted skills set `OriginLocal`.
- No schema bump required. The field is additive; legacy state entries load with the zero value, which is correct (existing entries all came from registries).

### CLI changes

- `cmd/adopt.go` (new) — `scribe adopt` command with `--yes`, `--dry-run`, `--json`, `--verbose`, and a positional name filter.
- `cmd/sync.go` — call the adopter before registry sync; merge events into the existing TUI/plain-text printers.
- `cmd/list.go` — render managed/unmanaged column and `(local)` origin tag.
- `cmd/config.go` (new subcommand `adoption`) — mutate `Config.Adoption` and save.
- `cmd/firstrun.go` (or the existing firstrun flow) — post-registry adoption prompt, one-shot prompt mode.

## Error Handling

| Situation | Auto mode | Prompt mode | CLI `scribe adopt` | `scribe sync` wrapper |
|---|---|---|---|---|
| No adoption paths detected | No-op, no output | No-op, no output | Exit 0, print "nothing to adopt" | Continues to registry sync |
| Config `adoption.paths` outside home | — | — | Exit 1 at config load | Exit 1 at config load |
| Candidate SKILL.md unreadable | Skip, log warn to stderr | Skip, continue | Skip, continue, exit 1 at end | Emit `AdoptErrorMsg`, continue |
| `tools.WriteToStore` fails (reserved name, traversal) | Skip, emit error | Skip, emit error | Skip, continue, exit 1 at end | Same |
| `tool.Install` symlink swap fails | Roll back: leave canonical store copy, do **not** record in state, emit error | Same | Same | Same |
| Name conflict (hash differs) | Skip silently, count in summary | Prompt user | Auto skip unless `--resolve=overwrite-managed` (future) | Same as sync prelude |
| Name match, hash identical | Re-link tool-facing path to store; record as adopted | Same | Same | Same |
| Prompt mode on non-TTY | n/a | Degrade to off with stderr warning | Exit 1 unless `--yes` passed | Warn, continue to registry sync |
| `--dry-run` | n/a | n/a | No writes, print plan | n/a |
| Package-origin candidate | Skipped in `FindCandidates` | Skipped | Skipped | Skipped |
| `discovery.OnDisk` returns error (e.g. read perm on `~/.claude/skills/`) | Log warn, continue with whatever was collected | Same | Same, exit 1 at end | Emit `SyncAdoptionFailedMsg`, continue |

Install-first rollback is the most important property: a failed adoption must never delete the user's original file before the canonical copy exists and every target install has succeeded. The adopter tracks "fully committed" per-candidate and only calls `state.RecordInstall` after all tool installs returned without error.

## Testing

### `internal/adopt`

Table-driven tests using `t.Setenv("HOME", t.TempDir())`:

- `FindCandidates` returns:
  - Candidates from `~/.claude/skills/` that are not in state.
  - Nothing when the dir is empty or missing.
  - Nothing when the skill already lives in `~/.scribe/skills/` (resolved via symlink).
  - A `Conflict` when the name exists in state with a different content hash.
  - No candidate when the name exists in state with the same content hash *and* the tool-facing path is already a symlink into the store (pure no-op).
  - Skips package sub-skills.
  - Honors `cfg.Paths` additions (relative, `~/`, absolute-in-home).
  - Rejects `cfg.Paths` entries outside home at config load time.

- `Apply` happy path:
  - Candidate found → canonical store has `SKILL.md` with matching content → tool-facing path is now a symlink into the store → `state.Installed[name]` populated with `Origin=OriginLocal`, `ToolsMode=ToolsModeInherit`, `InstalledHash` non-empty, `Revision=1`, `Sources=nil`, `Tools` = discovered targets.
  - Idempotent: running `Apply` twice on the same input is a no-op on the second run.

- `Apply` failure path:
  - Mock `tools.Tool.Install` that returns an error. Assert the candidate's original file is still present at its source path, the canonical store copy was written (leftover for retry, documented), and `state.Installed` was **not** mutated.
  - Partial failure in a batch of 3 candidates: first succeeds, second fails mid-install, third still runs. Summary reports 2 adopted, 1 failed.

- `Resolve`:
  - Unresolved conflicts filtered out.
  - `DecisionOverwriteManaged` promotes the conflict into a candidate.
  - `DecisionReplaceUnmanaged` produces a "re-link only" candidate (no store write, just `tool.Install` swap) — represented via a field on `Candidate` or a sibling type, TBD in implementation.

### `internal/config`

- `AdoptionMode` returns `auto` for zero value, raw value for `auto`/`prompt`/`off`, and falls back to `auto` for garbage.
- `AdoptionPaths` expands `~/` and resolves relative-to-home.
- `AdoptionPaths` rejects absolute paths outside home.
- Round-trip: load → mutate `Config.Adoption` → save → load → values match.

### `internal/discovery`

- `OnDisk` populates the new `managed` field correctly for every code path (canonical store, tool-facing symlink into store, tool-facing non-managed, state-orphan).

### `internal/sync`

- Syncer prelude calls the adopter when config is auto, does not call it when off, and degrades to off on non-TTY in prompt mode.
- A registry-sourced skill with the same name as an adopted skill promotes the state entry: `Origin` flips from `OriginLocal` to `OriginRegistry`, `Sources` becomes non-empty.

### `cmd`

- `cmd/adopt_test.go`: `--yes` forces auto, `--dry-run` writes nothing, `--json` structure, unknown-name exit code.
- `cmd/sync_test.go`: existing tests extended with an adoption fixture in the test temp home.
- `cmd/list_test.go`: managed/unmanaged column renders for both states; `(local)` origin tag renders; `--json` includes `origin` field.
- `cmd/config_test.go`: `config adoption --mode off` round-trips through `Config.Save`.

### `internal/firstrun`

- New test: firstrun with `~/.claude/skills/` non-empty prompts for adoption after registry setup and honors the user's choice independently of the saved config default.

## Rollout

Single PR. No feature flag. Order of changes within the PR:

1. `internal/state`: add `Origin` field. No migration needed.
2. `internal/config`: add `AdoptionConfig`, helpers, validation, tests.
3. `internal/discovery`: add `managed` field, tests.
4. `internal/adopt`: new package, tests.
5. `internal/sync`: add adoption prelude hook in `cmd/sync.go`, tests.
6. `cmd/adopt.go`: new command, tests.
7. `cmd/list.go`: new column + origin tag.
8. `cmd/config.go`: adoption subcommand.
9. `cmd/firstrun`: post-registry adoption prompt.
10. Docs: README section + `cmd/guide.go` section.

Depends on `2026-04-11-per-skill-tool-management-design.md` landing first, because adoption sets `ToolsMode: ToolsModeInherit` on records and the reconcile-on-`tools enable/disable` behavior is what makes inherited adopted skills pick up newly enabled tools. If that spec is not yet landed, adoption still works — the `ToolsMode` field is zero-valued, which the per-skill spec defines as inherit — but the UX of "enable a new tool and adopted skills auto-backfill" requires both changes.

On first sync after upgrade, auto-mode users will see one adoption summary line printing however many hand-rolled skills Scribe found. This is the intended effect — the whole point is to make those skills visible. The one-line output avoids drowning the sync UI.

## Open Questions

1. **Should `scribe adopt --dry-run` also run in non-TTY mode for CI discovery?** Leaning yes — `--dry-run --json` should emit a machine-readable list so CI agents can preflight adoption before committing. No blocking decision needed for v1; `--dry-run --json` is additive.

2. **Cursor adoption.** Cursor skills are in `<project>/.cursor/rules/` (project-scoped, per the per-skill tool-management non-goals). v1 does not adopt them. But if a user has `~/.cursor/rules/` globally, should we? Safer to keep the v1 scope to Claude + Codex and revisit Cursor when the project-scoped adoption spec lands.

3. (resolved) `scribe remove` on adopted skills behaves identically to remove on registry-sourced skills: drop from `state.Installed`, delete `~/.scribe/skills/<name>/`, uninstall every recorded symlink. No restoration of the pre-adoption file tree. Remove means "stop managing this skill" and that is all it means. Adopted skills get no special-case fallback path. Users who want the files back pull them from git history or reinstall from source.
