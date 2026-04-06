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

func writeYAMLConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissing(t *testing.T) {
	setupHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(cfg.TeamRepos()) != 0 {
		t.Errorf("expected empty TeamRepos, got %v", cfg.TeamRepos())
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
	if len(cfg.TeamRepos()) != 2 {
		t.Fatalf("expected 2 team repos, got %d", len(cfg.TeamRepos()))
	}
	if cfg.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("first repo: got %q", cfg.TeamRepos()[0])
	}
	if cfg.TeamRepos()[1] != "vercel/skills" {
		t.Errorf("second repo: got %q", cfg.TeamRepos()[1])
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
	if len(cfg.TeamRepos()) != 1 {
		t.Fatalf("expected 1 team repo from legacy migration, got %d", len(cfg.TeamRepos()))
	}
	if cfg.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("migrated repo: got %q", cfg.TeamRepos()[0])
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
	if len(cfg.TeamRepos()) != 1 || cfg.TeamRepos()[0] != "new/repo" {
		t.Errorf("expected [new/repo], got %v", cfg.TeamRepos())
	}
}

func TestSave(t *testing.T) {
	home := setupHome(t)

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true, Type: "github"},
		},
		Token: "ghp_test",
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
	if len(loaded.TeamRepos()) != 1 || loaded.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("TeamRepos: got %v", loaded.TeamRepos())
	}
	if loaded.Token != "ghp_test" {
		t.Errorf("Token: got %q", loaded.Token)
	}

	// Verify file exists on disk.
	data, err := os.ReadFile(filepath.Join(home, ".scribe", "config.yaml"))
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
		Registries: []config.RegistryConfig{
			{Repo: "a/b", Enabled: true, Type: "github"},
			{Repo: "c/d", Enabled: true, Type: "github"},
		},
		Token: "tok",
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.TeamRepos()) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.TeamRepos()))
	}
	if loaded.TeamRepos()[0] != "a/b" || loaded.TeamRepos()[1] != "c/d" {
		t.Errorf("repos: got %v", loaded.TeamRepos())
	}
}

// --- Migration tests (Task 1) ---

func TestMigrateTOMLToYAML(t *testing.T) {
	home := setupHome(t)

	// Write a legacy TOML config.
	writeConfig(t, home, `
team_repos = ["ArtistfyHQ/team-skills", "vercel/skills"]
token = "ghp_migrate_test"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify fields migrated correctly.
	if len(cfg.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(cfg.Registries))
	}
	if cfg.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("first registry: got %q", cfg.Registries[0].Repo)
	}
	if cfg.Registries[1].Repo != "vercel/skills" {
		t.Errorf("second registry: got %q", cfg.Registries[1].Repo)
	}
	if cfg.Token != "ghp_migrate_test" {
		t.Errorf("token: got %q", cfg.Token)
	}

	// Verify YAML file was written.
	yamlPath := filepath.Join(home, ".scribe", "config.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Error("expected config.yaml to be created during migration")
	}

	// Verify TOML backup preserved.
	tomlPath := filepath.Join(home, ".scribe", "config.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		t.Error("expected config.toml to be preserved as backup")
	}
}

func TestMigrateIdempotent(t *testing.T) {
	home := setupHome(t)

	// Write YAML config directly (already migrated).
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	yamlContent := `registries:
  - repo: ArtistfyHQ/team-skills
    enabled: true
token: ghp_yaml_test
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(cfg.Registries))
	}
	if cfg.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("registry repo: got %q", cfg.Registries[0].Repo)
	}
	if cfg.Token != "ghp_yaml_test" {
		t.Errorf("token: got %q", cfg.Token)
	}
}

func TestMigrateYAMLWinsOverTOML(t *testing.T) {
	home := setupHome(t)
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	// Both files exist -- YAML should win.
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
team_repos = ["old/toml-repo"]
token = "toml_token"
`), 0o644)

	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`registries:
  - repo: new/yaml-repo
    enabled: true
token: yaml_token
`), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 1 || cfg.Registries[0].Repo != "new/yaml-repo" {
		t.Errorf("expected YAML to win, got registries %+v", cfg.Registries)
	}
	if cfg.Token != "yaml_token" {
		t.Errorf("expected YAML token, got %q", cfg.Token)
	}
}

func TestLoadYAML(t *testing.T) {
	home := setupHome(t)
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	yamlContent := `registries:
  - repo: ArtistfyHQ/team-skills
    enabled: true
    builtin: false
    type: github
    writable: true
  - repo: vercel/skills
    enabled: true
    builtin: false
    type: github
    writable: false
tools:
  - name: claude
    enabled: true
  - name: cursor
    enabled: false
token: ghp_yaml_full
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(cfg.Registries))
	}
	if !cfg.Registries[0].Writable {
		t.Error("first registry should be writable")
	}
	if cfg.Registries[1].Writable {
		t.Error("second registry should not be writable")
	}
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Name != "claude" || !cfg.Tools[0].Enabled {
		t.Errorf("first tool: got %+v", cfg.Tools[0])
	}
	if cfg.Tools[1].Name != "cursor" || cfg.Tools[1].Enabled {
		t.Errorf("second tool: got %+v", cfg.Tools[1])
	}
}

func TestSaveYAML(t *testing.T) {
	home := setupHome(t)

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true, Type: "github", Writable: true},
		},
		Tools: []config.ToolConfig{
			{Name: "claude", Enabled: true},
		},
		Token: "ghp_save_test",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify YAML file written.
	yamlPath := filepath.Join(home, ".scribe", "config.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if len(data) == 0 {
		t.Error("config.yaml is empty")
	}

	// Verify .tmp file cleaned up.
	if _, err := os.Stat(yamlPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after save")
	}

	// Reload and verify round-trip.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.Registries) != 1 || loaded.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("Registries round-trip: got %+v", loaded.Registries)
	}
	if loaded.Token != "ghp_save_test" {
		t.Errorf("Token round-trip: got %q", loaded.Token)
	}
}

// TeamRepos returns the list of registry repo strings for backward compatibility.
func TestTeamReposCompat(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true},
			{Repo: "disabled/repo", Enabled: false},
			{Repo: "vercel/skills", Enabled: true},
		},
	}
	repos := cfg.TeamRepos()
	if len(repos) != 2 {
		t.Fatalf("expected 2 enabled repos, got %d: %v", len(repos), repos)
	}
	if repos[0] != "ArtistfyHQ/team-skills" || repos[1] != "vercel/skills" {
		t.Errorf("repos: got %v", repos)
	}
}
