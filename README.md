# scribe

Keep your team's AI coding agent skills in sync. One command, no copy-paste.

```bash
scribe sync
```

## The problem

If you run a curated set of Claude Code skills, sharing them with teammates means Slack links and manual file copying. Nobody knows if they're on the latest version. The person who just joined has no idea what they're missing.

Scribe fixes this with a shared `scribe.toml` in a GitHub repo. Point your teammates at it once, and they stay in sync.

## How it works

A shared `scribe.toml` in a GitHub repo defines the team's skill stack. Anyone can contribute a skill — each person's skills live in their own folder, so it's clear who owns what:

```
ArtistfyHQ/team-skills/
  scribe.toml
  krishan/
    deploy/
      SKILL.md
    code-review/
      SKILL.md
  markus/
    frontend-prs/
      SKILL.md
```

```toml
# ArtistfyHQ/team-skills/scribe.toml
[team]
name = "artistfy"

[skills]
# External packages
"gstack"       = { source = "github:garrytan/gstack@v0.12.9.0" }
"laravel-init" = { source = "github:Naoray/scribe-skills@v1.0.0", path = "skills/laravel-init" }

# Team-authored skills — path implies maintainer
"deploy"        = { source = "github:ArtistfyHQ/team-skills@main", path = "krishan/deploy" }
"code-review"   = { source = "github:ArtistfyHQ/team-skills@main", path = "krishan/code-review" }
"frontend-prs"  = { source = "github:ArtistfyHQ/team-skills@main", path = "markus/frontend-prs" }
```

Your teammates connect once:

```bash
scribe init
# Enter your team's skills repo: ArtistfyHQ/team-skills
```

From then on, `scribe sync` diffs their local setup against the loadout and installs what's missing or outdated. Works with Claude Code and Cursor from the same manifest.

## Commands

```bash
scribe init                                         # Connect to your team repo (or scaffold a package)
scribe sync                                         # Install missing, update outdated
scribe list                                         # See what's installed vs what the team has
scribe add gstack                                   # Add an installed skill to the team loadout
scribe add github:garrytan/gstack@v0.12.9.0         # Add by source directly
```

`scribe add` puts your skill in your `/{github-username}/` folder automatically and registers it in `scribe.toml`. If two people add a skill with the same name, the first registration wins in `scribe list`.

`scribe list` shows the full picture including who maintains each skill:

```
Team: artistfy (ArtistfyHQ/team-skills) · Last sync: 2 hours ago

SKILL                  VERSION         STATUS      TARGETS         MAINTAINER
gstack                 v0.12.9.0       ✓ current   claude          garrytan
laravel-init           v1.0.0          ⬆ outdated  claude, cursor  Naoray
deploy                 main@a3f2c1b    ✓ current   claude          krishan
code-review            v1.0.0          ✓ current   claude          krishan
frontend-prs           main@c9f1d2e    ● missing   claude          markus
my-custom-skill        —               ◇ extra     claude          —

Summary: 3 current · 1 outdated · 1 missing · 1 extra
Run `scribe sync` to install missing and update outdated skills.
```

The maintainer is inferred from the folder path — no extra config needed. For external packages it comes from the GitHub repo owner. A good team-skills repo pairs this with a [CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners) file so PRs touching someone's folder go to them for review.

## Agent-friendly

Scribe auto-detects non-TTY environments — so it works fine when Claude itself runs it. The `--json` flag gives structured output for scripting and CI.

There's a `/scribe-sync` Claude Code skill for hands-free syncing from within a session. (Coming soon.)

## Private skills

Private GitHub repos work out of the box if you're already authenticated with the `gh` CLI. Scribe piggybacks on `gh auth token` so there's nothing extra to configure.

Auth chain: `gh auth token` → `GITHUB_TOKEN` env var → `~/.scribe/config.toml` → unauthenticated (public repos only).

## Install

```bash
go install github.com/Naoray/scribe@latest
```

Pre-built binaries in [Releases](https://github.com/Naoray/scribe/releases) for macOS (arm64, amd64) and Linux (amd64).

## Status

Early. The core sync loop is being built now. What works today: project scaffolded, CLI skeleton with all four commands, Bubbletea TUI wired up. What's next: manifest parsing, GitHub fetcher, install targets (Claude Code + Cursor), state tracking.

Lockfile, `scribe add`, publish workflow, and search come after the sync loop is solid.

## Skill format

Scribe follows the [agentskills.io](https://agentskills.io) SKILL.md specification. Any skill that works with `skills.sh` or Paks will work with Scribe.

## Requirements

- Go 1.22+
- GitHub account with access to your team skills repo
- `gh` CLI recommended (for auth), not required
