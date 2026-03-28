package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissing(t *testing.T) {
	setupHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(cfg.TeamRepos) != 0 {
		t.Errorf("expected empty TeamRepos, got %v", cfg.TeamRepos)
	}
}

func TestLoadTeamRepos(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, home, `
team_repos = ["ArtistfyHQ/team-skills", "vercel/skills"]
token = "ghp_test"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos) != 2 {
		t.Fatalf("expected 2 team repos, got %d", len(cfg.TeamRepos))
	}
	if cfg.TeamRepos[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("first repo: got %q", cfg.TeamRepos[0])
	}
	if cfg.TeamRepos[1] != "vercel/skills" {
		t.Errorf("second repo: got %q", cfg.TeamRepos[1])
	}
	if cfg.Token != "ghp_test" {
		t.Errorf("token: got %q", cfg.Token)
	}
}

func TestLoadLegacyTeamRepo(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, home, `
team_repo = "ArtistfyHQ/team-skills"
token = "ghp_legacy"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos) != 1 {
		t.Fatalf("expected 1 team repo from legacy migration, got %d", len(cfg.TeamRepos))
	}
	if cfg.TeamRepos[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("migrated repo: got %q", cfg.TeamRepos[0])
	}
}

func TestLoadLegacyIgnoredWhenNewPresent(t *testing.T) {
	home := setupHome(t)
	// Both keys present — team_repos takes precedence, legacy is ignored.
	writeConfig(t, home, `
team_repo = "old/repo"
team_repos = ["new/repo"]
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos) != 1 || cfg.TeamRepos[0] != "new/repo" {
		t.Errorf("expected [new/repo], got %v", cfg.TeamRepos)
	}
}

func TestSave(t *testing.T) {
	home := setupHome(t)

	cfg := &config.Config{
		TeamRepos: []string{"ArtistfyHQ/team-skills"},
		Token:     "ghp_test",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the .tmp file doesn't linger.
	path, _ := config.Path()
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after save")
	}

	// Reload and verify.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.TeamRepos) != 1 || loaded.TeamRepos[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("TeamRepos: got %v", loaded.TeamRepos)
	}
	if loaded.Token != "ghp_test" {
		t.Errorf("Token: got %q", loaded.Token)
	}

	// Verify file exists on disk.
	data, err := os.ReadFile(filepath.Join(home, ".scribe", "config.toml"))
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if len(data) == 0 {
		t.Error("config file is empty")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	setupHome(t)

	original := &config.Config{
		TeamRepos: []string{"a/b", "c/d"},
		Token:     "tok",
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.TeamRepos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.TeamRepos))
	}
	if loaded.TeamRepos[0] != "a/b" || loaded.TeamRepos[1] != "c/d" {
		t.Errorf("repos: got %v", loaded.TeamRepos)
	}
}
