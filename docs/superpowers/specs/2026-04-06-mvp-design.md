# Scribe MVP — "Leave It Alone" Release

**Date:** 2026-04-06
**Status:** Approved
**Supersedes:** 2026-04-03-catalog-packages-design.md (incorporated and extended)

## Summary

Ship a complete Scribe that handles team registries, community registries, skill updates, tool management, and self-updates — then leave it alone. After this release, Scribe is a fully functional skill manager for any AI coding tool.

## Goals

1. Connect to any skill source: team registries, community repos, marketplace plugins, skills.sh
2. Install, update, and remove skills across all AI tools on the machine
3. Show everything on the machine in one view — what's installed, where it came from, who authored it
4. Self-update without friction
5. Let AI agents manage skills through Scribe via an agent skill

## Non-Goals

- Lockfile / `scribe.lock` (post-MVP)
- `scribe push` command (post-MVP)
- `scribe check` without applying (post-MVP)
- Global behavioral rules — managing CLAUDE.md rule fragments as shareable artifacts (post-MVP)
- Rules sync across LLMs — CLAUDE.md, .cursorrules, etc. (post-MVP)
- MCP server management (post-MVP)
- `npx skills` install/fetch adapter — skills.sh repos are GitHub repos, installation handled natively by GitHubProvider. Discovery via `npx skills find` IS in scope (search only)

---

## 1. Command Taxonomy

Daily skill management is top-level — no subcommand needed. Registry management (less frequent) gets the `registry` subcommand.

### Top-level commands (manage skills on MY machine)

| Command | Description |
|---------|-------------|
| `scribe list` | Show all skills on this machine |
| `scribe add [query]` | Browse & install skills from registries (+ skills.sh discovery) |
| `scribe remove <skill>` | Remove a skill from my machine |
| `scribe sync` | Pull updates from connected registries |
| `scribe upgrade` | Self-update Scribe |
| `scribe tools` | List detected tools, show enabled/disabled |
| `scribe tools enable/disable` | Toggle a tool on/off |

### Registry commands (manage registries & share skills)

| Command | Description |
|---------|-------------|
| `scribe registry connect <repo>` | Connect to a registry |
| `scribe registry create` | Create a new registry repo (unchanged from current) |
| `scribe registry add` | Share/publish a skill to a registry |
| `scribe registry list` | List connected registries with type, skill count, enabled status |
| `scribe registry enable/disable` | Toggle a registry on/off (skipped during sync/list when disabled) |
| `scribe registry migrate` | Convert TOML → YAML |

### `scribe add` — the discovery + install experience

`scribe add` is the main way users find and add skills to their machine. Interactive TUI by default, flags for agentic use.

```bash
scribe add                          # interactive TUI: browse all available skills
scribe add react                    # search "react" across registries + skills.sh
scribe add antfu/skills:nuxt        # install specific skill from specific registry
scribe add antfu/skills:nuxt --yes  # non-interactive, skip prompts (agent use)
scribe add react --json             # return search results as JSON (agent use)
```

**Search priority:**
1. Connected registries (already in config) — searched first, fastest
2. skills.sh directory (via `npx skills find`) — searched for broader discovery, shows repos not yet connected. If `npx` not available, search is limited to connected registries with a one-time hint to install Node.js.

**Auto-connect:** When a user installs a skill from a repo they haven't connected yet, Scribe auto-connects it (adds to config as community registry with cached type). Future `scribe sync` keeps it updated automatically.

**Argument parsing:** `owner/repo:skillname` format (matched by `^\w[\w.-]*/[\w.-]+:\S+$`) is treated as a direct install. Anything else is a search query.

### `scribe remove`

```bash
scribe remove recap                 # interactive: pick which registry's recap
scribe remove Artistfy-hq/recap     # specific: remove this exact skill
```

**Behavior:**
1. Remove symlinks from all tools where this skill is installed (calls `Tool.Uninstall()` using `InstalledSkill.Tools`, not just enabled tools)
2. Remove files from canonical store (`~/.scribe/skills/<registry-slug>/<name>/`)
3. Remove entry from `state.json`
4. If skill is still in a connected registry, note: "This skill is managed by Artistfy/hq. It will be re-installed on next sync."

For packages (`type: package`): warn that Scribe cannot uninstall what the install command did. Clear state entry only.

### Migration from current commands

| Old | New |
|-----|-----|
| `scribe add` (install to machine) | `scribe add` (unchanged) |
| `scribe add` (share to registry) | `scribe registry add` |
| `scribe list` | `scribe list` (unchanged, but now machine-first) |
| `scribe list --local` | `scribe list` (now the default) |
| `scribe connect` | `scribe registry connect` |
| `scribe create registry` | `scribe registry create` |

---

## 2. Terminology: Targets → Tools

"Targets" becomes "tools" throughout. This matches how users think — "I use Claude and Cursor" not "I target Claude and Cursor."

### Rename scope

| Old | New |
|-----|-----|
| `internal/targets/` | `internal/tools/` |
| `Target` interface | `Tool` interface |
| `ClaudeTarget` | `ClaudeTool` |
| `CursorTarget` | `CursorTool` |
| `DefaultTargets()` | `DetectTools()` |
| `InstalledSkill.Targets` | `InstalledSkill.Tools` |
| `--target` flags | `--tool` flags |
| `Manifest.Targets` | `Manifest.Tools` |

### Tool interface

```go
type Tool interface {
    Name() string
    Detect() bool  // NEW: checks if tool is installed on this machine
    Install(skillName, canonicalDir string) (paths []string, err error)
    Uninstall(skillName string) error  // NEW: remove symlinks for this tool
}
```

`Detect()` checks for the tool's config directory:
- Claude: `~/.claude/` exists
- Cursor: `~/.cursor/` exists
- Future tools: Windsurf, Cline, Copilot, etc.

### Active tools config

First run of any scribe command triggers auto-detection:
1. Run `Detect()` on all known tools
2. Show what was found, ask user to confirm (TTY) or accept defaults (non-TTY)
3. Save to config

```yaml
tools:
  - name: claude
    enabled: true
  - name: cursor
    enabled: true
```

`scribe tools` command:
- `scribe tools` — list detected tools, show enabled/disabled
- `scribe tools enable cursor` — enable a tool
- `scribe tools disable cursor` — disable a tool

Per-skill tool override happens at install time via `--tool` flag on `scribe sync` or through the list TUI action menu.

---

## 3. Manifest Rewrite: TOML → YAML

Incorporated from catalog-packages-design spec. Key changes:

### New format

File changes from `scribe.toml` to `scribe.yaml`.

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
```

### Structs

```go
type Manifest struct {
    APIVersion string   `yaml:"apiVersion"`
    Kind       string   `yaml:"kind"`       // "Registry" or "Package"
    Team       *Team    `yaml:"team"`
    Package    *Package `yaml:"package"`
    Catalog    []Entry  `yaml:"catalog"`
    Tools      *Tools   `yaml:"tools"`      // renamed from Targets
}

type Entry struct {
    Name        string `yaml:"name"`
    Source      string `yaml:"source"`
    Path        string `yaml:"path"`
    Type        string `yaml:"type"`        // "" or "package"
    Install     string `yaml:"install"`
    Update      string `yaml:"update"`
    Author      string `yaml:"author"`
    Description string `yaml:"description"`
    Timeout     int    `yaml:"timeout"`
}
```

Full struct details in catalog-packages-design spec. All validation rules from that spec apply.

### Config migration: TOML → YAML

`~/.scribe/config.toml` migrates to `~/.scribe/config.yaml`. Auto-migrated on first load — read TOML if YAML doesn't exist, write YAML, leave TOML as backup.

### New config structure

```yaml
tools:
  - name: claude
    enabled: true
  - name: cursor
    enabled: true

registries:
  - repo: Artistfy/hq
  - repo: anthropic/skills
    builtin: true
  - repo: openai/codex-skills
    builtin: true
  - repo: expo/skills
    builtin: true

# Token only if not using gh auth
token: ""
```

`registries` replaces `team_repos`. Each entry is a repo with optional metadata. The `builtin: true` flag marks default registries that shipped with Scribe (user can remove them).

Registry type is inferred on first connect, then cached in config (see Section 6 for cached fields: `type`, `writable`).

---

## 4. `scribe registry migrate`

Converts existing `scribe.toml` registries to `scribe.yaml`.

### Flow

1. Fetch `scribe.toml` from registry repo
2. Parse with TOML parser (kept in `internal/migrate/` only)
3. Convert to YAML `Manifest` struct
4. Infer author from `Maintainer()` logic. Flag org-name authors with `# TODO: verify author` comment
5. Present converted YAML to user for review
6. Push single commit: delete `scribe.toml`, create `scribe.yaml`
7. Catalog entries sorted alphabetically by name

### Legacy fallback

`scribe sync` and `scribe add` check for `scribe.yaml` first, fall back to `scribe.toml`. When falling back, emit warning: "This registry uses the legacy format. Run `scribe registry migrate` to upgrade."

---

## 5. Provider Adapters

Pluggable skill source abstraction. Scribe doesn't assume all skills come from GitHub repos with `scribe.yaml`.

### Interface

```go
// Provider knows how to discover and install skills from a source.
type Provider interface {
    // Discover returns available skills/entries from the source.
    Discover(ctx context.Context, repo string) ([]Entry, error)

    // Fetch downloads skill files for a single entry.
    Fetch(ctx context.Context, entry Entry) ([]File, error)
}
```

Providers handle **discovery and fetching only**. Writing to the canonical store, symlinking to tools, and state management remain in the sync engine — providers don't touch the local machine.

### Built-in providers

**GitHubProvider** (default): Uses GitHub API. Discovery chain when connecting to a repo:

1. **Has `scribe.yaml`?** → parse it, return catalog entries. Structured registry.
2. **Has legacy `scribe.toml`?** → parse it, return entries with legacy warning.
3. **Has `.claude-plugin/marketplace.json`?** → parse it, flatten plugins into entries. Each plugin's skills become individual entries. Plugin name preserved as `group` for display.
4. **Has SKILL.md files?** → tree scan via `GetTree`, return entries for each discovered skill. Author defaults to repo owner.
5. **Nothing found** → error: "No skills found in this repository."

Steps 1-4 are tried in order. First match wins, except: if step 3 finds a marketplace.json, also check for SKILL.md files outside plugin directories (some repos have both).

**skills.sh integration — discovery only, not installation.** All skills.sh registries are standard GitHub repos. Scribe uses `npx skills find` to search the skills.sh directory (discovery), then connects to found repos natively via GitHubProvider (installation/sync). No `npx skills add` or install adapter needed — only the search/discovery capability is used. Requires `npx` on PATH for `scribe add` search fallback; if unavailable, search is limited to connected registries only.

### Provider resolution

Default: GitHubProvider for all repos. Provider type is cached in config after first connect (see Section 6).

### Package location

`internal/provider/` with `provider.go` (interface), `github.go`.

---

## 6. Skill Identity & Community Registries

### Skill identity: namespaced by registry

Skills are namespaced by their source registry to prevent collisions. Two registries can both have a skill named `deploy` without conflict.

**State key format:** `registry-slug/skillname` (e.g., `Artistfy-hq/deploy`, `antfu-skills/deploy`). For unmanaged local skills, the key is `local/skillname`. The registry slug uses the same format as disk paths (owner/repo → owner-repo).

**CLI format:** Same as state keys. Users reference skills as `Artistfy-hq/recap` in commands like `scribe remove Artistfy-hq/recap`. This matches the disk layout (`~/.scribe/skills/Artistfy-hq/recap/`).

```go
// State key examples:
// "Artistfy-hq/recap"       — team registry skill
// "antfu-skills/nuxt"       — community registry skill
// "local:my-helper"         — unmanaged local skill

type State struct {
    LastSync  time.Time                `json:"last_sync,omitempty"`
    Installed map[string]InstalledSkill `json:"installed"`  // key = "registry-slug/name"
}
```

**Display:** `scribe list` shows just the skill name (grouped by registry), not the full key. The full key is visible in `--json` output and the detail panel.

**Disk layout:** The canonical store is namespaced by registry to prevent file-level collisions:
- `~/.scribe/skills/Artistfy-hq/recap/` (not `~/.scribe/skills/recap/`)
- `~/.scribe/skills/antfu-skills/nuxt/`
- `~/.scribe/skills/local/my-helper/` (unmanaged)

Registry names are slugified (`/` → `-`) for filesystem safety. Tool symlinks follow the same pattern:
- `~/.claude/skills/Artistfy-hq/recap` → `~/.scribe/skills/Artistfy-hq/recap/`

**Migration:** Existing state keys (bare names like `"recap"`) are migrated to `"registry:recap"` using `InstalledSkill.Registries[0]` as the registry prefix. If no registry is recorded, the key becomes `"local:recap"`.

### Registry type inference & caching

Scribe infers registry type from its contents on first connect, then **caches the result** in config to avoid repeated API calls:

| Repo contents | Type | Behavior |
|---------------|------|----------|
| `scribe.yaml` with `team:` block + user has write access | Team | Full read/write: sync, add, push |
| `scribe.yaml` with `team:` block + no write access | Community | Read-only: sync, list |
| `scribe.yaml` with `package:` block | Package | Read-only: sync, list |
| `.claude-plugin/marketplace.json` | Marketplace | Read-only: sync, list. Plugins flattened to entries |
| SKILL.md files (no manifest) | Community | Read-only: sync, list. Discovered via tree scan |

Write access check: `c.gh.Repositories.Get()` → `repo.GetPermissions()["push"]` (using go-github, not shelling out to `gh` CLI). Result cached in config alongside registry type.

### Config registry entry (with cached metadata)

```yaml
registries:
  - repo: Artistfy/hq
    enabled: true
    type: team           # cached: "team", "community", "marketplace", "package"
    writable: true       # cached: user has push access
  - repo: antfu/skills
    enabled: true
    type: community      # cached after first connect
    writable: false
```

Cache is refreshed on `scribe registry connect` (re-runs inference). Stale cache never causes errors — worst case, a user gets "read-only" for a repo they can actually write to, fixable by re-connecting.

### Connecting

`scribe registry connect` works the same for all types:

```bash
scribe registry connect Artistfy/hq          # team registry (writable)
scribe registry connect antfu/skills          # community (tree-scanned)
scribe registry connect expo/skills           # community (marketplace.json)
```

Validation step changes: instead of requiring `[team]` section, the provider's `Discover()` must return at least one entry. If zero entries, error: "No skills found in this repository."

### Flattening marketplace.json

When a repo has `.claude-plugin/marketplace.json`:

```json
{
  "name": "claude-skills",
  "plugins": [
    {
      "name": "cloudflare",
      "source": "./plugins/cloudflare",
      "skills": ["skills/workers", "skills/pages"]
    }
  ]
}
```

Each plugin's skills become individual `Entry` values:
- `Name`: skill directory name (e.g., "workers")
- `Source`: `github:owner/repo@HEAD`
- `Path`: full path within repo (e.g., `plugins/cloudflare/skills/workers`)
- `Author`: plugin author or repo owner
- `Group`: plugin name (e.g., "cloudflare") — for display grouping only

Group is a new display-only field on `Entry`. Not persisted in state.

---

## 7. Built-in Registries

Scribe ships with default registries enabled out of the box. Users can disable any of them.

### Defaults

| Registry | Reason |
|----------|--------|
| `anthropic/skills` | Official Claude skills |
| `openai/codex-skills` | Official OpenAI/Codex skills |
| `expo/skills` | Popular community registry with good marketplace.json |

These are added to config on first run (alongside tool detection). Marked with `builtin: true` so Scribe knows they were auto-added, not user-chosen.

### Managing built-ins

```bash
scribe registry disable anthropic/skills   # stop syncing
scribe registry enable anthropic/skills    # re-enable
scribe registry list                       # shows all, marks disabled
```

Disabled registries stay in config but are skipped during sync/list.

### Config representation

```yaml
registries:
  - repo: anthropic/skills
    builtin: true
    enabled: true
  - repo: openai/codex-skills
    builtin: true
    enabled: true
  - repo: expo/skills
    builtin: true
    enabled: true
  - repo: Artistfy/hq
    enabled: true
```

---

## 8. Package Sync: Install & Update

Incorporated from catalog-packages-design spec. Skills with `type: package` use declared `install`/`update` commands instead of Scribe's download-and-symlink.

### Install flow (not yet installed)

1. Hash `install + "\n" + update` (SHA-256)
2. TOFU trust check: prompt user to approve command + source
3. Execute install command with timeout (entry `timeout` or default 300s)
4. Capture stdout/stderr — don't let subprocess write to terminal in TUI mode
5. On success: record in state with commit SHA, command hash, approval
6. On failure: don't record. Emit error with captured stderr

### Update flow (already installed)

1. Fetch latest commit SHA from source
2. Same SHA → current, skip
3. Different → has `update` command? Execute it. No `update` command? Warn, skip.

### Trust state

New fields on `InstalledSkill`:

```go
Type       string    `json:"type,omitempty"`
InstallCmd string    `json:"install_cmd,omitempty"`
UpdateCmd  string    `json:"update_cmd,omitempty"`
CmdHash    string    `json:"cmd_hash,omitempty"`
Approval   string    `json:"approval,omitempty"`       // "approved" or "denied"
ApprovedAt time.Time `json:"approved_at,omitempty"`
```

Re-prompt when command hash changes in registry.

### Non-interactive mode

- `--trust-all` flag: approve all without prompting (CI)
- Without flag: skip packages needing approval, warn, continue with skills
- `--json` includes skipped packages with reason

### TOFU explained

Trust On First Use — like SSH fingerprints. First encounter: show the install command, ask user to approve. Hash and save the approval. Future syncs: if hash matches, run silently. If hash changes (command was modified in registry), re-prompt with old vs new. This prevents a compromised registry from silently injecting malicious install commands.

### Subprocess execution contract

All package install/update commands are executed via:
- **Shell:** `sh -c "<command>"` — enables pipes, tilde expansion, variable substitution
- **Working directory:** user's current working directory (`os.Getwd()`)
- **Environment:** inherited from parent process (full `PATH`, `HOME`, etc.)
- **Tilde:** expanded by `sh`, not by Go — commands using `~` work correctly
- **Timeout:** `context.WithTimeout` wrapping `exec.CommandContext`. On timeout, process group is killed via `syscall.Kill(-pid, syscall.SIGKILL)`
- **Output capture:** stdout and stderr captured via `cmd.CombinedOutput()` in TUI mode (prevents Bubble Tea corruption). In non-TTY mode, stderr is forwarded to the user's stderr for real-time visibility.

**Security note:** Install commands run with the user's full permissions. There is no sandboxing. This is the same trust model as Homebrew, npm scripts, and Makefile targets. The TOFU mechanism provides awareness, not isolation.

Full details in catalog-packages-design spec sections "Sync Engine Changes" and "Trust state."

---

## 9. Active Tools & Per-Skill Tool Management

### Auto-detection on first run

```go
func DetectTools() []Tool {
    var found []Tool
    for _, t := range AllTools() {
        if t.Detect() {
            found = append(found, t)
        }
    }
    return found
}
```

Tool detection checks:
- Claude: `~/.claude/` exists
- Cursor: `~/.cursor/` exists
- Additional tools added as the ecosystem grows

First run shows detected tools, confirms with user (TTY), saves to config.

### `scribe tools` command

```bash
scribe tools                    # list tools, show enabled/disabled
scribe tools enable windsurf    # enable a tool
scribe tools disable cursor     # disable a tool
```

### Per-skill tool override

Default: skills install to all enabled tools. Override via:

1. **At install time**: `scribe sync --tool claude` — only sync to Claude this run
2. **In list TUI**: action menu → "Change tools" → multi-select which tools get this skill
3. **In state**: `InstalledSkill.Tools` records which tools have this skill

When a skill's tools list differs from the global enabled tools, `scribe list` shows which tools it's installed to.

---

## 10. Reworked `scribe list`

> Note: `scribe list` is a top-level command, not behind a `skill` subcommand.

### Default: machine-first view

`scribe list` (no flags) shows **all skills on this machine**, grouped by source:

```
Artistfy/hq (team):
  recap         v1.0.0   krishan   ✓ current    claude, cursor
  deploy        v1.1.0   krishan   ↑ update     claude

antfu/skills:
  antfu         latest   antfu     ✓ current    claude
  nuxt          latest   antfu     ✓ current    claude

Local (unmanaged):
  my-helper     #a3f2c1b  you      —            claude
  quick-fix     #e7d4a2f  you      —            cursor
```

Each skill shows:
- **Name**
- **Version** (semver, branch@sha, or content hash)
- **Author** (from registry or "you" for local unmanaged)
- **Status**: ✓ current, ↑ update available, ! error
- **Tools**: which AI tools have this skill installed

### Grouping

Skills grouped by source registry. Within a registry, skills sorted alphabetically. Local unmanaged skills shown last under "Local (unmanaged)."

For marketplace repos, optionally sub-group by plugin name if the user connected via marketplace.json discovery.

### Remote view

`scribe list --remote` shows skills available in connected registries that are NOT yet installed. This is the old default behavior, now opt-in.

`scribe list --remote --registry antfu/skills` filters to one registry.

### Ownership

- **"you"**: skill exists locally, not in any registry, or authored by the authenticated GitHub user
- **Author name**: from registry catalog entry's `author` field
- **Repo owner**: fallback when no author field

### Sharing visibility

`scribe list --shared` or detail view in TUI shows which registries reference each skill:

```
recap:
  Shared with: Artistfy/hq, other-team/skills
  Author: krishan
  Version: v1.0.0
  Tools: claude, cursor
```

### JSON output

`scribe list --json` returns all the above as structured JSON. Includes `registries`, `author`, `tools`, `status` fields per skill.

---

## 11. Skill Ownership & Sharing

### Tracking authorship

The `author` field on catalog entries (from the manifest rewrite) tracks who created each skill. For unmanaged local skills, Scribe checks the authenticated GitHub username via `AuthenticatedUser()` and marks them as "you."

### Sharing view

`scribe list` shows which registries manage each skill via the grouping. The detail view (split-pane TUI or `--json`) shows the full list of registries that reference a skill.

### Adding skills to a registry (sharing with a team)

`scribe registry add` covers this. The workflow:
1. `scribe registry add` — interactive picker shows local skills
2. Select skills to share
3. Choose target registry (e.g., Artistfy/hq)
4. Skills are pushed to that registry's catalog

Author enforcement from catalog-packages-design spec applies: only the original author can update skill files in a registry.

---

## 12. `scribe upgrade`

Self-update command for all install methods.

### Detection

Scribe knows its own version (embedded at build time via `-ldflags`). Check GitHub Releases API for latest version.

```bash
scribe upgrade          # check and install latest
scribe upgrade --check  # just check, don't install
```

### Update strategies by install method

**Homebrew:**
```bash
brew upgrade Naoray/tap/scribe
```

**Go install:**
```bash
go install github.com/Naoray/scribe/cmd/scribe@latest
```

**Curl / direct binary:**
Download latest release binary from GitHub Releases, replace current binary in-place. Detect install method by checking:
1. `brew list scribe` succeeds → Homebrew
2. Binary path contains `go/bin` → Go install
3. Otherwise → direct binary replacement

### Implementation

```go
// internal/upgrade/upgrade.go
type Upgrader struct {
    CurrentVersion string
    Client         *github.Client
}

func (u *Upgrader) Check(ctx) (*Release, error)    // latest release info
func (u *Upgrader) Apply(ctx, release) error        // perform upgrade
```

### README update

Add "Updating" section to README:
```
scribe upgrade
```

One command. Works regardless of install method.

---

## 13. Bug Fix: README Sync

### Problem

`scribe sync` downloads README files from skill directories in the registry repo and writes them into the canonical store alongside SKILL.md. These READMEs then get symlinked into tool directories.

### Root cause

`FetchDirectory()` fetches all files in the skill's directory path recursively. There's no filter — everything comes down, including README.md, LICENSE, .gitignore, etc.

### Fix

Filter fetched files in the sync engine's apply phase. Only write files that are relevant to the skill:

**Deny list** (simpler than an allow list — skills can contain arbitrary supporting files):
- `README.md`, `README.*`
- `LICENSE`, `LICENSE.*`
- `.gitignore`, `.gitkeep`
- `.git/` (if somehow included)

Everything else is kept. This preserves SKILL.md, any tool-specific files (`.cursor.mdc`), supporting scripts, and assets that skills may reference.

Implementation: add a `shouldInclude(filename string) bool` filter function in `internal/sync/syncer.go`, called in `apply()` before `WriteToStore()`.

---

## 14. Scribe Agent Skill

A SKILL.md that teaches AI coding agents how to use Scribe. Managed by Scribe itself — installable and updatable through the registry.

### Location

Lives in the Scribe repo itself: `skills/scribe-agent/SKILL.md`

Published to a Scribe registry (e.g., `Naoray/scribe` or a dedicated `Naoray/scribe-skills` repo) so it can be installed via `scribe sync`.

### Content

The SKILL.md teaches agents:
- How to run `scribe list` to see installed skills
- How to run `scribe add` to find and install new skills
- How to run `scribe registry connect` to add registries
- How to run `scribe sync` to update skills
- How to run `scribe registry add` to share skills with a team
- How to run `scribe tools` to manage AI tool targets
- How to run `scribe upgrade` to update Scribe itself
- When to suggest these commands (e.g., "you don't have the deploy skill installed, want me to add it?")

### Self-referential install

The Scribe agent skill is one of the skills in a built-in registry. When a user runs `scribe sync` for the first time, this skill gets installed alongside other defaults — teaching their AI agent how to manage skills from that point on.

---

## 15. Config & State Changes Summary

### Config (`~/.scribe/config.yaml`, migrated from config.toml)

```yaml
tools:
  - name: claude
    enabled: true
  - name: cursor
    enabled: true

registries:
  - repo: Artistfy/hq
    enabled: true
    type: team
    writable: true
  - repo: anthropic/skills
    builtin: true
    enabled: true
    type: community
    writable: false
  - repo: openai/codex-skills
    builtin: true
    enabled: true
    type: community
    writable: false
  - repo: expo/skills
    builtin: true
    enabled: true
    type: marketplace
    writable: false

token: ""
```

### State (`~/.scribe/state.json`)

```go
type State struct {
    LastSync  time.Time                `json:"last_sync,omitempty"`
    Installed map[string]InstalledSkill `json:"installed"`
}

// Key format: "registry-slug/skillname" (e.g., "Artistfy-hq/recap", "local/my-helper")
// Each registry:skill combination is a unique entry — no Registries array needed.

type InstalledSkill struct {
    // Existing
    Version     string    `json:"version"`
    CommitSHA   string    `json:"commit_sha,omitempty"`
    Source      string    `json:"source"`
    InstalledAt time.Time `json:"installed_at"`
    Tools       []string  `json:"tools"`          // renamed from targets
    Paths       []string  `json:"paths"`

    // New — packages
    Type       string    `json:"type,omitempty"`
    InstallCmd string    `json:"install_cmd,omitempty"`
    UpdateCmd  string    `json:"update_cmd,omitempty"`
    CmdHash    string    `json:"cmd_hash,omitempty"`
    Approval   string    `json:"approval,omitempty"`
    ApprovedAt time.Time `json:"approved_at,omitempty"`

    // New — authorship
    Author string `json:"author,omitempty"`
}
```

`TeamState` removed. `LastSync` moves to top-level `State` (was `State.Team.LastSync`). Simplified — there's no "team" concept at the state level anymore, just registries.

---

## 16. Migration Strategy

All migrations are auto-applied on first load. No manual `scribe registry migrate-state` command needed. Each migration is idempotent — safe to run multiple times.

### Migration order

Migrations must be applied in this order (dependencies flow downward):

1. **Config: TOML → YAML** (no dependencies)
2. **State: field renames + structure changes** (no dependencies)
3. **Manifest: TOML → YAML** (separate — `scribe registry migrate` command, per-registry)

### Migration 1: Config (`config.toml` → `config.yaml`)

**Trigger:** `config.Load()` finds `config.toml` but no `config.yaml`.

**Transformation:**
```
BEFORE (config.toml):                    AFTER (config.yaml):
team_repos = ["Artistfy/hq"]    →       registries:
token = "ghp_..."                          - repo: Artistfy/hq
                                             enabled: true
                                           token: "ghp_..."
```

- Each `team_repos` entry becomes a `registries` entry with `enabled: true`
- Built-in registries are added with `builtin: true, enabled: true`
- `config.toml` is left as backup (not deleted)
- If both files exist, `config.yaml` wins

### Migration 2: State (`state.json` in-place)

**Trigger:** `state.Load()` detects old format (has `team` key or `targets` key on any skill).

**Transformations (applied sequentially):**

| Before | After | Logic |
|--------|-------|-------|
| `state.team.last_sync` | `state.last_sync` | Promote from nested to top-level |
| `state.team` | (removed) | Delete after promoting `last_sync` |
| `installed.X.targets` | `installed.X.tools` | Rename field |
| `installed["recap"]` | `installed["Artistfy-hq/recap"]` | Namespace key using slugified `skill.Registries[0]` or `"local"` |

**Implementation:** Use shadow structs for deserialization (same pattern as existing `rawConfig` in `config.go`):

```go
type rawState struct {
    Team      *rawTeamState              `json:"team"`
    LastSync  time.Time                  `json:"last_sync,omitempty"`
    Installed map[string]rawInstalledSkill `json:"installed"`
}

type rawInstalledSkill struct {
    // accepts both old and new field names
    Targets []string `json:"targets"`
    Tools   []string `json:"tools"`
    // ... all other fields
}
```

After deserialization, merge `Targets` into `Tools` (prefer `Tools` if both present), promote `Team.LastSync`, namespace keys, then save in new format.

### Migration 3: Manifest (`scribe.toml` → `scribe.yaml`)

**Trigger:** User runs `scribe registry migrate [registry]`. NOT auto-applied — this modifies a remote repo.

**Covered in Section 3.** This is the only migration that requires user action.

### Testing strategy

Write migration tests FIRST, before any implementation:
- Fixture: real `config.toml` from current install
- Fixture: real `state.json` with `team.last_sync`, `targets`, bare skill names
- Assert: round-trip through load → migrate → save → reload produces correct new format
- Assert: already-migrated files are not re-migrated (idempotent)
- Assert: both old and new format files coexisting resolves correctly

---

## 17. First-Run Setup

First-run detection triggers on the first *meaningful* command (`sync`, `connect`, `list`, `add`) — NOT on `--help` or `version`.

### Flow

1. Check if `config.yaml` exists
2. If not: run config migration (if `config.toml` exists) or create fresh config
3. Auto-detect tools via `DetectTools()`
4. Show detected tools, ask user to confirm (TTY) or accept defaults (non-TTY)
5. Add built-in registries (can be disabled later)
6. Save config
7. Continue with the original command

Subsequent runs skip setup entirely (config exists).

---

## 18. UX Principle: Loading Spinners

Every operation that makes the user wait MUST show a loading spinner with a descriptive message. No silent hangs.

Examples:
- `Connecting to Artistfy/hq...`
- `Syncing 3 registries...`
- `Discovering skills in antfu/skills...`
- `Installing gstack...`
- `Checking for updates...`

Use Bubble Tea's spinner component (already in the codebase) for TUI mode. For non-TTY mode, emit progress lines to stderr (`Syncing Artistfy/hq... done (3 skills)`).

---

## 19. Files Changed

| Area | Files | Change |
|------|-------|--------|
| **Tools** | `internal/tools/tool.go` | New: `Tool` interface with `Detect()`, `Uninstall()` |
| | `internal/tools/claude.go` | Renamed from targets, add `Detect()` |
| | `internal/tools/cursor.go` | Renamed from targets, add `Detect()` |
| | `internal/tools/store.go` | Move from targets |
| | `internal/tools/symlink.go` | Move from targets |
| **Provider** | `internal/provider/provider.go` | New: `Provider` interface |
| | `internal/provider/github.go` | New: GitHub discovery chain (yaml → marketplace.json → tree scan) |
| **Manifest** | `internal/manifest/manifest.go` | Rewrite: YAML structs, `[]Entry` catalog |
| | `internal/manifest/manifest_test.go` | Rewrite for YAML |
| **Marketplace** | `internal/provider/marketplace.go` | New: `.claude-plugin/marketplace.json` parser |
| **Config** | `internal/config/config.go` | Rewrite: YAML, registries list, tools list |
| **State** | `internal/state/state.go` | New fields, `TeamState` removed, `Targets` → `Tools` |
| **Sync** | `internal/sync/syncer.go` | Provider-based fetch, package install/update, file filter |
| | `internal/sync/compare.go` | Package SHA comparison |
| | `internal/sync/events.go` | Package events |
| **Migrate** | `internal/migrate/migrate.go` | New: TOML→YAML conversion |
| **Upgrade** | `internal/upgrade/upgrade.go` | New: self-update logic |
| **Discovery** | `internal/discovery/discovery.go` | `ReadSkillMeta` with YAML frontmatter |
| **Commands** | `cmd/migrate.go` | New command |
| | `cmd/upgrade.go` | New command |
| | `cmd/tools.go` | New command (replaces any target refs) |
| | `cmd/registry.go` | Extended: enable/disable, built-in management |
| | `cmd/connect.go` | Provider-aware, remove `[team]` requirement |
| | `cmd/list.go` | Machine-first default, remote flag, authorship display |
| | `cmd/list_tui.go` | Reworked grouping, ownership, tool display |
| | `cmd/sync.go` | Package prompts, tool filtering |
| | `cmd/add.go` | Author enforcement |
| **Skill** | `skills/scribe-agent/SKILL.md` | New: agent skill for managing Scribe |
| **Docs** | `README.md` | Update: upgrade instructions, new commands |

### Dependencies

**Added:**
- `gopkg.in/yaml.v3` — YAML parsing

**Removed:**
- `github.com/BurntSushi/toml` — replaced (kept in `internal/migrate/` temporarily)

---

## 20. Deferred Work

| Item | Reason |
|------|--------|
| Lockfile (`scribe.lock`) | Adds complexity without MVP value. Version pinning via `@ref` is sufficient. |
| `scribe push` | Author-side updates. `scribe registry add` covers the initial share workflow. |
| `scribe check` | Show updates without applying. `scribe list` shows update status already. |
| Global behavioral rules | Managing CLAUDE.md rule fragments (commit discipline, worktree conventions) as shareable artifacts. Distinct from project rules (`.claude/rules/`). |
| Rules sync across LLMs | Translating/distributing rules to each tool's format (CLAUDE.md, .cursorrules, GEMINI.md). |
| MCP server management | Related but separate concern. Tools section in config could grow to cover this. |
| `npx skills` CLI adapter | skills.sh repos are GitHub repos — covered natively by tree scan. Add adapter only if demand for skills.sh-specific install behavior. |
| `scribe init` (package author mode) | Scaffolding for new skill packages. Can use `npx skills init` as stopgap. |
| User-facing documentation site | Dedicated `docs/` with ASCII diagrams for TOFU trust model, registry architecture, etc. |

---

## 21. Documentation

The MVP README covers essential usage. A dedicated `docs/` folder is planned as a fast follow for:

- **TOFU trust model** — ASCII diagram showing the approve → hash → re-prompt flow
- **Registry architecture** — how team vs community vs marketplace registries work
- **Provider discovery chain** — the yaml → toml → marketplace.json → tree scan fallback
- **Tool management** — how skills get symlinked to different AI tools
- **Migration guide** — for existing users upgrading from TOML-era Scribe
