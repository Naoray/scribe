package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/google/go-github/v69/github"
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

func TestNeedsUpgrade(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		latestTag   string
		wantSkip    bool // dev build — skip entirely
		wantUpgrade bool
	}{
		{
			name:        "dev build skips upgrade",
			current:     "dev",
			latestTag:   "v0.5.0",
			wantSkip:    true,
			wantUpgrade: false,
		},
		{
			name:        "same version, no upgrade",
			current:     "0.5.0",
			latestTag:   "v0.5.0",
			wantSkip:    false,
			wantUpgrade: false,
		},
		{
			name:        "older version, needs upgrade",
			current:     "0.4.0",
			latestTag:   "v0.5.0",
			wantSkip:    false,
			wantUpgrade: true,
		},
		{
			name:        "tag without v prefix",
			current:     "0.5.0",
			latestTag:   "0.5.0",
			wantSkip:    false,
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

func TestUpgradeHomebrewRefreshesTapAndRejectsVersionMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "brew.log")
	brewPath := filepath.Join(dir, "brew")
	script := `#!/bin/sh
echo "$*" >> "` + logPath + `"
case "$1 $2" in
  "update ")
    exit 0
    ;;
  "upgrade scribe")
    exit 0
    ;;
  "list --versions")
    if [ "$3" = "scribe" ]; then
      echo "scribe 1.2.3"
      exit 0
    fi
    ;;
esac
echo "unexpected brew args: $*" >&2
exit 64
`
	if err := os.WriteFile(brewPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := UpgradeHomebrew(context.Background(), "v1.2.4")
	if err == nil {
		t.Fatal("UpgradeHomebrew() error = nil, want version mismatch")
	}

	var ce *clierrors.Error
	if !errors.As(err, &ce) {
		t.Fatalf("UpgradeHomebrew() error = %T, want *clierrors.Error", err)
	}
	if ce.Code != "UPGRADE_VERSION_MISMATCH" {
		t.Fatalf("code = %q, want UPGRADE_VERSION_MISMATCH", ce.Code)
	}
	if ce.Remediation != "brew tap is stale; run 'brew update' and retry" {
		t.Fatalf("remediation = %q", ce.Remediation)
	}

	logBytes, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	got := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	want := []string{"update", "upgrade scribe", "list --versions scribe"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("brew calls = %q, want %q", got, want)
	}
}

func TestReplaceBinary(t *testing.T) {
	t.Run("replaces binary with correct content and permissions", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "scribe")

		if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
			t.Fatal(err)
		}

		newContent := []byte("new-binary-content")
		if err := ReplaceBinary(target, newContent); err != nil {
			t.Fatalf("ReplaceBinary() error = %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, newContent) {
			t.Errorf("content = %q, want %q", got, newContent)
		}

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

type fakeAssetDownloader struct {
	data    map[int64][]byte
	digests map[int64]string
}

func (f fakeAssetDownloader) DownloadReleaseAsset(_ context.Context, _, _ string, id int64) (io.ReadCloser, error) {
	data, ok := f.data[id]
	if !ok {
		return nil, fmt.Errorf("missing asset %d", id)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f fakeAssetDownloader) ReleaseAssetDigest(_ context.Context, _, _ string, id int64) (string, error) {
	return f.digests[id], nil
}

func TestUpgradeBinaryVerifiesGitHubAssetDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-upgrade is not supported on Windows")
	}

	archive := createTarGz(t, []tarEntry{
		{Name: "scribe", Content: []byte("new-scribe")},
	})
	assetName := fmt.Sprintf("scribe_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	assetID := int64(123)
	digest := "sha256:" + sha256sum(archive)
	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{{
			ID:   github.Ptr(assetID),
			Name: github.Ptr(assetName),
		}},
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "scribe")
	if err := os.WriteFile(target, []byte("old-scribe"), 0755); err != nil {
		t.Fatal(err)
	}

	origExec := executablePath
	origSymlinks := evalSymlinks
	t.Cleanup(func() {
		executablePath = origExec
		evalSymlinks = origSymlinks
	})
	executablePath = func() (string, error) { return target, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	err := UpgradeBinary(context.Background(), release, fakeAssetDownloader{
		data:    map[int64][]byte{assetID: archive},
		digests: map[int64]string{assetID: digest},
	})
	if err != nil {
		t.Fatalf("UpgradeBinary() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-scribe" {
		t.Fatalf("upgraded binary = %q, want new-scribe", got)
	}
}

func TestUpgradeBinaryRejectsMissingGitHubAssetDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-upgrade is not supported on Windows")
	}

	assetName := fmt.Sprintf("scribe_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{{
			ID:   github.Ptr(int64(123)),
			Name: github.Ptr(assetName),
		}},
	}

	err := UpgradeBinary(context.Background(), release, fakeAssetDownloader{})
	if err == nil {
		t.Fatal("UpgradeBinary() error = nil, want missing digest error")
	}
	if !strings.Contains(err.Error(), "missing GitHub digest") {
		t.Fatalf("UpgradeBinary() error = %v, want missing GitHub digest", err)
	}
}

func TestUpgradeBinaryRejectsInvalidGitHubAssetDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-upgrade is not supported on Windows")
	}

	tests := []struct {
		name   string
		digest string
		want   string
	}{
		{name: "malformed", digest: "not-a-digest", want: "invalid digest"},
		{name: "unsupported algorithm", digest: "sha512:" + strings.Repeat("0", 128), want: "unsupported digest algorithm"},
		{name: "invalid sha256", digest: "sha256:not-hex", want: "invalid sha256 digest"},
	}

	assetName := fmt.Sprintf("scribe_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assetID := int64(123)
			release := &github.RepositoryRelease{
				Assets: []*github.ReleaseAsset{{
					ID:   github.Ptr(assetID),
					Name: github.Ptr(assetName),
				}},
			}

			err := UpgradeBinary(context.Background(), release, fakeAssetDownloader{
				digests: map[int64]string{assetID: tt.digest},
			})
			if err == nil {
				t.Fatal("UpgradeBinary() error = nil, want digest error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UpgradeBinary() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestUpgradeBinaryRejectsGitHubAssetDigestMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-upgrade is not supported on Windows")
	}

	archive := createTarGz(t, []tarEntry{
		{Name: "scribe", Content: []byte("new-scribe")},
	})
	assetName := fmt.Sprintf("scribe_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	assetID := int64(123)
	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{{
			ID:   github.Ptr(assetID),
			Name: github.Ptr(assetName),
		}},
	}

	err := UpgradeBinary(context.Background(), release, fakeAssetDownloader{
		data:    map[int64][]byte{assetID: archive},
		digests: map[int64]string{assetID: "sha256:" + strings.Repeat("0", 64)},
	})
	if err == nil {
		t.Fatal("UpgradeBinary() error = nil, want checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("UpgradeBinary() error = %v, want checksum mismatch", err)
	}
}
