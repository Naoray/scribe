package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/google/go-github/v69/github"
)

// Method represents how scribe was installed.
type Method int

const (
	MethodHomebrew Method = iota
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

// ReplaceBinary atomically replaces the binary at targetPath with newContent.
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

// UpgradeHomebrew refreshes Homebrew metadata, runs `brew upgrade scribe`,
// and verifies the installed formula version matches releaseTag.
func UpgradeHomebrew(ctx context.Context, releaseTag string) ([]byte, error) {
	var combined []byte

	updateCmd := exec.CommandContext(ctx, "brew", "update")
	updateOut, err := updateCmd.CombinedOutput()
	combined = append(combined, updateOut...)
	if err != nil {
		return combined, fmt.Errorf("brew update: %s: %w", string(updateOut), err)
	}

	upgradeCmd := exec.CommandContext(ctx, "brew", "upgrade", "scribe")
	upgradeOut, err := upgradeCmd.CombinedOutput()
	combined = append(combined, upgradeOut...)
	if err != nil {
		return combined, fmt.Errorf("brew upgrade: %s: %w", string(upgradeOut), err)
	}

	listCmd := exec.CommandContext(ctx, "brew", "list", "--versions", "scribe")
	listOut, err := listCmd.CombinedOutput()
	combined = append(combined, listOut...)
	if err != nil {
		return combined, fmt.Errorf("brew list --versions scribe: %s: %w", string(listOut), err)
	}
	if !brewVersionsContain(listOut, releaseTag) {
		installed := strings.TrimSpace(string(listOut))
		if installed == "" {
			installed = "scribe not listed"
		}
		return combined, clierrors.Wrap(
			fmt.Errorf("installed Homebrew scribe version does not match %s (%s)", releaseTag, installed),
			"UPGRADE_VERSION_MISMATCH",
			clierrors.ExitConflict,
			clierrors.WithRemediation("brew tap is stale; run 'brew update' and retry"),
		)
	}
	return combined, nil
}

func brewVersionsContain(out []byte, releaseTag string) bool {
	want := strings.TrimPrefix(strings.TrimSpace(releaseTag), "v")
	for _, field := range strings.Fields(string(out)) {
		if strings.TrimPrefix(field, "v") == want {
			return true
		}
	}
	return false
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

// AssetDownloader downloads a release asset by ID.
type AssetDownloader interface {
	DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, error)
}

// UpgradeBinary downloads the release asset, verifies its checksum, extracts
// the binary, and atomically replaces the current executable.
func UpgradeBinary(ctx context.Context, release *github.RepositoryRelease, downloader AssetDownloader) error {
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
	checksumRC, err := downloader.DownloadReleaseAsset(ctx, "Naoray", "scribe", checksumID)
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
	assetRC, err := downloader.DownloadReleaseAsset(ctx, "Naoray", "scribe", assetID)
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
