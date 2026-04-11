# Scribe

Team skill sync CLI for AI coding agents. Go + Cobra + Charm (Bubble Tea).

## Project

- **Module**: `github.com/Naoray/scribe`
- **Binary**: `cmd/scribe/main.go`
- **Go version**: 1.26.1

## Architecture

```
cmd/                    # Cobra commands (add, list, remove, sync, tools, guide, create, explain, registry)
internal/
  add/                  # Add workflow — local/remote discovery, GitHub push
  config/               # config.yaml loading (~/.scribe/config.yaml), legacy config.toml migration
  discovery/            # On-disk skill discovery, YAML frontmatter parsing, content hashing
  firstrun/             # First-run experience and onboarding
  github/               # GitHub API client (go-github + oauth2)
  logo/                 # ASCII logo rendering with lipgloss gradient (width-adaptive, NO_COLOR safe)
  manifest/             # scribe.yaml parsing (gopkg.in/yaml.v3), legacy scribe.toml fallback
  migrate/              # TOML → YAML manifest conversion
  paths/                # XDG-style path helpers (~/.scribe/)
  prereq/               # Prerequisite checks (gh CLI availability)
  provider/             # Provider abstraction — GitHubProvider, marketplace.json, tree scan
  state/                # ~/.scribe/state.json management
  sync/                 # Sync algorithm — UI-agnostic, emits tea.Msg events
  tools/                # Install target writers (claude, cursor)
  scaffold/             # Registry scaffolding (scribe create registry)
  workflow/             # Step-sequence engine: Runner, Bag, Formatter, per-command steps
```

## North Star

**Convenience first, tech debt second.** When facing implementation choices, always ask: "which makes Scribe more convenient for the person running it?" Ship usable > ship perfect.

**The best-designed CLI tool.** Every output should be visually pleasing, intuitively structured, and delightful to use. Color-coded status indicators, styled headers, aligned columns, helpful summaries. If a user screenshots our output, it should look good enough to share. Aim for the most adorable, polished CLI experience in the ecosystem.

**Show exactly what's useful, nothing more.** Every line of output should earn its place. Default to the minimum information needed to understand what happened and what to do next. Hide details behind flags (`--verbose`, `--json`) for users who want them. When in doubt, leave it out. An overwhelmed user is a lost user.

## Key Conventions

- Core packages (`sync/`, `add/`, `state/`, `github/`) are **UI-agnostic** — they emit events, never print
- TUI models live in `cmd/` (e.g. `add_tui.go`) as pure presentation consuming those events
- Non-TTY auto-detected: when stdout is not a terminal, fall back to plain line output
- `--json` flag available on `sync`, `list`, and `add` for CI/agent use
- GitHub auth chain: `gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.yaml` → unauthenticated

## Build

```bash
go build ./...
go run ./cmd/scribe --help
```

## Data Directories

```
~/.scribe/
  state.json      # installed packages + team connection
  skills/         # canonical skill store (symlinked by targets)
  config.yaml     # user preferences (tool settings, registries)
```
