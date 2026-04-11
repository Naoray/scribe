# Scribe Upgrade Command — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## Overview

Self-upgrade command that detects how scribe was installed and upgrades accordingly. One command, one job: `scribe upgrade`.

## Install Method Detection

`DetectMethod() Method` uses a hybrid approach — resolve symlinks, fast path heuristic on the resolved path, with a `brew list` fallback for ambiguous cases.

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
2. Resolve symlinks via `filepath.EvalSymlinks` to get the real binary location
3. Resolved path contains `/Cellar/`, `/opt/homebrew/`, or `/linuxbrew/` → `MethodHomebrew` (instant, no subprocess)
4. Resolved path contains `/go/bin/` → `MethodGoInstall`
5. Ambiguous path (e.g. `/usr/local/bin/scribe`) → run `brew list scribe`; if exit 0 → `MethodHomebrew` (swappable via `brewListCheck` var for tests)
6. Fallback → `MethodCurlBinary`

Homebrew tap exists at `github.com/naoray/homebrew-tap`.

## Version Comparison

- **Current version**: `cmd.Version` (set via goreleaser ldflags `-X github.com/Naoray/scribe/cmd.Version={{.Version}}`). Passed as a parameter from the command layer — no `CurrentVersion()` function in the upgrade package.
- **Latest version**: fetched via a new `LatestRelease(ctx, owner, repo)` method on the existing `internal/github` client, which wraps `go-github`'s `Repositories.GetLatestRelease`. Returns the tag name (e.g. `v0.5.0`).

**Normalization:** goreleaser's `{{.Version}}` strips the `v` prefix, so `cmd.Version` = `0.5.0` while GitHub tag = `v0.5.0`. Before comparison, strip the `v` prefix from the tag name (e.g. `strings.TrimPrefix(tag, "v")`). Compare the normalized strings.

**Special cases:**
- `cmd.Version == "dev"` → print "Running development build, skipping upgrade" and exit 0. Development builds should never attempt self-upgrade.

## Upgrade Execution

`Upgrade(ctx context.Context, method Method, latestVersion string, client *github.Client) error`

| Method | Action |
|---|---|
| `MethodHomebrew` | `exec.Command("brew", "upgrade", "scribe")` — capture stdout/stderr, surface brew's output on error |
| `MethodGoInstall` | `exec.Command("go", "install", "github.com/Naoray/scribe/cmd/scribe@latest")` — capture stdout/stderr |
| `MethodCurlBinary` | Download release asset, verify checksum, extract, atomic replace |

### Binary Self-Upgrade (MethodCurlBinary)

1. Determine asset name: `scribe_{runtime.GOOS}_{runtime.GOARCH}.tar.gz` (matches goreleaser `name_template`). If no asset matches the current OS/arch, return a clear "no binary available for your platform" error.
2. Download the asset from the GitHub release
3. Download `checksums.txt` from the same release. Verify SHA256 of the downloaded archive against the checksum. Abort on mismatch.
4. Decompress + extract the `scribe` binary in-memory from the tar.gz stream. Safety guards:
   - Accept only a single file entry named `scribe`
   - Reject entries with path traversal (`../`)
   - Cap decompressed size at 100MB (sanity limit)
5. Write extracted binary to a temp file in `filepath.Dir(executablePath)` — **same directory as the target**, guaranteeing same-filesystem for atomic rename
6. `os.Stat` the current executable to capture file permissions
7. `os.Chmod` the **temp file** to match original permissions — **before rename**, not after, to eliminate the race window
8. Atomic `os.Rename` from temp file to current executable path
9. If rename fails with permission error → return error suggesting `sudo`

**Non-goals:**
- Windows support — goreleaser only builds linux/darwin. Guard against `runtime.GOOS == "windows"` with a clear error message.
- Signature verification — future consideration, not v1.

## Package Boundary

`internal/upgrade/` is **UI-agnostic** — no `fmt.Print`, no direct output. Returns values and errors only. All user-facing output lives in `cmd/upgrade.go`.

## GitHub Client Addition

Add to `internal/github/client.go`:

```go
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, error)
```

Thin wrapper around `c.gh.Repositories.GetLatestRelease()`. Follows existing method patterns and reuses the auth chain.

## Command: `cmd/upgrade.go`

`newUpgradeCommand() *cobra.Command` registered in `cmd/root.go` init alongside existing commands. Also add `"upgrade"` to the first-run skip list in `PersistentPreRunE` (alongside `help`, `version`, `migrate`).

**Flow:**

1. If `cmd.Version == "dev"` → print "Running development build, skipping upgrade" → exit 0
2. Detect install method → print `Installed via: homebrew` / `go install` / `direct binary`
3. Fetch latest version via GitHub API
4. Normalize both versions (strip `v` prefix) and compare
5. If equal → print `Already up to date (vX.Y.Z)` → exit 0
6. Print `Upgrading vCurrent → vLatest...`
7. TTY: show braille spinner during upgrade (reuse pattern from `cmd/explain.go`). Non-TTY: plain text, no spinner.
8. On success → `Successfully upgraded to vLatest`
9. On error → print error, exit 1

## Tests: `internal/upgrade/upgrade_test.go`

Table-driven tests with swappable package-level vars.

### DetectMethod tests

| Executable path | brewListCheck | Expected |
|---|---|---|
| `/opt/homebrew/bin/scribe` | n/a | `MethodHomebrew` |
| `/usr/local/Cellar/scribe/0.5.0/bin/scribe` | n/a | `MethodHomebrew` |
| `/home/linuxbrew/.linuxbrew/bin/scribe` | n/a | `MethodHomebrew` |
| `/home/user/go/bin/scribe` | n/a | `MethodGoInstall` |
| `/usr/local/bin/scribe` | returns true | `MethodHomebrew` |
| `/usr/local/bin/scribe` | returns false | `MethodCurlBinary` |
| `/tmp/scribe` | n/a | `MethodCurlBinary` |

### LatestVersion test

`httptest` server returning mock GitHub release JSON. Verify tag name extraction.

### Binary extraction roundtrip test

1. Create a tar.gz archive containing a known binary in `t.TempDir()`
2. Call the extraction function
3. Verify output file contents match the original
4. Verify file permissions are preserved

### Tar safety tests

- Reject archive with path traversal entry (`../evil`)
- Reject archive with multiple entries
- Reject archive exceeding size cap

### Version comparison tests

| Current | Latest tag | Expected |
|---|---|---|
| `dev` | `v0.5.0` | skip upgrade |
| `0.5.0` | `v0.5.0` | already up to date |
| `0.4.0` | `v0.5.0` | proceed with upgrade |

## Constraints

- No `--force` flag, no `--check` flag. Minimal: one command, one job.
- No Bubble Tea TUI. Simple sequential output.
- Uses `internal/github` client for all API calls (respects auth chain).
- Windows is an explicit non-goal (no goreleaser builds). Guard with clear error.
