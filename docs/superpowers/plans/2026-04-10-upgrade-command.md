# Upgrade Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `scribe upgrade` — a self-upgrade command that detects install method and upgrades accordingly.

**Architecture:** UI-agnostic `internal/upgrade/` package handles detection, version fetching, and upgrade execution. `cmd/upgrade.go` handles all user-facing output with TTY-aware spinner. GitHub client gets one new method.

**Tech Stack:** Go, Cobra, go-github/v69, tar/gzip stdlib, os/exec

**Spec:** `docs/superpowers/specs/2026-04-10-upgrade-command-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/upgrade/upgrade.go` | Types, detection, version fetch, upgrade execution |
| Create | `internal/upgrade/upgrade_test.go` | Table-driven tests for all upgrade logic |
| Modify | `internal/github/client.go` | Add `LatestRelease` method |
| Create | `cmd/upgrade.go` | Cobra command, TTY output, spinner |
| Modify | `cmd/root.go:27` | Add `"upgrade"` to first-run skip list |
| Modify | `cmd/root.go:92-95` | Register `newUpgradeCommand()` |

---

### Task 1: Add `LatestRelease` to GitHub client

**Files:**
- Modify: `internal/github/client.go:287` (after `GetTree` method)

- [ ] **Step 1: Write the method**

Add after the `HasPushAccess` method at the end of the file (before `wrapErr`):

```go
// LatestRelease fetches the latest published release for a repository.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, error) {
	release, _, err := c.gh.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("latest release %s/%s", owner, repo))
	}
	return release, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/github/...`
Expected: Clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/github/client.go
git commit -m "[agent] feat: add LatestRelease method to GitHub client

Step 1 of task: upgrade command"
```

---

### Task 2: Create `internal/upgrade/upgrade.go` — types and detection

**Files:**
- Create: `internal/upgrade/upgrade.go`

- [ ] **Step 1: Write the DetectMethod tests**

Create `internal/upgrade/upgrade_test.go`:

```go
package upgrade

import (
	"testing"
)

func TestDetectMethod(t *testing.T) {
	tests := []struct {
		name           string
		executablePath string
		evalSymlinks   string // resolved path (empty = same as executablePath)
		brewListOK     bool
		want           Method
	}{
		{
			name:           "homebrew via /opt/homebrew",
			executablePath: "/opt/homebrew/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via Cellar",
			executablePath: "/usr/local/Cellar/scribe/0.5.0/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via linuxbrew",
			executablePath: "/home/linuxbrew/.linuxbrew/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via symlink resolution",
			executablePath: "/usr/local/bin/scribe",
			evalSymlinks:   "/opt/homebrew/Cellar/scribe/0.5.0/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "go install",
			executablePath: "/home/user/go/bin/scribe",
			want:           MethodGoInstall,
		},
		{
			name:           "ambiguous path, brew list succeeds",
			executablePath: "/usr/local/bin/scribe",
			brewListOK:     true,
			want:           MethodHomebrew,
		},
		{
			name:           "ambiguous path, brew list fails",
			executablePath: "/usr/local/bin/scribe",
			brewListOK:     false,
			want:           MethodCurlBinary,
		},
		{
			name:           "tmp path, fallback to curl binary",
			executablePath: "/tmp/scribe",
			want:           MethodCurlBinary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origExec := executablePath
			origSymlinks := evalSymlinks
			origBrew := brewListCheck
			t.Cleanup(func() {
				executablePath = origExec
				evalSymlinks = origSymlinks
				brewListCheck = origBrew
			})

			executablePath = func() (string, error) {
				return tt.executablePath, nil
			}

			resolved := tt.evalSymlinks
			if resolved == "" {
				resolved = tt.executablePath
			}
			evalSymlinks = func(path string) (string, error) {
				return resolved, nil
			}

			brewListCheck = func(name string) bool {
				return tt.brewListOK
			}

			got := DetectMethod()
			if got != tt.want {
				t.Errorf("DetectMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/upgrade/... -run TestDetectMethod -v`
Expected: Compilation failure — `upgrade` package doesn't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/upgrade/upgrade.go`:

```go
package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Method represents how scribe was installed.
type Method int

const (
	MethodHomebrew  Method = iota
	MethodGoInstall
	MethodCurlBinary
)

// String returns a human-readable label for the install method.
func (m Method) String() string {
	switch m {
	case MethodHomebrew:
		return "homebrew"
	case MethodGoInstall:
		return "go install"
	case MethodCurlBinary:
		return "direct binary"
	default:
		return "unknown"
	}
}

// Swappable for testing.
var (
	executablePath = os.Executable
	evalSymlinks   = filepath.EvalSymlinks
	brewListCheck  = defaultBrewListCheck
)

func defaultBrewListCheck(name string) bool {
	return exec.Command("brew", "list", name).Run() == nil
}

// DetectMethod determines how scribe was installed by inspecting the
// executable path. Uses path heuristics first (instant), falls back
// to `brew list` for ambiguous paths.
func DetectMethod() Method {
	exePath, err := executablePath()
	if err != nil {
		return MethodCurlBinary
	}

	resolved, err := evalSymlinks(exePath)
	if err != nil {
		resolved = exePath
	}

	if isHomebrewPath(resolved) {
		return MethodHomebrew
	}
	if strings.Contains(resolved, "/go/bin/") {
		return MethodGoInstall
	}

	// Ambiguous path — ask Homebrew directly.
	if brewListCheck("scribe") {
		return MethodHomebrew
	}

	return MethodCurlBinary
}

func isHomebrewPath(path string) bool {
	return strings.Contains(path, "/Cellar/") ||
		strings.Contains(path, "/opt/homebrew/") ||
		strings.Contains(path, "/linuxbrew/")
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/upgrade/... -run TestDetectMethod -v`
Expected: All 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/upgrade/upgrade.go internal/upgrade/upgrade_test.go
git commit -m "[agent] feat: add install method detection with hybrid heuristic

Step 2 of task: upgrade command"
```

---

### Task 3: Add version comparison logic

**Files:**
- Modify: `internal/upgrade/upgrade.go`
- Modify: `internal/upgrade/upgrade_test.go`

- [ ] **Step 1: Write the version comparison tests**

Append to `internal/upgrade/upgrade_test.go`:

```go
func TestNeedsUpgrade(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		latestTag  string
		wantSkip   bool // dev build — skip entirely
		wantUpgrade bool
	}{
		{
			name:       "dev build skips upgrade",
			current:    "dev",
			latestTag:  "v0.5.0",
			wantSkip:   true,
			wantUpgrade: false,
		},
		{
			name:       "same version, no upgrade",
			current:    "0.5.0",
			latestTag:  "v0.5.0",
			wantSkip:   false,
			wantUpgrade: false,
		},
		{
			name:       "older version, needs upgrade",
			current:    "0.4.0",
			latestTag:  "v0.5.0",
			wantSkip:   false,
			wantUpgrade: true,
		},
		{
			name:       "tag without v prefix",
			current:    "0.5.0",
			latestTag:  "0.5.0",
			wantSkip:   false,
			wantUpgrade: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, upgrade := NeedsUpgrade(tt.current, tt.latestTag)
			if skip != tt.wantSkip {
				t.Errorf("NeedsUpgrade() skip = %v, want %v", skip, tt.wantSkip)
			}
			if upgrade != tt.wantUpgrade {
				t.Errorf("NeedsUpgrade() upgrade = %v, want %v", upgrade, tt.wantUpgrade)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/upgrade/... -run TestNeedsUpgrade -v`
Expected: Compilation failure — `NeedsUpgrade` not defined.

- [ ] **Step 3: Write the implementation**

Add to `internal/upgrade/upgrade.go`:

```go
// NeedsUpgrade compares the current version against the latest release tag.
// Returns (isDevBuild, needsUpgrade).
// Dev builds return (true, false) — caller should skip upgrade.
// Equal versions return (false, false).
// Different versions return (false, true).
func NeedsUpgrade(current, latestTag string) (isDevBuild bool, needsUpgrade bool) {
	if current == "dev" || current == "" {
		return true, false
	}
	latest := strings.TrimPrefix(latestTag, "v")
	return false, current != latest
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/upgrade/... -run TestNeedsUpgrade -v`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/upgrade/upgrade.go internal/upgrade/upgrade_test.go
git commit -m "[agent] feat: add version comparison with dev build detection

Step 3 of task: upgrade command"
```

---

### Task 4: Add tar extraction with safety guards

**Files:**
- Modify: `internal/upgrade/upgrade.go`
- Modify: `internal/upgrade/upgrade_test.go`

- [ ] **Step 1: Write the extraction and safety tests**

Append to `internal/upgrade/upgrade_test.go`:

```go
import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func createTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.Name,
			Size: int64(len(e.Content)),
			Mode: 0755,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.Content); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

type tarEntry struct {
	Name    string
	Content []byte
}

func TestExtractBinary(t *testing.T) {
	binaryContent := []byte("#!/bin/fake-scribe")

	t.Run("valid single-entry archive", func(t *testing.T) {
		archive := createTarGz(t, []tarEntry{
			{Name: "scribe", Content: binaryContent},
		})
		got, err := ExtractBinary(bytes.NewReader(archive), "scribe")
		if err != nil {
			t.Fatalf("ExtractBinary() error = %v", err)
		}
		if !bytes.Equal(got, binaryContent) {
			t.Errorf("content mismatch: got %q, want %q", got, binaryContent)
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		archive := createTarGz(t, []tarEntry{
			{Name: "../evil", Content: []byte("bad")},
		})
		_, err := ExtractBinary(bytes.NewReader(archive), "scribe")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("rejects multiple entries", func(t *testing.T) {
		archive := createTarGz(t, []tarEntry{
			{Name: "scribe", Content: binaryContent},
			{Name: "extra", Content: []byte("extra")},
		})
		_, err := ExtractBinary(bytes.NewReader(archive), "scribe")
		if err == nil {
			t.Fatal("expected error for multiple entries, got nil")
		}
	})

	t.Run("rejects missing target binary", func(t *testing.T) {
		archive := createTarGz(t, []tarEntry{
			{Name: "wrong-name", Content: binaryContent},
		})
		_, err := ExtractBinary(bytes.NewReader(archive), "scribe")
		if err == nil {
			t.Fatal("expected error for missing binary, got nil")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/upgrade/... -run TestExtractBinary -v`
Expected: Compilation failure — `ExtractBinary` not defined.

- [ ] **Step 3: Write the implementation**

Add to `internal/upgrade/upgrade.go` (add the imports at the top: `"archive/tar"`, `"compress/gzip"`, `"fmt"`, `"io"`):

```go
const maxDecompressedSize = 100 * 1024 * 1024 // 100MB

// ExtractBinary decompresses a tar.gz stream and extracts a single binary
// named binaryName. Returns the binary contents.
//
// Safety guards:
//   - Rejects entries with path traversal (../)
//   - Accepts only a single file entry named binaryName
//   - Caps decompressed size at 100MB
func ExtractBinary(r io.Reader, binaryName string) ([]byte, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var found bool
	var content []byte

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		// Safety: reject path traversal.
		if strings.Contains(hdr.Name, "..") {
			return nil, fmt.Errorf("archive contains path traversal: %s", hdr.Name)
		}

		// Safety: reject multiple entries.
		if found {
			return nil, fmt.Errorf("archive contains multiple entries, expected only %q", binaryName)
		}

		// Only accept the target binary.
		name := filepath.Base(hdr.Name)
		if name != binaryName {
			return nil, fmt.Errorf("archive contains %q, expected %q", hdr.Name, binaryName)
		}

		// Safety: cap decompressed size.
		limited := io.LimitReader(tr, maxDecompressedSize+1)
		content, err = io.ReadAll(limited)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", binaryName, err)
		}
		if len(content) > maxDecompressedSize {
			return nil, fmt.Errorf("binary %s exceeds %d byte limit", binaryName, maxDecompressedSize)
		}
		found = true
	}

	if !found {
		return nil, fmt.Errorf("binary %q not found in archive", binaryName)
	}
	return content, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/upgrade/... -run TestExtractBinary -v`
Expected: All 4 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/upgrade/upgrade.go internal/upgrade/upgrade_test.go
git commit -m "[agent] feat: add tar.gz extraction with safety guards

Step 4 of task: upgrade command"
```

---

### Task 5: Add upgrade execution logic

**Files:**
- Modify: `internal/upgrade/upgrade.go`
- Modify: `internal/upgrade/upgrade_test.go`

- [ ] **Step 1: Write the binary replacement test**

Append to `internal/upgrade/upgrade_test.go`:

```go
func TestReplaceBinary(t *testing.T) {
	t.Run("replaces binary with correct content and permissions", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "scribe")

		// Create a "current" binary with specific permissions.
		if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
			t.Fatal(err)
		}

		newContent := []byte("new-binary-content")
		if err := ReplaceBinary(target, newContent); err != nil {
			t.Fatalf("ReplaceBinary() error = %v", err)
		}

		// Verify content.
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, newContent) {
			t.Errorf("content = %q, want %q", got, newContent)
		}

		// Verify permissions preserved.
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0755 {
			t.Errorf("permissions = %v, want 0755", info.Mode().Perm())
		}
	})

	t.Run("error on nonexistent target", func(t *testing.T) {
		err := ReplaceBinary("/nonexistent/path/scribe", []byte("data"))
		if err == nil {
			t.Fatal("expected error for nonexistent target")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/upgrade/... -run TestReplaceBinary -v`
Expected: Compilation failure — `ReplaceBinary` not defined.

- [ ] **Step 3: Write the upgrade functions**

Add to `internal/upgrade/upgrade.go` (add imports: `"bytes"`, `"context"`, `"crypto/sha256"`, `"encoding/hex"`, `"os/exec"`, `"path/filepath"`, `"runtime"`; add `github.com/google/go-github/v69/github`):

```go
// ReplaceBinary atomically replaces the binary at targetPath with newContent.
// Writes to a temp file in the same directory, sets permissions to match
// the existing binary, then renames.
func ReplaceBinary(targetPath string, newContent []byte) error {
	info, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".scribe-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on any error path.
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(newContent); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Set permissions before rename to eliminate race window.
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied replacing %s — try running with sudo: %w", targetPath, err)
		}
		return fmt.Errorf("rename %s → %s: %w", tmpPath, targetPath, err)
	}

	tmpPath = "" // prevent deferred cleanup
	return nil
}

// UpgradeHomebrew runs `brew upgrade scribe`.
func UpgradeHomebrew(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "brew", "upgrade", "scribe")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("brew upgrade: %s: %w", string(out), err)
	}
	return out, nil
}

// UpgradeGoInstall runs `go install github.com/Naoray/scribe/cmd/scribe@latest`.
func UpgradeGoInstall(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "install", "github.com/Naoray/scribe/cmd/scribe@latest")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("go install: %s: %w", string(out), err)
	}
	return out, nil
}

// UpgradeBinary downloads the release asset, verifies its checksum, extracts
// the binary, and atomically replaces the current executable.
func UpgradeBinary(ctx context.Context, release *github.RepositoryRelease, ghClient interface {
	DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, error)
}) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-upgrade is not supported on Windows — download manually from GitHub Releases")
	}

	exePath, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	resolved, err := evalSymlinks(exePath)
	if err != nil {
		resolved = exePath
	}

	assetName := fmt.Sprintf("scribe_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	checksumName := "checksums.txt"

	var assetID, checksumID int64
	for _, a := range release.Assets {
		switch a.GetName() {
		case assetName:
			assetID = a.GetID()
		case checksumName:
			checksumID = a.GetID()
		}
	}
	if assetID == 0 {
		return fmt.Errorf("no release asset %q found for %s/%s", assetName, runtime.GOOS, runtime.GOARCH)
	}
	if checksumID == 0 {
		return fmt.Errorf("no checksums.txt found in release")
	}

	// Download checksum file.
	checksumRC, err := ghClient.DownloadReleaseAsset(ctx, "Naoray", "scribe", checksumID)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	checksumData, err := io.ReadAll(checksumRC)
	checksumRC.Close()
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	expectedHash, err := findChecksum(checksumData, assetName)
	if err != nil {
		return err
	}

	// Download the archive.
	assetRC, err := ghClient.DownloadReleaseAsset(ctx, "Naoray", "scribe", assetID)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	assetData, err := io.ReadAll(assetRC)
	assetRC.Close()
	if err != nil {
		return fmt.Errorf("read asset: %w", err)
	}

	// Verify checksum.
	actualHash := sha256sum(assetData)
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expectedHash, actualHash)
	}

	// Extract binary.
	content, err := ExtractBinary(bytes.NewReader(assetData), "scribe")
	if err != nil {
		return err
	}

	return ReplaceBinary(resolved, content)
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// findChecksum parses a goreleaser checksums.txt and returns the SHA256
// for the named asset. Format: "<hash>  <filename>\n"
func findChecksum(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found in checksums.txt", assetName)
}
```

- [ ] **Step 4: Add the `bytes` import and the `DownloadReleaseAsset` wrapper to the GitHub client**

Add to `internal/github/client.go` after the `LatestRelease` method:

```go
// DownloadReleaseAsset downloads a release asset by ID, following redirects.
func (c *Client) DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, error) {
	rc, redirectURL, err := c.gh.Repositories.DownloadReleaseAsset(ctx, owner, repo, id, http.DefaultClient)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("download asset %d from %s/%s", id, owner, repo))
	}
	if redirectURL != "" {
		resp, err := http.Get(redirectURL)
		if err != nil {
			return nil, fmt.Errorf("follow redirect for asset %d: %w", id, err)
		}
		return resp.Body, nil
	}
	return rc, nil
}
```

Add `"io"` to the imports in `client.go` if not already present.

- [ ] **Step 5: Verify it compiles**

Run: `go build ./internal/upgrade/... && go build ./internal/github/...`
Expected: Clean build.

- [ ] **Step 6: Run all upgrade tests**

Run: `go test ./internal/upgrade/... -v`
Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/upgrade/upgrade.go internal/upgrade/upgrade_test.go internal/github/client.go
git commit -m "[agent] feat: add upgrade execution — homebrew, go install, binary replace

Step 5 of task: upgrade command"
```

---

### Task 6: Create `cmd/upgrade.go` — Cobra command

**Files:**
- Create: `cmd/upgrade.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Create the command file**

Create `cmd/upgrade.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"os"

	gogithub "github.com/google/go-github/v69/github"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/upgrade"
)

func newUpgradeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade scribe to the latest version",
		Long:  "Detects how scribe was installed and upgrades using the appropriate method.",
		Args:  cobra.NoArgs,
		RunE:  runUpgrade,
	}
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Dev builds should not attempt self-upgrade.
	isDevBuild, _ := upgrade.NeedsUpgrade(Version, "")
	if isDevBuild {
		fmt.Println("Running development build, skipping upgrade.")
		return nil
	}

	// Detect install method.
	method := upgrade.DetectMethod()
	fmt.Printf("Installed via: %s\n", method)

	// Fetch latest release.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := github.NewClient(ctx, cfg.Token)
	release, err := client.LatestRelease(ctx, "Naoray", "scribe")
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	latestTag := release.GetTagName()
	_, needsUpgrade := upgrade.NeedsUpgrade(Version, latestTag)
	if !needsUpgrade {
		fmt.Printf("Already up to date (%s)\n", latestTag)
		return nil
	}

	fmt.Printf("Upgrading v%s → %s...\n", Version, latestTag)

	isTTY := isatty.IsTerminal(os.Stdout.Fd())
	return doUpgrade(ctx, method, release, client, isTTY)
}

func doUpgrade(ctx context.Context, method upgrade.Method, release *gogithub.RepositoryRelease, client *github.Client, isTTY bool) error {
	var spin *spinState
	if isTTY {
		spin = startSpinner(os.Stdout, "Downloading and installing...")
	}

	var upgradeErr error
	switch method {
	case upgrade.MethodHomebrew:
		if spin != nil {
			spin.stop()
		}
		// Brew has its own progress output — don't wrap with spinner.
		_, upgradeErr = upgrade.UpgradeHomebrew(ctx)
	case upgrade.MethodGoInstall:
		_, upgradeErr = upgrade.UpgradeGoInstall(ctx)
		if spin != nil {
			spin.stop()
		}
	case upgrade.MethodCurlBinary:
		upgradeErr = upgrade.UpgradeBinary(ctx, release, client)
		if spin != nil {
			spin.stop()
		}
	}

	if upgradeErr != nil {
		return fmt.Errorf("upgrade failed: %w", upgradeErr)
	}

	fmt.Printf("Successfully upgraded to %s\n", release.GetTagName())
	return nil
}
```

Note: this reuses the `spinState` and `startSpinner` already defined in `cmd/explain.go`.

- [ ] **Step 2: Add `"upgrade"` to first-run skip list in `cmd/root.go`**

In `cmd/root.go`, change line 27:

```go
if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "migrate" {
```

to:

```go
if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "migrate" || cmd.Name() == "upgrade" {
```

- [ ] **Step 3: Register the command in `cmd/root.go`**

In `cmd/root.go`, change lines 92-95:

```go
	// Other.
	rootCmd.AddCommand(
		newCreateCommand(),
		newExplainCommand(),
	)
```

to:

```go
	// Other.
	rootCmd.AddCommand(
		newCreateCommand(),
		newExplainCommand(),
		newUpgradeCommand(),
	)
```

- [ ] **Step 4: Fix the `DownloadReleaseAsset` interface usage**

The `UpgradeBinary` function in `internal/upgrade/upgrade.go` takes a `ghClient` interface. The `github.Client` in `internal/github/client.go` needs to satisfy that interface. Since `doUpgrade` passes `client *github.Client`, the `DownloadReleaseAsset` method signature must match.

Update the `UpgradeBinary` function signature in `internal/upgrade/upgrade.go` to use a concrete approach — accept the download function directly:

Replace the `UpgradeBinary` function signature and first section:

```go
// AssetDownloader downloads a release asset by ID.
type AssetDownloader interface {
	DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, error)
}

// UpgradeBinary downloads the release asset, verifies its checksum, extracts
// the binary, and atomically replaces the current executable.
func UpgradeBinary(ctx context.Context, release *github.RepositoryRelease, downloader AssetDownloader) error {
```

Then replace all `ghClient.DownloadReleaseAsset(` calls with `downloader.DownloadReleaseAsset(`.

- [ ] **Step 5: Verify it compiles**

Run: `go build ./cmd/scribe/...`
Expected: Clean build.

- [ ] **Step 6: Test the help output**

Run: `go run ./cmd/scribe upgrade --help`
Expected: Shows usage text with "Upgrade scribe to the latest version".

- [ ] **Step 7: Commit**

```bash
git add cmd/upgrade.go cmd/root.go internal/upgrade/upgrade.go
git commit -m "[agent] feat: add scribe upgrade command with TTY spinner

Step 6 of task: upgrade command"
```

---

### Task 7: Verify full build and test suite

**Files:** None new — verification only.

- [ ] **Step 1: Run full build**

Run: `go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: All tests PASS, including the new upgrade tests.

- [ ] **Step 3: Run upgrade help**

Run: `go run ./cmd/scribe upgrade --help`
Expected: Shows command help.

- [ ] **Step 4: Run upgrade with dev version**

Run: `go run ./cmd/scribe upgrade`
Expected: "Running development build, skipping upgrade."

- [ ] **Step 5: Final commit if any fixes were needed**

Only if tests revealed issues that needed fixing. Otherwise skip.
