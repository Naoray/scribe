package prereq_test

import (
	"os"
	"testing"

	"github.com/Naoray/scribe/internal/prereq"
)

func TestCheck_NoScribeDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	result := prereq.Check()

	if result.ScribeDir.OK {
		t.Error("expected ScribeDir.OK to be false for missing dir")
	}
}

func TestCheck_NoAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "") // prevent gh CLI from being found

	result := prereq.Check()

	if result.GitHubAuth.OK {
		t.Error("expected GitHubAuth.OK to be false without any auth")
	}
}

func TestCheck_WithToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("PATH", "") // prevent gh CLI from winning over GITHUB_TOKEN

	result := prereq.Check()

	if !result.GitHubAuth.OK {
		t.Error("expected GitHubAuth.OK to be true with GITHUB_TOKEN")
	}
	if result.GitHubAuth.Method != "GITHUB_TOKEN" {
		t.Errorf("expected method GITHUB_TOKEN, got %s", result.GitHubAuth.Method)
	}
}

func TestCheck_WithConnections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	// Write a config with team_repos
	configDir := home + "/.scribe"
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configDir+"/config.toml", []byte("team_repos = [\"Org/repo\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := prereq.Check()

	if !result.ScribeDir.OK {
		t.Error("expected ScribeDir.OK to be true when ~/.scribe/ exists")
	}
	if len(result.Connections.Repos) != 1 {
		t.Errorf("expected 1 connection, got %d", len(result.Connections.Repos))
	}
}
