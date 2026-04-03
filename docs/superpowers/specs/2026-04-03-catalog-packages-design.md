# Catalog, Packages & Author Enforcement

**Date:** 2026-04-03
**Status:** Approved

## Summary

Evolve Scribe's registry format from TOML to YAML, rename `[skills]` to `catalog`, add support for third-party packages with custom installers, enforce skill authorship via GitHub identity, and add a migration path for existing registries.

## Context

Scribe currently treats all registry entries as individual skills it fully manages (download, symlink, update). Third-party skill frameworks like gstack and superpowers have their own install mechanisms that Scribe can't replace. The agentskills.io spec defines the skill format but deliberately excludes distribution — Scribe fills that gap as a complementary distribution layer.

### Key decisions from research

- **Format/distribution split**: agentskills.io = format layer (SKILL.md), Scribe = distribution layer (registry + sync). Don't extend the spec, build alongside it.
- **External recipes**: Registry entries describe how to install packages that don't know Scribe exists (Homebrew formula pattern).
- **Hybrid install model**: Simple skills get symlinked by Scribe. Packages with custom installers declare install commands. The AI agent handles anything in-skill via SKILL.md instructions.
- **TOFU trust**: Prompt once for install commands, hash them, re-prompt on change.
- **Author = GitHub username**: Enforced on the upload path only.
- **Version in registry, not SKILL.md**: Registry entry + lockfile (future) is authoritative. `metadata.version` in SKILL.md is informational.

## New Manifest Format

File changes from `scribe.toml` to `scribe.yaml`. Two kinds:

### Team Registry

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
  description: Artistfy team skills

catalog:
  - name: recap
    source: github:Artistfy/hq@v1.0.0
    path: skills/recap
    author: krishan

  - name: gstack
    source: github:garrytan/gstack@main
    type: package
    install: >-
      git clone --depth 1 https://github.com/garrytan/gstack.git
      ~/.claude/skills/gstack && cd ~/.claude/skills/gstack && ./setup
    update: cd ~/.claude/skills/gstack && git pull && ./setup
    author: garrytan

  - name: superpowers
    source: github:obra/superpowers@main
    type: package
    install: /plugin install superpowers@claude-plugins-official
    author: obra
```

### Skill Package

```yaml
apiVersion: scribe/v1
kind: Package
package:
  name: my-skills
  version: "1.0.0"
  description: My custom skills
  license: MIT
  authors:
    - krishan
  repository: github.com/krishan/my-skills

catalog:
  - name: deploy
    path: skills/deploy
  - name: review
    path: skills/review
```

### Catalog Entry Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Skill or package name |
| `source` | Registry: yes, Package: no | `github:owner/repo@ref`. Package manifests omit this (skills are local to the repo). |
| `path` | No | Subdirectory in source repo. Omitted = whole repo. |
| `type` | No | `"package"` for packages with custom installers. Default: skill. |
| `install` | No | Shell command for package installation. Only for `type: package`. |
| `update` | No | Shell command for package updates. Only for `type: package`. |
| `author` | Registry: yes, Package: no | GitHub username of the creator. Package manifests use `package.authors` instead. |
| `description` | No | Short description for display in `scribe list` and `scribe add`. For skills, falls back to SKILL.md frontmatter. For packages, recommended since there may be no SKILL.md to read. |
| `timeout` | No | Timeout in seconds for install/update commands. Default: 300 (5 minutes). Only for `type: package`. |

### Filename

The manifest filename changes from `scribe.toml` to `scribe.yaml`. The constant `ManifestFilename` in `internal/manifest/` updates accordingly.

### Changes from current format

- TOML → YAML
- `[skills]` keyed map → `catalog` ordered list with `name` field
- `apiVersion` + `kind` for future extensibility
- `private` field removed
- `author` field added (was inferred, now explicit)
- `type`, `install`, `update` fields added for packages
- `BurntSushi/toml` dependency replaced by `gopkg.in/yaml.v3`

## Manifest Parser Rewrite

`internal/manifest/` rewritten for YAML.

### Structs

```go
type Manifest struct {
    APIVersion string   `yaml:"apiVersion"`
    Kind       string   `yaml:"kind"`       // "Registry" or "Package"
    Team       *Team    `yaml:"team"`
    Package    *Package `yaml:"package"`
    Catalog    []Entry  `yaml:"catalog"`
    Targets    *Targets `yaml:"targets"`
}

type Entry struct {
    Name        string `yaml:"name"`
    Source      string `yaml:"source"`
    Path        string `yaml:"path"`
    Type        string `yaml:"type"`
    Install     string `yaml:"install"`
    Update      string `yaml:"update"`
    Author      string `yaml:"author"`
    Description string `yaml:"description"`
    Timeout     int    `yaml:"timeout"`    // seconds, default 300
}

type Team struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
}

type Package struct {
    Name        string   `yaml:"name"`
    Version     string   `yaml:"version"`
    Description string   `yaml:"description"`
    License     string   `yaml:"license"`
    Authors     []string `yaml:"authors"`
    Repository  string   `yaml:"repository"`
}
```

### Key behaviors

- `Entry.IsPackage()` returns `e.Type == "package"`
- `Entry.Maintainer()` returns `e.Author` directly (no inference needed)
- `Source` parsing stays the same (`github:owner/repo@ref` → `Source` struct)
- Lookup by name: linear scan over `[]Entry` (catalog is small)
- `Encode()` serializes back to YAML
- `Parse()` accepts `[]byte`, returns `*Manifest`
- Validation: `apiVersion` must be `scribe/v1`, `kind` must be `Registry` or `Package`, cannot have both `team` and `package`
- Validation: duplicate `name` values in `catalog` are rejected at parse time (lists allow duplicates unlike TOML maps — must be enforced)
- Validation: `type` must be `""` (empty/omitted) or `"package"` — unknown values are parse errors, not silent defaults
- Validation: `install` without `type: package` is a warning (allows future skill post-install hooks)
- Validation: `path` on a `type: package` entry is ignored (packages manage their own install location)
- `FindByName(string) *Entry` helper for consistent O(1)-like lookup pattern

## Catalog Entry Discovery (`scribe add`)

### Adding from a third-party repo

`scribe add garrytan/gstack` runs a detection chain:

**Step 1 — Check for `scribe.yaml` (or legacy `scribe.toml`)**
If found, parse it. Catalog entries are already defined. Present to user. For `scribe.toml`, emit legacy format warning alongside results.

**Step 2 — Check for plugin manifest (`.claude-plugin/plugin.json`)**
If found, it's a plugin package. Auto-generate catalog entry with `type: package` and inferred install command (`/plugin install <name>`). Author defaults to repo owner. Note: if the repo also has SKILL.md files alongside the plugin manifest, present both the package entry and individual skills — let the user choose.

**Step 3 — Scan tree for `SKILL.md` files**
Use the recursive tree (already fetched via `GetTree`) and filter for any path ending in `/SKILL.md`. This catches all conventions:
- `skills/recap/SKILL.md` (agentskills convention)
- `.ai/skills/deploy/SKILL.md` (Laravel Boost convention)
- `browse/SKILL.md` (gstack-style, flat)
- `.agents/skills/foo/SKILL.md` (cross-client convention)

If found, it's a skills repo:
- Multiple SKILL.md files → offer as single package entry (whole repo) or individual skill entries. Default: single package entry. TUI shows both options with a toggle.
- Single SKILL.md → individual skill entry.

**Step 4 — Nothing detected**
Prompt: "No skills or plugin manifest found. Is this a package with a custom install process?" If yes, prompt for install command.

### New method

`Adder.DiscoverRepo(ctx, owner, repo string) ([]Candidate, error)` in `internal/add/`. Uses `GetTree` (recursive) to scan repo structure, then runs detection chain.

### Author resolution for third-party repos

1. `metadata.author` from SKILL.md frontmatter (if present)
2. Repo owner from source URL (fallback)

### Spec compliance gate

When uploading skill files (`NeedsUpload` path), Scribe validates and prompts for missing required agentskills spec fields:
- Missing `name` → default to directory name, confirm with user
- Missing `description` → prompt user to provide one
- Inject `metadata.author` with authenticated GitHub username
- Write updated frontmatter before pushing

This applies to both new skills and future skill updates (when `scribe push` is implemented).

## Sync Engine Changes

### Skills (type = "" / omitted)

Unchanged — download files from source, write to canonical store, symlink to targets. Compare via source ref + commit SHA.

### Packages (type = "package")

Scribe delegates to declared `install`/`update` commands.

**Install flow (not yet installed):**
1. Hash `install + "\n" + update` concatenated (SHA-256). Single hash covers both commands.
2. Check trust state: approved?
   - No → prompt user, show command + source registry
   - Yes → proceed
   - Non-interactive (CI/piped stdout) → skip package with warning, or proceed if `--trust-all` flag is set
3. Execute install command with context deadline (entry `timeout` or default 300s)
   - Capture stdout/stderr — do not let subprocess write to terminal in TUI mode (corrupts Bubble Tea display)
   - On success: record in state with commit SHA, command hash, approval timestamp
   - On failure: do NOT record in state. Emit `PackageErrorMsg` with captured stderr. Do not attempt cleanup — warn user to clean up manually.
4. Record in state only on success

**Update flow (already installed):**
1. Fetch latest commit SHA from source
2. Match installed SHA? → StatusCurrent, skip
3. Different? → has update command?
   - Yes → execute update command (same timeout/capture rules)
   - No → warn: "No update command declared and install may not be idempotent. Run manually: `<install command>`". Skip, do not re-run install blindly.
4. Update state only on success

### Trust state

Stored in `state.json` (no separate trust DB). New fields on `InstalledSkill`:

```go
type InstalledSkill struct {
    // existing fields
    Version     string    `json:"version"`
    CommitSHA   string    `json:"commit_sha,omitempty"`
    Source      string    `json:"source"`
    InstalledAt time.Time `json:"installed_at"`
    Targets     []string  `json:"targets"`
    Paths       []string  `json:"paths"`
    Registries  []string  `json:"registries,omitempty"`

    // new fields
    Type       string    `json:"type,omitempty"`
    InstallCmd string    `json:"install_cmd,omitempty"`
    UpdateCmd  string    `json:"update_cmd,omitempty"`
    CmdHash    string    `json:"cmd_hash,omitempty"`       // sha256(install + "\n" + update)
    Approval   string    `json:"approval,omitempty"`       // "", "approved", "denied" — tri-state, not bool
    ApprovedAt time.Time `json:"approved_at,omitempty"`
}
```

### Re-prompt triggers

- Install command hash changed in registry → show old vs new, ask again
- New package added → always prompt
- Unchanged approved command → proceed silently

### New events

```go
type PackageInstallPromptMsg struct{ Name, Command, Source string }
type PackageApprovedMsg      struct{ Name string }
type PackageDeniedMsg        struct{ Name string }
type PackageSkippedMsg       struct{ Name, Reason string }  // CI skip, user denied
type PackageInstallingMsg    struct{ Name string }
type PackageInstalledMsg     struct{ Name string }
type PackageErrorMsg         struct{ Name string; Err error; Stderr string }
```

### Package removal

When a package entry is removed from the registry catalog, `scribe sync` detects it as `StatusExtra`. For skills, Scribe can remove symlinks and store files. For packages, Scribe **cannot** clean up — it doesn't know what the install command did. Behavior: emit a warning ("gstack was removed from the registry but Scribe cannot uninstall it — remove manually") and clear the entry from `state.json`. Do not attempt automated cleanup.

### Non-interactive mode (CI)

When stdout is not a TTY, TOFU prompts cannot be shown. Behavior:
- `--trust-all` flag: approve all install commands without prompting (for trusted CI environments)
- Without `--trust-all`: skip packages that need approval, emit warning, continue with skills
- `--json` output includes skipped packages with reason `"approval_required"`

### Compare changes

`compareSkill` extended with entry type parameter. For packages: commit SHA match = current, mismatch = outdated. No semver logic for packages. Note: any commit to the source repo (even a README change) triggers "outdated" for branch-pinned packages. This is documented and accepted — the alternative (no update detection) is worse.

## Author Enforcement

Gate lives in one place: `Adder.Add()`, the `NeedsUpload` path.

### Flow

1. Call `client.AuthenticatedUser(ctx)` once at start of `Add()`
2. For each candidate being uploaded:
   - Entry exists in target registry?
     - No → new entry, current user becomes author
     - Yes → `author == current user`? Proceed. Otherwise emit `SkillAddDeniedMsg`, skip.

### New GitHub client method

```go
func (c *Client) AuthenticatedUser(ctx context.Context) (string, error)
```

Uses existing `c.gh.Users.Get(ctx, "")`, returns login.

### What's NOT gated

- Adding/removing source references (pointers, no file upload)
- Changing source ref, path, install command on existing entries
- Any operation on packages (they have their own install mechanisms)

**Scope limitation (documented, not a bug):** Author enforcement protects **skill file content**, not **registry pointer integrity**. Anyone with registry repo write access can point a catalog entry at a different source. This is intentional — the team lead must be able to update source refs, pin versions, and curate the catalog. Registry-level integrity is delegated to GitHub repository permissions and code review. For higher assurance, teams should enable branch protection and required reviews on their registry repo.

### Event

```go
type SkillAddDeniedMsg struct{ Name, Author string }
```

Output: `✗ recap — owned by krishan, only they can modify skill files`

## Frontmatter Parser Enhancement

`internal/discovery/` gains a proper YAML-based frontmatter parser.

### New struct and function

```go
type SkillMeta struct {
    Name        string
    Description string
    Version     string
    Author      string
}

func ReadSkillMeta(skillDir string) SkillMeta
```

Replaces `readSkillDescription`. Uses `yaml.v3` to parse frontmatter instead of line-by-line string matching.

### Read priority

**Version:**
1. `metadata.version` (agentskills spec)
2. `version` (top-level, gstack-style)
3. Empty string

**Author:**
1. `metadata.author` (agentskills spec)
2. `author` (top-level)
3. Empty string (caller falls back to repo owner)

### Internal struct

```go
type rawFrontmatter struct {
    Name        string         `yaml:"name"`
    Description string         `yaml:"description"`
    Version     string         `yaml:"version"`
    Author      string         `yaml:"author"`
    Metadata    map[string]any `yaml:"metadata"` // any, not string — agentskills allows nested values
}
```

Priority chain: `Metadata["version"]` (stringified) overrides `Version`, `Metadata["author"]` (stringified) overrides `Author`. Non-string metadata values are coerced via `fmt.Sprint`.

## `scribe migrate` Command

Converts existing `scribe.toml` registries to `scribe.yaml`.

### Transformation rules

| TOML | YAML |
|------|------|
| `[team]` section | `kind: Registry`, `team:` block |
| `[package]` section | `kind: Package`, `package:` block |
| `[skills.name]` keyed map | `catalog:` list with `name:` field |
| `skill.private` | Dropped |
| `[targets]` | Preserved |

### Flow

1. Fetch `scribe.toml` from registry repo
2. Parse with TOML parser (kept in `internal/migrate/` for this purpose only)
3. Convert to new YAML `Manifest` struct
4. Infer author from existing `Maintainer()` logic (path-based or source owner). Flag entries where author was inferred from an org name (not a personal GitHub username) with a `# TODO: verify author` comment in the output YAML.
5. Present converted YAML to user for review — user must confirm before push
6. Push single commit: delete `scribe.toml`, create `scribe.yaml`. Note: `PushFiles` must be extended to support file deletions (create tree entries with `SHA: nil` to remove files). This is a non-trivial GitHub client change.
7. Catalog order in output YAML is sorted alphabetically by name (TOML map iteration was non-deterministic)

### Legacy fallback

`scribe sync` and `scribe add` check for `scribe.yaml` first, fall back to `scribe.toml` if not found. When falling back, emit warning: "This registry uses the legacy format. Run `scribe migrate` to upgrade."

## Command Changes

### `scribe add`

New argument form:

```bash
scribe add                              # interactive discovery (existing)
scribe add garrytan/gstack              # add from third-party repo (new)
scribe add garrytan/gstack --skill browse  # add specific skill (new)
scribe add --to Artistfy/hq             # target registry (existing)
```

### `scribe sync`

Gains package awareness. Skills synced as before. Packages: trust check → execute install/update → record state.

### `scribe list`

Shows both types with author:

```
Artistfy/hq:
  recap         v1.0.0  krishan    ✓ current
  deploy        v1.1.0  krishan    ✓ current
  gstack        main    garrytan   ✓ package
  superpowers   main    obra       ✓ package
```

### `scribe migrate`

New command (see section above).

## Deferred Work

### Lockfile (`scribe.lock`)

Content-addressed pinning committed to registry repo for reproducibility. Ensures deterministic `scribe sync` across team. Separates `scribe check` (what's available) from `scribe sync` (apply pinned versions).

### `scribe push`

Explicit command for pushing local skill changes to a registry. Validates `name` and `description` on push. Distinct from `scribe add` (discovery + selection).

### `scribe check`

Shows available updates without applying them. Pairs with lockfile for the check → review → apply flow.

## Dependencies

### Added
- `gopkg.in/yaml.v3` — YAML parsing for manifest and SKILL.md frontmatter

### Removed
- `github.com/BurntSushi/toml` — replaced by YAML (kept temporarily in `internal/migrate/` for migration)

## Files Changed

| Area | Files | Change |
|------|-------|--------|
| Manifest | `internal/manifest/manifest.go` | Rewrite: YAML structs, `[]Entry` catalog |
| Manifest | `internal/manifest/source.go` | Unchanged (source parsing stays) |
| Manifest | `internal/manifest/manifest_test.go` | Rewrite for YAML |
| Sync | `internal/sync/syncer.go` | Package-aware apply, trust check, new events. New `CommandExecutor` interface for testability (fake success/failure/timeout in tests). |
| Sync | `internal/sync/compare.go` | Entry type parameter, package SHA comparison |
| Sync | `internal/sync/events.go` | New package events |
| Add | `internal/add/add.go` | Author enforcement, `DiscoverRepo`, spec compliance gate |
| Add | `internal/add/events.go` | New denied/package events |
| State | `internal/state/state.go` | New fields on `InstalledSkill` |
| GitHub | `internal/github/client.go` | `AuthenticatedUser()` method, `PushFiles` extended to support file deletions (tree entries with nil SHA) |
| Discovery | `internal/discovery/discovery.go` | `ReadSkillMeta`, YAML frontmatter parser |
| Migrate | `internal/migrate/migrate.go` | New: TOML→YAML conversion |
| Commands | `cmd/migrate.go` | New command |
| Commands | `cmd/add.go` | Repo argument, detection chain |
| Commands | `cmd/sync.go` | Package install prompts |
| Commands | `cmd/list.go` | Package display, author column |
