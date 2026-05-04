package github_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/github"
)

func TestTreeEntryStruct(t *testing.T) {
	// Verify the TreeEntry struct has the fields we need.
	entry := github.TreeEntry{
		Path: "skills/deploy/SKILL.md",
		Type: "blob",
		SHA:  "abc123",
	}
	if entry.Path != "skills/deploy/SKILL.md" {
		t.Errorf("Path: got %q", entry.Path)
	}
	if entry.Type != "blob" {
		t.Errorf("Type: got %q", entry.Type)
	}
}

func TestNewClientPrefersGhAuthToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")

	tmpDir := t.TempDir()
	ghPath := filepath.Join(tmpDir, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = \"auth\" ] && [ \"$2\" = \"token\" ]; then\n  printf 'gh-token\\n'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client := github.NewClient(t.Context(), "config-token")
	if !client.IsAuthenticated() {
		t.Fatal("expected authenticated client from gh auth token")
	}
}

func TestNewClientKeepsGhStateOutOfHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")

	tmpDir := t.TempDir()
	ghPath := filepath.Join(tmpDir, "gh")
	script := `#!/bin/sh
mkdir -p "$XDG_STATE_HOME/gh"
printf device > "$XDG_STATE_HOME/gh/device-id"
printf 'gh-token\n'
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", tmpDir)

	client := github.NewClient(t.Context(), "")
	if !client.IsAuthenticated() {
		t.Fatal("expected authenticated client from gh auth token")
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "state", "gh", "device-id")); !os.IsNotExist(err) {
		t.Fatalf("gh state written under HOME: %v", err)
	}
}
