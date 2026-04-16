# scribe

Keep your team's AI coding agent skills in sync. One command, no copy-paste.

```bash
scribe registry connect ArtistfyHQ/team-skills
scribe sync
```

## Set up in one paste

Paste this into any agent with shell access (Claude Code, Cursor, Codex, etc.) to set up Scribe on the machine:

```
I want to use Scribe to manage my AI coding-agent skills on this machine.
Repo: https://github.com/Naoray/scribe  (setup steps: /blob/main/SKILL.md)

Please set it up for me:
  1. If `scribe --version` fails, install it. Prefer methods that land the binary on PATH:
       a. Homebrew: `brew install Naoray/tap/scribe`
       b. Release binary to /usr/local/bin, e.g. Apple Silicon:
          `curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_arm64.tar.gz | tar xz && sudo mv scribe /usr/local/bin/`
          (swap the asset for your OS/arch — see https://github.com/Naoray/scribe/releases/latest)
       c. Last resort: `go install github.com/Naoray/scribe/cmd/scribe@latest`
          (lands in ~/go/bin which is often NOT on PATH — only use if a and b aren't available,
          and add the go bin dir to PATH afterwards)
  2. Register Scribe's own agent-facing skill locally so future sessions pick it up from
     `~/.scribe/skills/scribe-agent/`:
        scribe add Naoray/scribe:scribe-agent --yes --json
  3. Show me `scribe list --json` to confirm.
```

That's it — two commands wrapped in a readable request. You're telling the agent what you want installed and from where; the agent stays in charge of running the commands.

## What is this?

AI coding agents like Claude Code and Cursor work better with "skills" — markdown instruction files that teach the agent how to do specific tasks (code reviews, deployments, Laravel patterns, etc.). If you've built a good set of skills, sharing them with teammates currently means Slack links and manual file copying. Nobody knows if they're on the latest version. The person who just joined has no idea what they're missing.

Scribe fixes this. You put your team's skills in a GitHub repo with a `scribe.yaml` manifest (or legacy `scribe.toml`), teammates run `scribe registry connect`, and `scribe sync` keeps the whole machine healthy: canonical store, adopted local skills, and tool-facing installs. Works with Claude Code, Codex, and Cursor from the same manifest.

## Install

**macOS / Linux — download the binary:**

```bash
# macOS (Apple Silicon)
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_arm64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_amd64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_linux_amd64.tar.gz | tar xz
sudo mv scribe /usr/local/bin/
```

**Homebrew (macOS) — recommended:**

```bash
brew install Naoray/tap/scribe
```

**Go users:**

```bash
go install github.com/Naoray/scribe/cmd/scribe@latest
```

Verify: `scribe --version`

### Updating

**Homebrew:**

```bash
brew upgrade scribe
```

**Go:**

```bash
go install github.com/Naoray/scribe/cmd/scribe@latest
```

**Binary:** Download the latest release from the [releases page](https://github.com/Naoray/scribe/releases) and replace the binary in `/usr/local/bin/`.

## Quickstart

### For a teammate joining your team

```bash
# 1. Connect to your team's skills repo (one time)
scribe registry connect ArtistfyHQ/team-skills

# 2. That's it — skills are now installed and you stay in sync
scribe sync        # run again anytime to pick up new skills
scribe list        # see what's installed
```

### For the team lead setting up the shared repo

**Option A — Let Scribe scaffold it (recommended):**

```bash
scribe registry create
# Interactive prompts for team name, GitHub org, repo name, visibility
# Creates the repo, pushes scribe.yaml + README, and connects automatically
```

**Option B — Manual setup:**

1. Create a GitHub repo (e.g., `ArtistfyHQ/team-skills`) — can be private
2. Create `scribe.yaml` at the root:

```yaml
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
  description: Artistfy dev team skill stack
catalog:
  - name: gstack
    source: "github:garrytan/gstack@v0.12.9.0"
    author: garrytan
  - name: deploy
    source: "github:ArtistfyHQ/team-skills@main"
    path: krishan/deploy
    author: krishan
```

3. Add your skill files at the matching paths:

```
ArtistfyHQ/team-skills/
  scribe.yaml
  krishan/
    deploy/
      SKILL.md       ← your skill file
```

4. Tell your teammate to run `scribe registry connect ArtistfyHQ/team-skills`

## Commands

### Daily Use

| Command | What it does |
|---|---|
| `scribe` | Open the local skill manager |
| `scribe list` | Show all skills on this machine (managed + unmanaged) |
| `scribe add [query]` | Find and install skills from registries |
| `scribe adopt [name]` | Import hand-rolled skills from `~/.claude/skills` etc. into the store |
| `scribe remove <skill>` | Remove a skill from this machine |
| `scribe sync` | Reconcile local skill state, tool installs, and connected registries |
| `scribe doctor` | Inspect managed skills and projections for repairable issues |
| `scribe doctor --fix` | Normalize canonical skill metadata and repair affected projections |
| `scribe doctor --skill recap --fix` | Repair a single managed skill and its projections |
| `scribe skill repair <skill> --tool <tool>` | Resolve preserved managed drift when a tool-local copy differs from the canonical store |
| `scribe status` | Show connected registries, installed count, and last sync |
| `scribe tools` | List detected AI tools, enable/disable |
| `scribe explain <skill>` | AI-powered skill explanation (or `--raw` for rendered SKILL.md) |

### Registry Management

| Command | What it does |
|---|---|
| `scribe registry connect <repo>` | Connect to a skill registry |
| `scribe registry create` | Scaffold a new registry repo on GitHub |
| `scribe registry add [name]` | Share a local skill to a registry |
| `scribe registry list` | Show connected registries with skill counts |
| `scribe registry enable/disable` | Enable or disable a connected registry |
| `scribe registry migrate` | Convert a `scribe.toml` registry to `scribe.yaml` |

### Other

| Command | What it does |
|---|---|
| `scribe guide` | Interactive setup guide |
| `scribe --version` | Show version |

### scribe list output

```
Team: artistfy (ArtistfyHQ/team-skills) · Last sync: 2 hours ago

SKILL              VERSION         STATUS       TARGETS
gstack             v0.12.9.0       ✓ current    claude
laravel-init       v1.0.0          ⬆ outdated   claude, cursor
deploy             main@a3f2c1b    ✓ current    claude
frontend-prs       main@c9f1d2e    ● missing    claude
my-old-skill       —               ◇ extra      claude

3 current · 1 outdated · 1 missing · 1 extra
```

Status meanings:
- `✓ current` — installed, matches team version
- `⬆ outdated` — installed, but team has a newer version
- `● missing` — in team loadout, not yet installed locally
- `◇ extra` — installed locally but not in the team loadout (informational, never auto-removed)

### scribe sync output

```
syncing ArtistfyHQ/team-skills...

  gstack               ok (v0.12.9.0)
  laravel-init         updated to v1.1.0
  deploy               ok (main@a3f2c1b)
  frontend-prs         installed main@c9f1d2e
repaired 1 tool installs

done: 1 installed, 1 updated, 2 current, 0 failed
```

If sync finds divergent content in a managed tool path, it preserves that content and prints a repair hint instead of overwriting it:

```bash
conflict: recap in codex differs from managed copy
run `scribe skill repair recap --tool codex` to resolve
```

### `scribe doctor` v1 scope

```bash
scribe doctor
scribe doctor --fix
scribe doctor --skill recap --fix
```

`scribe doctor` audits managed skills for canonical `SKILL.md` metadata issues and projection drift.
`scribe doctor --fix` applies safe metadata normalization and then repairs affected tool projections.

`scribe doctor` v1 does not attempt to rewrite mixed package layouts for Codex.
It focuses on canonical metadata health plus projection repair.

## Adoption — claim skills you already have

If you've been hand-rolling skills in `~/.claude/skills/` or `~/.codex/skills/`, Scribe adopts them into the canonical store on first sync so the rest of the commands (list, remove, tools) work on them unmodified. Adoption is install-first and reversible: the original path becomes a symlink into `~/.scribe/skills/<name>/`.

```bash
scribe adopt                 # interactive: review conflicts, adopt clean candidates
scribe adopt --yes           # force auto (adopt all clean, skip conflicts)
scribe adopt --dry-run       # print the plan, no writes
scribe adopt <name>          # adopt a single named candidate
```

Adopted skills are marked with `(local)` in `scribe list` and carry `origin: "local"` in `--json` output. They have no upstream and stay put until you `scribe remove` them.

### Adoption modes

`scribe sync` runs adoption as a prelude. Control it via `config.yaml`:

```yaml
adoption:
  mode: auto       # auto | prompt | off
  paths:           # optional: extra dirs beyond the builtins
    - ~/src/my-skills
```

- `auto` (default) — adopt clean candidates silently on every sync.
- `prompt` — `scribe sync` defers to `scribe adopt`; the dedicated command owns interactive prompts.
- `off` — never adopt automatically; unmanaged skills still show up in `scribe list`.

Manage the config without editing YAML by hand:

```bash
scribe config adoption                       # show current settings
scribe config adoption --mode off
scribe config adoption --add-path ~/src/my-skills
scribe config adoption --remove-path ~/src/my-skills
```

First-run always prompts once, regardless of the persisted mode — that's the only moment a brand-new user can't know they need to check.

## Private skills

Private GitHub repos work if you're authenticated with the `gh` CLI — Scribe piggybacks on `gh auth token`. Nothing extra to configure.

```bash
gh auth login   # if not already done
```

Auth fallback chain: `gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.yaml` → unauthenticated (public repos only).

## Agent-friendly

Scribe auto-detects non-TTY environments (CI, agent invocations). `--json` flag gives structured output:

```bash
scribe --json
scribe list --json
scribe status --json
scribe add --json skillname --yes
```

Output:
```json
{
  "team_repos": ["ArtistfyHQ/team-skills"],
  "skills": [
    { "name": "gstack", "action": "skipped", "status": "current", "version": "v0.12.9.0" },
    { "name": "laravel-init", "action": "updated", "version": "v1.1.0" }
  ],
  "summary": { "installed": 0, "updated": 1, "skipped": 1, "failed": 0 }
}
```

## Skill format

Scribe follows the [agentskills.io](https://agentskills.io) SKILL.md specification. Any skill that works with `skills.sh` or Paks will work with Scribe. A skill is a directory with a `SKILL.md` at the root — the frontmatter tells Scribe the name, description, and compatible tools.

## Data stored locally

```
~/.scribe/
  config.yaml    # connected registries, tool settings
  state.json     # what's installed, last sync time
  skills/        # canonical skill store (symlinked by tools)
```

## Requirements

- macOS or Linux (Windows not yet supported)
- GitHub account with access to your team skills repo
- `gh` CLI recommended for auth (not required for public repos)

## Status

**What works today:**
- `scribe sync` — full sync loop (diff, download, install, update)
- `scribe list` — TUI for browsing installed skills, grouped by registry, with managed/unmanaged markers
- `scribe add` — browse and install skills from connected registries
- `scribe adopt` — import hand-rolled skills from tool dirs into the canonical store
- `scribe remove` — uninstall skills from the machine
- `scribe tools` — list and toggle AI tools (Claude Code, Cursor)
- `scribe registry connect/create/add` — manage team registries
- Private repo support via `gh auth token`
- `--json` flag on all commands for CI/agent use

**Coming later:**
- `scribe upgrade` — self-update command
- Lockfile (`scribe.lock`) — pin exact versions across the team

## Contributing

The project is early. See the [open issues](https://github.com/Naoray/scribe/issues) for what's planned — each one has enough context to pick up and run with.

```bash
git clone https://github.com/Naoray/scribe
cd scribe
go build ./...
go run ./cmd/scribe --help
```

Tests:
```bash
go test ./...
```
