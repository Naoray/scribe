# Scribe

Team skill sync CLI for AI coding agents. Go + Cobra + Charm (Bubble Tea).

## Project

- **Module**: `github.com/Naoray/scribe`
- **Binary**: `cmd/scribe/main.go`
- **Go version**: 1.26.1

## Architecture

```
cmd/                    # Cobra commands (connect, sync, list, add, create, guide)
internal/
  config/               # config.toml loading (~/.scribe/config.toml)
  manifest/             # scribe.toml parsing (BurntSushi/toml)
  github/               # GitHub API client (go-github + oauth2)
  targets/              # Install target writers (claude, cursor)
  state/                # ~/.scribe/state.json management
  sync/                 # Sync algorithm — UI-agnostic, emits tea.Msg events
  prereq/               # Prerequisite checker (auth, dir, connections)
  ui/                   # Bubbletea TUI layer (sync progress, styles)
```

## North Star

**Convenience first, tech debt second.** When facing implementation choices, always ask: "which makes Scribe more convenient for the person running it?" Ship usable > ship perfect.

## Key Conventions

- Core packages (`sync/`, `state/`, `github/`) are **UI-agnostic** — they emit events, never print
- TUI (`internal/ui/`) is a pure presentation layer consuming those events
- Non-TTY auto-detected: when stdout is not a terminal, fall back to plain line output
- `--json` flag available on `sync`, `list`, and `guide` for CI/agent use
- GitHub auth chain: `gh auth token` → `GITHUB_TOKEN` env → `~/.scribe/config.toml` → unauthenticated

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
  config.toml     # user preferences
```
