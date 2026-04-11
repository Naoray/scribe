# Repository Guidelines

## Project Structure & Module Organization

`cmd/` contains CLI entrypoints and Bubble Tea TUIs, including the main binary at `cmd/scribe`. Core application logic lives under `internal/`, split by concern such as `internal/workflow`, `internal/sync`, `internal/state`, `internal/tools`, and `internal/provider`. User-facing docs and design notes live in `docs/`. Sample or local agent assets may exist under `agents/`, but production code should stay in `cmd/` or `internal/`.

## Build, Test, and Development Commands

- `go build ./...` builds all packages and catches compile-time regressions.
- `go test ./...` runs the full test suite.
- `go test ./internal/workflow ./internal/sync ./cmd` is a good focused pass for command and sync changes.
- `go run ./cmd/scribe --help` runs the CLI locally.
- `go run ./cmd/scribe list --json` is a quick smoke test for non-TTY output.

## Coding Style & Naming Conventions

Use standard Go formatting and keep code `gofmt`-clean. Prefer small functions, explicit error handling, and package-local helpers over cross-package shortcuts. Keep package names lowercase and file names descriptive, for example `list_tui.go` or `syncer.go`. Cobra commands should follow the existing `newXCommand` and `runX` pattern. Tests should sit next to the code they cover.

## Testing Guidelines

Write table-driven Go tests where practical. Name tests with `TestXxx` and keep them close to the changed package. Cover behavior changes, not just happy paths; TUI and workflow regressions should include command-path tests when possible. Run `go test ./...` before committing.

## Commit & Pull Request Guidelines

Follow the repo’s history: short imperative commit subjects such as `fix: preserve tools on list TUI updates` or `perf: lazy-init command dependencies`. Prefer focused commits over mixed refactors. PRs should explain the user-visible change, note risks or migration impact, and include terminal output or screenshots when changing TUI behavior.

## Agent-Specific Instructions

Do not read from or modify files outside this repository worktree unless the user explicitly asks for it. Treat external paths such as `~/.scribe/` as off-limits by default.
