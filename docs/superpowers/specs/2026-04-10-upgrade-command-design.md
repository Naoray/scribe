# Scribe Upgrade Command — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## Overview

Self-upgrade command that detects how scribe was installed and upgrades accordingly. One command, one job: `scribe upgrade`.

## Install Method Detection

`DetectMethod() Method` uses a hybrid approach — fast path heuristic on the executable path, with a `brew list` fallback for ambiguous cases.

```go
type Method int
const (
    MethodHomebrew  Method = iota
    MethodGoInstall
    MethodCurlBinary
)
```

**Detection order:**

1. Resolve executable path via `os.Executable()` (swappable in tests via package-level `executablePath` var)
2. Path contains `/Cellar/` or `/opt/homebrew/` → `MethodHomebrew` (instant, no subprocess)
3. Path contains `/go/bin/` → `MethodGoInstall`
4. Ambiguous path (e.g. `/usr/local/bin/scribe`) → run `brew list scribe`; if exit 0 → `MethodHomebrew` (swappable via `brewListCheck` var for tests)
5. Fallback → `MethodCurlBinary`

Homebrew tap exists at `github.com/naoray/homebrew-tap`.

## Version Comparison

- **Current version**: `cmd.Version` (set via goreleaser ldflags `-X github.com/Naoray/scribe/cmd.Version={{.Version}}`). Passed as a parameter from the command layer — no `CurrentVersion()` function in the upgrade package.
- **Latest version**: fetched via a new `LatestRelease(ctx, owner, repo)` method on the existing `internal/github` client, which wraps `go-github`'s `Repositories.GetLatestRelease`. Returns the tag name (e.g. `v0.5.0`).

## Upgrade Execution

`Upgrade(ctx context.Context, method Method, latestVersion string, client *github.Client) error`

| Method | Action |
|---|---|
| `MethodHomebrew` | `exec brew upgrade scribe` |
| `MethodGoInstall` | `exec go install github.com/Naoray/scribe/cmd/scribe@latest` |
| `MethodCurlBinary` | Download release asset, extract, atomic replace |

### Binary Self-Upgrade (MethodCurlBinary)

1. Determine asset name: `scribe_{runtime.GOOS}_{runtime.GOARCH}.tar.gz` (matches goreleaser `name_template`)
2. Download the asset from the GitHub release
3. Decompress + extract the `scribe` binary in-memory from the tar.gz stream
4. Write to a temp file in `os.TempDir()`
5. `os.Stat` the current executable to capture file permissions
6. Atomic `os.Rename` from temp file to current executable path
7. `os.Chmod` to restore original permissions
8. If rename fails with permission error → return error suggesting `sudo`

## Package Boundary

`internal/upgrade/` is **UI-agnostic** — no `fmt.Print`, no direct output. Returns values and errors only. All user-facing output lives in `cmd/upgrade.go`.

## GitHub Client Addition

Add to `internal/github/client.go`:

```go
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, error)
```

Thin wrapper around `c.gh.Repositories.GetLatestRelease()`. Follows existing method patterns and reuses the auth chain.

## Command: `cmd/upgrade.go`

`newUpgradeCommand() *cobra.Command` registered in `cmd/root.go` init alongside existing commands.

**Flow:**

1. Detect install method → print `Installed via: homebrew` / `go install` / `direct binary`
2. Fetch latest version via GitHub API
3. Compare with `cmd.Version`
4. If equal → print `Already up to date (vX.Y.Z)` → exit 0
5. Print `Upgrading vCurrent → vLatest...`
6. TTY: show braille spinner during upgrade (reuse pattern from `cmd/explain.go`). Non-TTY: plain text, no spinner.
7. On success → `Successfully upgraded to vLatest`
8. On error → print error, exit 1

## Tests: `internal/upgrade/upgrade_test.go`

Table-driven tests with swappable package-level vars.

### DetectMethod tests

| Executable path | brewListCheck | Expected |
|---|---|---|
| `/opt/homebrew/bin/scribe` | n/a | `MethodHomebrew` |
| `/usr/local/Cellar/scribe/0.5.0/bin/scribe` | n/a | `MethodHomebrew` |
| `/home/user/go/bin/scribe` | n/a | `MethodGoInstall` |
| `/usr/local/bin/scribe` | returns true | `MethodHomebrew` |
| `/usr/local/bin/scribe` | returns false | `MethodCurlBinary` |
| `/tmp/scribe` | n/a | `MethodCurlBinary` |

### LatestVersion test

`httptest` server returning mock GitHub release JSON. Verify tag name extraction.

## Constraints

- No `--force` flag, no `--check` flag. Minimal: one command, one job.
- No Bubble Tea TUI. Simple sequential output.
- Uses `internal/github` client for all API calls (respects auth chain).
