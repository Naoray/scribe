package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
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
