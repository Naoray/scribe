# scribe

Your team's AI coding skills, always in sync. One manifest, one command.

```bash
brew install Naoray/tap/scribe
scribe registry connect ArtistfyHQ/team-skills
scribe sync
```

---

AI coding agents like Claude Code, Cursor, and Codex work better when you teach them how your team works — code review style, deployment checklists, Laravel patterns. These "skills" are markdown instruction files that live in `~/.claude/skills/` and similar dirs.

The problem: once you've built them, sharing means Slack links and manual copying. New teammates have no idea what exists. Skills get stale without anyone noticing.

Scribe fixes this. Put your team's skills in a GitHub repo with a `scribe.yaml` manifest. Teammates run `scribe registry connect`. Then `scribe sync` keeps every machine current — canonical store, tool-facing installs, and adoption of any hand-rolled skills already on the machine. One manifest works across Claude Code, Codex, and Cursor.

## Install

**Homebrew (recommended):**

```bash
brew install Naoray/tap/scribe
```

**Binary (macOS / Linux):**

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

**Go:**

```bash
go install github.com/Naoray/scribe/cmd/scribe@latest
```

Verify: `scribe --version`

**Updating:**

```bash
brew upgrade scribe                          # Homebrew
go install github.com/Naoray/scribe/cmd/scribe@latest   # Go
# Binary: replace from the releases page
```

## Quickstart

### Joining a team that already uses Scribe

```bash
scribe registry connect ArtistfyHQ/team-skills   # connect once
scribe sync                                        # install everything
scribe list                                        # verify
```

Run `scribe sync` again anytime to pick up new skills.

### Setting up a shared registry

**Let Scribe scaffold it:**

```bash
scribe registry create
# Prompts for team name, GitHub org, repo name, visibility
# Creates the repo, pushes scribe.yaml + README, and connects automatically
```

**Manual setup:**

1. Create a GitHub repo (can be private)
2. Add `scribe.yaml` at the root:

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

3. Add skill files at the matching paths:

```
ArtistfyHQ/team-skills/
  scribe.yaml
  krishan/
    deploy/
      SKILL.md
```

4. Share the repo name — teammates run `scribe registry connect ArtistfyHQ/team-skills`

## Let your agent set it up

Paste this into any agent with shell access (Claude Code, Cursor, Codex, etc.) to install Scribe on the machine without touching a terminal yourself:

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

## Commands

### Daily use

| Command | What it does |
|---|---|
| `scribe` | Open the local skill manager |
| `scribe list` | Show all skills on this machine (managed + unmanaged) |
| `scribe add [query]` | Find and install skills from registries |
| `scribe adopt [name]` | Import hand-rolled skills from `~/.claude/skills` etc. into the store |
| `scribe remove <skill>` | Remove a skill from this machine |
| `scribe sync` | Reconcile local skill state, tool installs, and connected registries |
| `scribe doctor` | Inspect managed skill health, detect drift |
| `scribe skill repair <skill> --tool <tool>` | Resolve drift when a tool-local copy diverges from the store |
| `scribe status` | Show connected registries, installed count, and last sync |
| `scribe tools` | List detected AI tools, enable/disable |
| `scribe explain <skill>` | AI-powered skill explanation (or `--raw` for rendered SKILL.md) |

### Registry management

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

### What `scribe list` looks like

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

- `✓ current` — matches team version
- `⬆ outdated` — installed, team has a newer version
- `● missing` — in team registry, not installed locally
- `◇ extra` — installed locally, not in team registry (informational, never auto-removed)

### What `scribe sync` looks like

```
syncing ArtistfyHQ/team-skills...

  gstack               ok (v0.12.9.0)
  laravel-init         updated to v1.1.0
  deploy               ok (main@a3f2c1b)
  frontend-prs         installed main@c9f1d2e
repaired 1 tool installs

done: 1 installed, 1 updated, 2 current, 0 failed
```

When sync finds divergent content in a managed tool path, it preserves the content and prints a repair hint rather than overwriting:

```
conflict: recap in codex differs from managed copy
run `scribe skill repair recap --tool codex` to resolve
```

## Adoption — claim skills you already have

If you've been hand-rolling skills in `~/.claude/skills/` or `~/.codex/skills/`, Scribe adopts them into the canonical store on first sync. The original path becomes a symlink — nothing moves, everything still works, and now Scribe manages it.

```bash
scribe adopt                 # interactive: review conflicts, adopt clean candidates
scribe adopt --yes           # auto-adopt all clean candidates
scribe adopt --dry-run       # preview the plan, no writes
scribe adopt <name>          # adopt a single named skill
```

Adopted skills appear with `(local)` in `scribe list`. They have no upstream and stay until you `scribe remove` them.

### Adoption mode

`scribe sync` runs adoption as a prelude. Configure it via `~/.scribe/config.yaml`:

```yaml
adoption:
  mode: auto       # auto | prompt | off
  paths:           # optional extra dirs beyond the defaults
    - ~/src/my-skills
```

- `auto` (default) — silently adopt clean candidates on every sync
- `prompt` — defer to `scribe adopt`; interactive prompts only when you run the dedicated command
- `off` — never adopt automatically; unmanaged skills still appear in `scribe list`

Or manage without touching YAML:

```bash
scribe config adoption                          # show current settings
scribe config adoption --mode off
scribe config adoption --add-path ~/src/my-skills
scribe config adoption --remove-path ~/src/my-skills
```

## Private skills

Private GitHub repos work automatically if you're authenticated with the `gh` CLI:

```bash
gh auth login   # if not already done
```

Auth fallback chain: `gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.yaml` → unauthenticated (public repos only).

## CI and agent use

Scribe auto-detects non-TTY environments. Use `--json` for structured output in CI pipelines or agent scripts:

```bash
scribe list --json
scribe status --json
scribe add --json skillname --yes
```

Example output:

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

## Health checks

`scribe doctor` inspects managed skill health and reports drift between the canonical store and tool-facing projections:

```bash
scribe doctor               # check all managed skills
scribe doctor --skill recap  # check a single skill
scribe doctor --json         # machine-readable output
```

## Skill format

Scribe follows the [agentskills.io](https://agentskills.io) SKILL.md specification. Any skill that works with `skills.sh` or Paks works with Scribe. A skill is a directory with a `SKILL.md` at the root — frontmatter declares the name, description, and compatible tools.

## Data stored locally

```
~/.scribe/
  config.yaml    # connected registries, tool settings
  state.json     # what's installed, last sync time
  skills/        # canonical skill store (symlinked by tools)
```

## Requirements

- macOS or Linux
- GitHub account with access to your team skills repo
- `gh` CLI recommended for auth (not required for public repos)

## Contributing

See the [open issues](https://github.com/Naoray/scribe/issues) — each one has enough context to pick up and run with.

```bash
git clone https://github.com/Naoray/scribe
cd scribe
go build ./...
go run ./cmd/scribe --help
go test ./...
```
