# scribe

Keep your team's AI coding agent skills in sync. One command, no copy-paste.

```bash
scribe connect ArtistfyHQ/team-skills
scribe sync
```

## What is this?

AI coding agents like Claude Code and Cursor work better with "skills" — markdown instruction files that teach the agent how to do specific tasks (code reviews, deployments, Laravel patterns, etc.). If you've built a good set of skills, sharing them with teammates currently means Slack links and manual file copying. Nobody knows if they're on the latest version. The person who just joined has no idea what they're missing.

Scribe fixes this. You put your team's skills in a GitHub repo with a `scribe.toml` manifest, teammates run `scribe connect`, and `scribe sync` keeps everyone up to date automatically. Works with Claude Code and Cursor from the same manifest.

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
scribe connect ArtistfyHQ/team-skills

# 2. That's it — skills are now installed and you stay in sync
scribe sync        # run again anytime to pick up new skills
scribe list        # see what's installed and what's outdated
```

### For the team lead setting up the shared repo

**Option A — Let Scribe scaffold it (recommended):**

```bash
scribe create registry
# Interactive prompts for team name, GitHub org, repo name, visibility
# Creates the repo, pushes scribe.toml + README, and connects automatically
```

**Option B — Manual setup:**

1. Create a GitHub repo (e.g., `ArtistfyHQ/team-skills`) — can be private
2. Create `scribe.toml` at the root:

```toml
[team]
name = "artistfy"
description = "Artistfy dev team skill stack"

[skills]
# External skills from other GitHub repos
"gstack" = { source = "github:garrytan/gstack@v0.12.9.0" }

# Skills you maintain directly in this repo
# Path format: github-username/skill-name
"deploy" = { source = "github:ArtistfyHQ/team-skills@main", path = "krishan/deploy" }
```

3. Add your skill files at the matching paths:

```
ArtistfyHQ/team-skills/
  scribe.toml
  krishan/
    deploy/
      SKILL.md       ← your skill file
```

4. Tell your teammate to run `scribe connect ArtistfyHQ/team-skills`

## Commands

| Command | What it does |
|---|---|
| `scribe connect <owner/repo>` | Connect to a team skills repo and run an initial sync |
| `scribe sync` | Install missing skills, update outdated ones |
| `scribe list` | Show all skills: what's installed, what's outdated, what's missing |
| `scribe add [name]` | Add a skill to the team registry (interactive picker or by name) |
| `scribe create registry` | Scaffold a new team skills registry on GitHub and connect to it |

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

done: 1 installed, 1 updated, 2 current, 0 failed
```

## Private skills

Private GitHub repos work if you're authenticated with the `gh` CLI — Scribe piggybacks on `gh auth token`. Nothing extra to configure.

```bash
gh auth login   # if not already done
```

Auth fallback chain: `gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.toml` → unauthenticated (public repos only).

## Agent-friendly

Scribe auto-detects non-TTY environments (CI, agent invocations). `--json` flag gives structured output:

```bash
scribe sync --json
scribe list --json
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
  config.toml    # which team repos you're connected to
  state.json     # what's installed, last sync time
  cache/         # cached GitHub downloads
```

## Requirements

- macOS or Linux (Windows not yet supported)
- GitHub account with access to your team skills repo
- `gh` CLI recommended for auth (not required for public repos)

## Status

**What works today:**
- `scribe sync` — full sync loop (diff, download, install, update)
- `scribe list` — shows installed skills and their status vs team loadout
- Claude Code and Cursor install targets
- Private repo support via `gh auth token`
- `--json` flag for CI/agent use

- `scribe connect` — connect to a team repo with interactive setup
- `scribe create registry` — scaffold a new team skills registry on GitHub
- `scribe add` — add local or remote skills to your team registry

**Coming later:**
- Lockfile (`scribe.lock`) — pin exact versions across the team
- `scribe publish` — publish skill packages

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
