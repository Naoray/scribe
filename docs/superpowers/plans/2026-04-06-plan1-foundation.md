# Plan 1: Foundation -- Config, State, and Tools Migration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate Scribe's mechanical foundations -- config (TOML to YAML), state (flatten TeamState, rename Targets to Tools, namespace keys), and targets (rename to tools with Detect/Uninstall) -- so all downstream features (multi-registry, packages, uninstall) build on the final data model.

**Architecture:** Three internal packages rewritten (`config/`, `state/`, `targets/` renamed to `tools/`), disk layout namespaced by registry slug, and a README deny-list filter in the syncer. All changes are backward-compatible via shadow structs and auto-migration on Load().

**Tech Stack:** Go 1.26.1, `gopkg.in/yaml.v3` (already in go.mod), `github.com/BurntSushi/toml` (retained for migration read-only)

**Spec:** `docs/superpowers/specs/2026-04-06-catalog-packages.md` (Section: Foundation)

---

### Task 1: Migration Tests (Write FIRST)

**Files:**
- Edit: `internal/config/config_test.go`
- Edit: `internal/state/state_test.go`
- Create: `testdata/config.toml` (fixture)
- Create: `testdata/state.json` (fixture)

- [ ] **Step 1: Create test fixtures**

```toml
# testdata/config.toml
# Legacy Scribe config used as migration fixture.
team_repos = ["ArtistfyHQ/team-skills", "vercel/skills"]
token = "ghp_fixture_token"
```

```json
// testdata/state.json
{
  "team": {
    "last_sync": "2026-03-15T10:00:00Z"
  },
  "installed": {
    "deploy": {
      "version": "v1.0.0",
      "source": "github:ArtistfyHQ/team-skills@v1.0.0",
      "installed_at": "2026-03-10T12:00:00Z",
      "targets": ["claude", "cursor"],
      "paths": ["/Users/test/.claude/skills/deploy", "/Users/test/project/.cursor/rules/deploy.mdc"],
      "registries": ["ArtistfyHQ/team-skills"]
    },
    "gstack": {
      "version": "main",
      "commit_sha": "a3f2c1b9e4d5f678",
      "source": "github:garrytan/gstack@main",
      "installed_at": "2026-03-12T08:00:00Z",
      "targets": ["claude"],
      "paths": ["/Users/test/.claude/skills/gstack"],
      "type": "package",
      "install_cmd": "git clone https://github.com/garrytan/gstack ~/.claude/skills/gstack",
      "update_cmd": "cd ~/.claude/skills/gstack && git pull",
      "cmd_hash": "deadbeef",
      "approval": "approved",
      "approved_at": "2026-03-12T08:00:00Z"
    }
  }
}
```

- [ ] **Step 2: Write config migration tests**

Add to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 3: Run config tests to verify they fail**

Run: `go test ./internal/config/ -run TestMigrate -v`
Expected: FAIL -- `config.RegistryConfig`, `config.ToolConfig`, `cfg.Registries`, `cfg.TeamRepos()` undefined

Run: `go test ./internal/config/ -run TestLoadYAML -v`
Expected: FAIL -- same compilation errors

Run: `go test ./internal/config/ -run TestSaveYAML -v`
Expected: FAIL -- same compilation errors

Run: `go test ./internal/config/ -run TestTeamReposCompat -v`
Expected: FAIL -- same compilation errors

- [ ] **Step 4: Write state migration tests**

Add to `internal/state/state_test.go`:

```go
func TestStateMigrateTargetsToTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write legacy state with "targets" field.
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude", "cursor"],
				"paths": ["/Users/test/.claude/skills/deploy"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skill := s.Installed["deploy"]
	if len(skill.Tools) != 2 {
		t.Fatalf("expected 2 tools after migration, got %d", len(skill.Tools))
	}
	if skill.Tools[0] != "claude" || skill.Tools[1] != "cursor" {
		t.Errorf("tools: got %v", skill.Tools)
	}
}

func TestStatePromoteLastSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.LastSync.IsZero() {
		t.Error("expected LastSync to be promoted from team.last_sync")
	}
	expected := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	if !s.LastSync.Equal(expected) {
		t.Errorf("LastSync: got %v, want %v", s.LastSync, expected)
	}
}

func TestStateNamespaceKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			},
			"recap": {
				"version": "v2.0.0",
				"source": "github:ArtistfyHQ/team-skills@v2.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Bare keys should be namespaced using Registries[0].
	if _, ok := s.Installed["ArtistfyHQ/deploy"]; !ok {
		t.Errorf("expected namespaced key ArtistfyHQ/deploy, got keys: %v", installedKeys(s))
	}
	if _, ok := s.Installed["ArtistfyHQ/recap"]; !ok {
		t.Errorf("expected namespaced key ArtistfyHQ/recap, got keys: %v", installedKeys(s))
	}

	// Bare keys should be gone.
	if _, ok := s.Installed["deploy"]; ok {
		t.Error("bare key 'deploy' should have been removed after namespacing")
	}
}

func TestStateNamespaceKeysNoRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"my-local-skill": {
				"version": "v1.0.0",
				"source": "",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// No registries -- should namespace as "local/<name>".
	if _, ok := s.Installed["local/my-local-skill"]; !ok {
		t.Errorf("expected namespaced key local/my-local-skill, got keys: %v", installedKeys(s))
	}
}

func TestStateMigrateIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	// Already-migrated state: tools field, namespaced keys, top-level last_sync.
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"last_sync": "2026-03-15T10:00:00Z",
		"installed": {
			"ArtistfyHQ/deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Should not double-namespace.
	if _, ok := s.Installed["ArtistfyHQ/deploy"]; !ok {
		t.Errorf("expected key to remain ArtistfyHQ/deploy, got keys: %v", installedKeys(s))
	}
	if len(s.Installed) != 1 {
		t.Errorf("expected 1 installed skill, got %d", len(s.Installed))
	}
}

func TestStateToolsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("ArtistfyHQ/deploy", state.InstalledSkill{
		Version: "v1.0.0",
		Source:  "github:ArtistfyHQ/team-skills@v1.0.0",
		Tools:   []string{"claude", "cursor"},
		Paths:   []string{"/test"},
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := loaded.Installed["ArtistfyHQ/deploy"]
	if len(skill.Tools) != 2 || skill.Tools[0] != "claude" {
		t.Errorf("Tools round-trip: got %v", skill.Tools)
	}
}

// installedKeys is a test helper that returns the keys of the Installed map.
func installedKeys(s *state.State) []string {
	keys := make([]string, 0, len(s.Installed))
	for k := range s.Installed {
		keys = append(keys, k)
	}
	return keys
}
```

- [ ] **Step 5: Run state tests to verify they fail**

Run: `go test ./internal/state/ -run TestStateMigrate -v`
Expected: FAIL -- `skill.Tools` undefined (field is currently `Targets`)

Run: `go test ./internal/state/ -run TestStatePromoteLastSync -v`
Expected: FAIL -- `s.LastSync` undefined (currently nested in `s.Team.LastSync`)

Run: `go test ./internal/state/ -run TestStateNamespaceKeys -v`
Expected: FAIL -- compilation errors on new fields

- [ ] **Step 6: Commit test fixtures and failing tests**

Commit message: `test: add migration test fixtures and failing tests for config/state`

---

### Task 2: Config Rewrite (TOML to YAML)

**Files:**
- Rewrite: `internal/config/config.go`
- Edit: `internal/paths/paths.go` (add `ConfigYAMLPath`)

- [ ] **Step 1: Add YAML config path to paths package**

```go
// Add to internal/paths/paths.go

// ConfigYAMLPath returns the path to ~/.scribe/config.yaml.
func ConfigYAMLPath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "config.yaml"), nil
}
```

- [ ] **Step 2: Rewrite config.go with new structs and migration**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/Naoray/scribe/internal/paths"
)

// RegistryConfig describes a connected skill registry.
type RegistryConfig struct {
	Repo     string `yaml:"repo"`
	Enabled  bool   `yaml:"enabled"`
	Builtin  bool   `yaml:"builtin,omitempty"`
	Type     string `yaml:"type,omitempty"`
	Writable bool   `yaml:"writable,omitempty"`
}

// ToolConfig describes an AI tool target (claude, cursor, etc.).
type ToolConfig struct {
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
}

// Config holds user preferences from ~/.scribe/config.yaml.
type Config struct {
	Registries []RegistryConfig `yaml:"registries,omitempty"`
	Tools      []ToolConfig     `yaml:"tools,omitempty"`
	Token      string           `yaml:"token,omitempty"`
}

// TeamRepos returns the list of enabled registry repos.
// Backward-compatible helper for code that previously used Config.TeamRepos.
func (c *Config) TeamRepos() []string {
	var repos []string
	for _, r := range c.Registries {
		if r.Enabled {
			repos = append(repos, r.Repo)
		}
	}
	return repos
}

// AddRegistry appends a registry if not already present (case-insensitive).
func (c *Config) AddRegistry(repo string) {
	for _, r := range c.Registries {
		if r.Repo == repo {
			return
		}
	}
	c.Registries = append(c.Registries, RegistryConfig{
		Repo:    repo,
		Enabled: true,
		Type:    "github",
	})
}

// legacyTOML is the shadow struct for reading old config.toml files.
type legacyTOML struct {
	TeamRepo  string   `toml:"team_repo"`
	TeamRepos []string `toml:"team_repos"`
	Token     string   `toml:"token"`
}

// Load reads ~/.scribe/config.yaml, falling back to config.toml with auto-migration.
// Priority: config.yaml > config.toml (migrated) > empty config.
func Load() (*Config, error) {
	yamlPath, err := paths.ConfigYAMLPath()
	if err != nil {
		return nil, err
	}

	// Try YAML first.
	data, err := os.ReadFile(yamlPath)
	if err == nil {
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config.yaml: %w", err)
		}
		return &cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	// No YAML -- try TOML migration.
	tomlPath, err := paths.ConfigPath()
	if err != nil {
		return nil, err
	}

	var raw legacyTOML
	if _, err := toml.DecodeFile(tomlPath, &raw); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config.toml: %w", err)
	}

	// Migrate TOML -> Config.
	cfg := migrateFromTOML(raw)

	// Write YAML (keep TOML as backup).
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("write migrated config.yaml: %w", err)
	}

	return cfg, nil
}

// migrateFromTOML converts legacy TOML fields to the new Config struct.
func migrateFromTOML(raw legacyTOML) *Config {
	repos := raw.TeamRepos
	if len(repos) == 0 && raw.TeamRepo != "" {
		repos = []string{raw.TeamRepo}
	}

	cfg := &Config{Token: raw.Token}
	for _, repo := range repos {
		cfg.Registries = append(cfg.Registries, RegistryConfig{
			Repo:    repo,
			Enabled: true,
			Type:    "github",
		})
	}
	return cfg
}

// Save writes the config to ~/.scribe/config.yaml atomically.
func (c *Config) Save() error {
	path, err := paths.ConfigYAMLPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Path returns the absolute path to the config file (YAML).
func Path() (string, error) {
	return paths.ConfigYAMLPath()
}
```

- [ ] **Step 3: Run config tests**

Run: `go test ./internal/config/ -v`
Expected: All new migration tests PASS. Existing tests that reference `cfg.TeamRepos` (the old field) will fail -- proceed to Step 4.

- [ ] **Step 4: Update existing config tests for new API**

Update tests that used `cfg.TeamRepos` (the slice field) to use `cfg.TeamRepos()` (the method) or `cfg.Registries`:

In `TestLoadTeamRepos`: change `cfg.TeamRepos` to `cfg.TeamRepos()`.
In `TestLoadLegacyTeamRepo`: change `cfg.TeamRepos` to `cfg.TeamRepos()`.
In `TestLoadLegacyIgnoredWhenNewPresent`: change `cfg.TeamRepos` to `cfg.TeamRepos()`.
In `TestSave` and `TestSaveRoundTrip`: update to use new `Config` struct with `Registries`.

The `writeConfig` helper still writes TOML, which is correct for migration tests. Add a `writeYAMLConfig` helper:

```go
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
```

- [ ] **Step 5: Run all config tests**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 6: Update callers of config.TeamRepos (field) to config.TeamRepos() (method)**

Every file that accesses `cfg.TeamRepos` as a field must switch to `cfg.TeamRepos()`. Files to update:

- `internal/workflow/sync.go` -- `b.Config.TeamRepos` -> `b.Config.TeamRepos()`
- `internal/workflow/list.go` -- `b.Config.TeamRepos` -> `b.Config.TeamRepos()`
- `cmd/add.go` -- `cfg.TeamRepos` -> `cfg.TeamRepos()`
- `cmd/connect.go` -- `cfg.TeamRepos` -> update `AddRegistry` usage
- `cmd/sync.go` (if exists) -- same pattern

Also update `Bag.Repos` population in workflow steps.

- [ ] **Step 7: Verify full build**

Run: `go build ./...`
Expected: Clean build, no errors

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 8: Commit**

Commit message: `feat: migrate config from TOML to YAML with auto-migration`

---

### Task 3: State Migration

**Files:**
- Rewrite: `internal/state/state.go`

- [ ] **Step 1: Rewrite state.go with new struct and migration logic**

```go
// internal/state/state.go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Naoray/scribe/internal/paths"
)

// State is the contents of ~/.scribe/state.json.
type State struct {
	LastSync  time.Time                `json:"last_sync,omitempty"`
	Installed map[string]InstalledSkill `json:"installed"`
}

// InstalledSkill records everything needed to detect updates and uninstall.
type InstalledSkill struct {
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Tools       []string  `json:"tools"`
	Paths       []string  `json:"paths"`

	// Package-specific fields (omitted for regular skills).
	Type       string    `json:"type,omitempty"`
	InstallCmd string    `json:"install_cmd,omitempty"`
	UpdateCmd  string    `json:"update_cmd,omitempty"`
	CmdHash    string    `json:"cmd_hash,omitempty"`
	Approval   string    `json:"approval,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}

// legacyState is a shadow struct for reading pre-migration state.json files.
type legacyState struct {
	Team      *legacyTeamState                    `json:"team,omitempty"`
	LastSync  *time.Time                          `json:"last_sync,omitempty"`
	Installed map[string]json.RawMessage          `json:"installed"`
}

type legacyTeamState struct {
	LastSync time.Time `json:"last_sync,omitempty"`
}

// legacyInstalledSkill handles both "targets" and "tools" JSON fields.
type legacyInstalledSkill struct {
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Targets     []string  `json:"targets,omitempty"`
	Tools       []string  `json:"tools,omitempty"`
	Paths       []string  `json:"paths"`
	Registries  []string  `json:"registries,omitempty"`
	Type        string    `json:"type,omitempty"`
	InstallCmd  string    `json:"install_cmd,omitempty"`
	UpdateCmd   string    `json:"update_cmd,omitempty"`
	CmdHash     string    `json:"cmd_hash,omitempty"`
	Approval    string    `json:"approval,omitempty"`
	ApprovedAt  time.Time `json:"approved_at,omitempty"`
}

// shortSHA returns the first 7 chars of CommitSHA, or "" if not set.
func (s InstalledSkill) shortSHA() string {
	if len(s.CommitSHA) >= 7 {
		return s.CommitSHA[:7]
	}
	return s.CommitSHA
}

// DisplayVersion returns the version string shown in `scribe list`.
func (s InstalledSkill) DisplayVersion() string {
	if s.CommitSHA != "" {
		return s.Version + "@" + s.shortSHA()
	}
	return s.Version
}

// Load reads state from disk. Returns an empty state if the file doesn't exist yet.
// Auto-migrates legacy formats: promotes team.last_sync, renames targets->tools,
// namespaces bare keys.
func Load() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	lf, err := lockFile(path+".lock", false)
	if err != nil {
		return nil, fmt.Errorf("lock state: %w", err)
	}
	defer unlockFile(lf)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{Installed: make(map[string]InstalledSkill)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	return parseAndMigrate(data)
}

// parseAndMigrate deserializes state.json and applies migrations.
func parseAndMigrate(data []byte) (*State, error) {
	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	s := &State{
		Installed: make(map[string]InstalledSkill, len(legacy.Installed)),
	}

	// Migration 1: Promote team.last_sync to top-level.
	if legacy.LastSync != nil {
		s.LastSync = *legacy.LastSync
	} else if legacy.Team != nil && !legacy.Team.LastSync.IsZero() {
		s.LastSync = legacy.Team.LastSync
	}

	// Migration 2+3: Parse each installed skill, rename targets->tools, namespace keys.
	for name, raw := range legacy.Installed {
		var ls legacyInstalledSkill
		if err := json.Unmarshal(raw, &ls); err != nil {
			return nil, fmt.Errorf("parse installed skill %q: %w", name, err)
		}

		tools := ls.Tools
		if len(tools) == 0 && len(ls.Targets) > 0 {
			tools = ls.Targets
		}

		skill := InstalledSkill{
			Version:     ls.Version,
			CommitSHA:   ls.CommitSHA,
			Source:      ls.Source,
			InstalledAt: ls.InstalledAt,
			Tools:       tools,
			Paths:       ls.Paths,
			Type:        ls.Type,
			InstallCmd:  ls.InstallCmd,
			UpdateCmd:   ls.UpdateCmd,
			CmdHash:     ls.CmdHash,
			Approval:    ls.Approval,
			ApprovedAt:  ls.ApprovedAt,
		}

		// Namespace bare keys.
		nsKey := namespaceKey(name, ls.Registries)
		s.Installed[nsKey] = skill
	}

	if s.Installed == nil {
		s.Installed = make(map[string]InstalledSkill)
	}

	return s, nil
}

// namespaceKey adds a registry prefix to bare skill names.
// Already-namespaced keys (containing "/") are returned unchanged.
func namespaceKey(name string, registries []string) string {
	if strings.Contains(name, "/") {
		return name
	}
	if len(registries) > 0 {
		owner, _, _ := strings.Cut(registries[0], "/")
		return owner + "/" + name
	}
	return "local/" + name
}

// Save writes state to disk atomically.
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	lf, err := lockFile(path+".lock", true)
	if err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer unlockFile(lf)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

// RecordSync updates the last sync timestamp.
func (s *State) RecordSync() {
	s.LastSync = time.Now().UTC()
}

// RecordInstall records a successful skill install.
func (s *State) RecordInstall(name string, skill InstalledSkill) {
	skill.InstalledAt = time.Now().UTC()
	s.Installed[name] = skill
}

// Remove deletes a skill from state.
func (s *State) Remove(name string) {
	delete(s.Installed, name)
}

func statePath() (string, error) {
	return paths.StatePath()
}

// Dir returns the path to the ~/.scribe directory.
func Dir() (string, error) {
	return paths.ScribeDir()
}

// lockFile acquires an advisory flock on the given path.
func lockFile(path string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	lockType := syscall.LOCK_SH
	if exclusive {
		lockType = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), lockType); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// unlockFile releases the advisory lock and closes the file.
func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}
```

- [ ] **Step 2: Update callers that used removed fields**

Files that reference `s.Team.LastSync`:
- `internal/workflow/list.go` line 260: `st.Team.LastSync` -> `st.LastSync`

Files that reference `s.Installed[name].Targets`:
- `internal/sync/syncer.go` line 221: `Targets: targetNames` -> `Tools: targetNames`
- `internal/discovery/discovery.go` line 111: `installed.Targets` -> `installed.Tools`
- `internal/workflow/list.go` lines 142, 235, 299: `sk.Targets` / `sk.Installed.Targets` -> `.Tools`
- `cmd/add.go` line 85: `targets.Target` references in Adder (handled in Task 4)

Files that reference `s.AddRegistry`, `s.RemoveRegistry`, `s.MigrateRegistries`:
- Remove `AddRegistry`, `RemoveRegistry`, `MigrateRegistries` methods (registries now tracked by namespaced keys)
- `internal/workflow/sync.go` lines 67, 139-140: remove `StepMigrateRegistries` and `AddRegistry` calls

- [ ] **Step 3: Update existing state tests**

Update `TestSaveAndLoad` to use `s.LastSync` instead of `s.Team.LastSync`.
Update `TestMigrationBackfillsRegistries` -- this test covers the old `MigrateRegistries` method which is removed. Convert it to test the new namespacing migration instead.
Update all tests that use `.Targets` field to use `.Tools`.

- [ ] **Step 4: Run all state tests**

Run: `go test ./internal/state/ -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: Compilation errors in packages that reference old state fields -- proceed to Task 4.

- [ ] **Step 6: Commit**

Commit message: `feat: migrate state to flat structure with namespaced keys and tools rename`

---

### Task 4: targets -> tools Rename

**Files:**
- Rename: `internal/targets/` -> `internal/tools/`
- Rename: `target.go` -> `tool.go`, `ClaudeTarget` -> `ClaudeTool`, etc.
- Edit: All files importing `internal/targets`

- [ ] **Step 1: Create the tools package (copy + rename)**

Create `internal/tools/` with the following files, renamed from `internal/targets/`:

**`internal/tools/tool.go`** (was `target.go`):
```go
package tools

// SkillFile represents a file to be written to the skill store.
type SkillFile struct {
	Path    string // relative to the skill root
	Content []byte
}

// Tool links a skill from the canonical store into a specific AI tool's directory.
type Tool interface {
	// Name returns the tool identifier (e.g. "claude", "cursor").
	Name() string
	// Install creates a link from the agent's expected directory into canonicalDir.
	Install(skillName, canonicalDir string) (paths []string, err error)
	// Uninstall removes the links for a skill.
	Uninstall(skillName string) error
	// Detect reports whether this tool is installed on the machine.
	Detect() bool
}

// DetectTools returns tools that are actually installed on this machine.
func DetectTools() []Tool {
	all := []Tool{ClaudeTool{}, CursorTool{}}
	var detected []Tool
	for _, t := range all {
		if t.Detect() {
			detected = append(detected, t)
		}
	}
	return detected
}

// AllTools returns all known tools regardless of detection.
func AllTools() []Tool {
	return []Tool{ClaudeTool{}, CursorTool{}}
}
```

**`internal/tools/claude.go`** (was `targets/claude.go`):
```go
package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

const ToolClaude = "claude"

// ClaudeTool symlinks ~/.claude/skills/<name> -> ~/.scribe/skills/<name>.
type ClaudeTool struct{}

func (t ClaudeTool) Name() string { return ToolClaude }

func (t ClaudeTool) Detect() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude"))
	return err == nil
}

func (t ClaudeTool) Install(skillName, canonicalDir string) ([]string, error) {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create claude skills dir: %w", err)
	}

	link := filepath.Join(skillsDir, skillName)
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, fmt.Errorf("symlink claude/%s: %w", skillName, err)
	}
	return []string{link}, nil
}

func (t ClaudeTool) Uninstall(skillName string) error {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove claude/%s: %w", skillName, err)
	}
	return nil
}

func claudeSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}
```

**`internal/tools/cursor.go`** (was `targets/cursor.go`) -- same pattern, add `Detect()` and `Uninstall()`:
```go
func (t CursorTool) Detect() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".cursor"))
	return err == nil
}

func (t CursorTool) Uninstall(skillName string) error {
	workDir := t.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
	}
	link := filepath.Join(workDir, ".cursor", "rules", skillName+".mdc")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cursor/%s: %w", skillName, err)
	}
	return nil
}
```

Copy `symlink.go` and `store.go` unchanged (package name updated to `tools`).

- [ ] **Step 2: Update all imports across the codebase**

Every file importing `"github.com/Naoray/scribe/internal/targets"` must change to `"github.com/Naoray/scribe/internal/tools"`. Files to update:

- `internal/sync/syncer.go` -- import + all `targets.Target` -> `tools.Tool`, `targets.SkillFile` -> `tools.SkillFile`, `targets.WriteToStore` -> `tools.WriteToStore`
- `internal/workflow/bag.go` -- import + `Targets []targets.Target` -> `Tools []tools.Tool`
- `internal/workflow/sync.go` -- import + `StepResolveTargets` -> `StepResolveTools`, `targets.DefaultTargets()` -> `tools.DetectTools()`, `b.Targets` -> `b.Tools`
- `internal/workflow/list.go` -- import + usage
- `internal/add/add.go` -- import + `Targets []targets.Target` -> `Tools []tools.Tool`
- `cmd/add.go` -- import + all `targets.` references

- [ ] **Step 3: Update tests**

Rename `internal/targets/targets_test.go` to `internal/tools/tools_test.go` (update package name and imports). Add `Detect` and `Uninstall` tests:

```go
func TestClaudeDetect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tool := tools.ClaudeTool{}
	if tool.Detect() {
		t.Error("should not detect claude when ~/.claude doesn't exist")
	}

	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	if !tool.Detect() {
		t.Error("should detect claude when ~/.claude exists")
	}
}

func TestClaudeUninstall(t *testing.T) {
	canonicalDir := setup(t)
	tool := tools.ClaudeTool{}

	// Install first.
	if _, err := tool.Install("deploy", canonicalDir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Uninstall.
	if err := tool.Uninstall("deploy"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// Verify symlink removed.
	home, _ := os.UserHomeDir()
	link := filepath.Join(home, ".claude", "skills", "deploy")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed after uninstall")
	}
}
```

- [ ] **Step 4: Delete old internal/targets/ directory**

After all imports are updated and tests pass, remove the old package:

```bash
rm -rf internal/targets/
```

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All PASS

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 6: Commit**

Commit message: `refactor: rename targets package to tools, add Detect and Uninstall`

---

### Task 5: Disk Layout Namespacing

**Files:**
- Edit: `internal/tools/store.go`
- Edit: `internal/tools/claude.go`
- Edit: `internal/tools/cursor.go`
- Edit: `internal/sync/syncer.go`
- Edit: `internal/discovery/discovery.go`
- Edit: `internal/tools/tools_test.go`

- [ ] **Step 1: Write failing tests for namespaced store**

Add to `internal/tools/tools_test.go`:

```go
func TestSlugifyRegistry(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"ArtistfyHQ/team-skills", "ArtistfyHQ-team-skills"},
		{"vercel/skills", "vercel-skills"},
		{"owner/repo", "owner-repo"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := tools.SlugifyRegistry(c.input)
			if got != c.want {
				t.Errorf("SlugifyRegistry(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestWriteToStoreNamespaced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := tools.WriteToStore("ArtistfyHQ-team-skills", "deploy", testFiles)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	// Verify path includes registry slug.
	storeDir, _ := tools.StoreDir()
	expected := filepath.Join(storeDir, "ArtistfyHQ-team-skills", "deploy")
	if dir != expected {
		t.Errorf("store dir = %q, want %q", dir, expected)
	}

	// Files exist.
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Error("SKILL.md not in namespaced store")
	}
}

func TestClaudeInstallNamespaced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := tools.WriteToStore("ArtistfyHQ-team-skills", "deploy", testFiles)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	tool := tools.ClaudeTool{}
	paths, err := tool.Install("ArtistfyHQ-team-skills/deploy", dir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Symlink should be at ~/.claude/skills/ArtistfyHQ-team-skills/deploy
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude", "skills", "ArtistfyHQ-team-skills", "deploy")
	if paths[0] != expected {
		t.Errorf("symlink path = %q, want %q", paths[0], expected)
	}

	resolved, _ := os.Readlink(paths[0])
	if resolved != dir {
		t.Errorf("symlink resolves to %q, want %q", resolved, dir)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tools/ -run TestSlugifyRegistry -v`
Expected: FAIL -- `tools.SlugifyRegistry` undefined

Run: `go test ./internal/tools/ -run TestWriteToStoreNamespaced -v`
Expected: FAIL -- `WriteToStore` signature mismatch (missing registrySlug param)

- [ ] **Step 3: Update store.go with registrySlug parameter**

```go
// internal/tools/store.go
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/paths"
)

// SlugifyRegistry converts "owner/repo" to "owner-repo" for filesystem paths.
func SlugifyRegistry(repo string) string {
	return strings.ReplaceAll(repo, "/", "-")
}

// WriteToStore writes all skill files to ~/.scribe/skills/<registrySlug>/<name>/.
// Returns the canonical directory path.
func WriteToStore(registrySlug, skillName string, files []SkillFile) (string, error) {
	base, err := StoreDir()
	if err != nil {
		return "", err
	}

	skillDir := filepath.Join(base, registrySlug, skillName)

	if err := os.RemoveAll(skillDir); err != nil {
		return "", fmt.Errorf("clear store for %s/%s: %w", registrySlug, skillName, err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("create store dir: %w", err)
	}

	for _, f := range files {
		dest := filepath.Join(skillDir, f.Path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", f.Path, err)
		}
	}

	return skillDir, nil
}

// StoreDir returns the ~/.scribe/skills/ directory path.
func StoreDir() (string, error) {
	return paths.StoreDir()
}
```

- [ ] **Step 4: Update ClaudeTool.Install for namespaced paths**

The `skillName` parameter now contains the registry-qualified name (e.g. `"ArtistfyHQ-team-skills/deploy"`). The symlink path should mirror this:

```go
func (t ClaudeTool) Install(skillName, canonicalDir string) ([]string, error) {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}

	link := filepath.Join(skillsDir, skillName)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return nil, fmt.Errorf("create claude skills subdir: %w", err)
	}
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, fmt.Errorf("symlink claude/%s: %w", skillName, err)
	}
	return []string{link}, nil
}
```

Similarly update `CursorTool.Install` and `Uninstall` to handle paths with `/`.

- [ ] **Step 5: Update syncer.apply() to pass registrySlug**

In `internal/sync/syncer.go`, the `apply` method needs the registry slug. Add `registrySlug` as a parameter or derive it from `SkillStatus.Entry.Source`:

```go
// In syncer.go apply(), update the WriteToStore call:
registrySlug := tools.SlugifyRegistry(teamRepo)
canonicalDir, err := tools.WriteToStore(registrySlug, sk.Name, tFiles)
```

This requires threading `teamRepo` through to `apply`. Update `Run` to pass it:

```go
func (s *Syncer) Run(ctx context.Context, teamRepo string, st *state.State) error {
	statuses, _, err := s.Diff(ctx, teamRepo, st)
	if err != nil {
		return err
	}
	return s.apply(ctx, teamRepo, statuses, st)
}
```

- [ ] **Step 6: Update discovery.OnDisk() to scan nested registry directories**

In `internal/discovery/discovery.go`, `OnDisk` currently scans one level deep. Add support for two-level scanning (registry-slug/skill-name):

```go
// In OnDisk, after scanning direct children, also scan subdirectories
// (registry slug directories) for their children.
for _, entry := range entries {
	if !entry.IsDir() {
		continue
	}
	name := entry.Name()

	// Check if this is a registry slug directory (contains subdirs with SKILL.md).
	subPath := filepath.Join(dir.path, name)
	subEntries, err := os.ReadDir(subPath)
	if err != nil {
		continue
	}
	for _, sub := range subEntries {
		if !sub.IsDir() {
			continue
		}
		qualifiedName := name + "/" + sub.Name()
		if seen[qualifiedName] {
			continue
		}
		skillDir := filepath.Join(subPath, sub.Name())
		// ... same SKILL.md reading logic ...
	}
}
```

- [ ] **Step 7: Update existing tests for new WriteToStore signature**

All existing tests that call `WriteToStore("deploy", testFiles)` need an extra `""` or `"test-registry"` first argument.

- [ ] **Step 8: Run full test suite**

Run: `go test ./...`
Expected: All PASS

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 9: Commit**

Commit message: `feat: namespace disk layout by registry slug`

---

### Task 6: README Sync Bug Fix

**Files:**
- Edit: `internal/sync/syncer.go`
- Create: `internal/sync/filter.go`
- Create: `internal/sync/filter_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/sync/filter_test.go
package sync

import "testing"

func TestShouldInclude(t *testing.T) {
	cases := []struct {
		name string
		file string
		want bool
	}{
		{"skill file", "SKILL.md", true},
		{"script", "scripts/deploy.sh", true},
		{"readme", "README.md", true},      // README.md inside a skill IS included
		{"nested file", "lib/helper.go", true},

		// Deny list — these are repo-root files that leak into skill directories
		// when the skill path == repo root.
		{"dot git", ".git/config", false},
		{"dot gitignore", ".gitignore", false},
		{"dot gitkeep", ".gitkeep", false},
		{"license", "LICENSE", false},
		{"license md", "LICENSE.md", false},
		{"license txt", "LICENSE.txt", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldInclude(c.file)
			if got != c.want {
				t.Errorf("shouldInclude(%q) = %v, want %v", c.file, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run TestShouldInclude -v`
Expected: FAIL -- `shouldInclude` undefined

- [ ] **Step 3: Implement the filter**

```go
// internal/sync/filter.go
package sync

import (
	"path/filepath"
	"strings"
)

// denyPrefixes are path prefixes that should never be synced into the skill store.
var denyPrefixes = []string{
	".git/",
}

// denyExact are exact filenames (case-insensitive) denied at the root of a skill.
var denyExact = []string{
	".gitignore",
	".gitkeep",
	"license",
	"license.md",
	"license.txt",
	"license.mit",
	"license.apache",
}

// shouldInclude reports whether a file should be synced into the skill store.
// Filters out repo infrastructure files that leak when skill path == repo root.
func shouldInclude(path string) bool {
	for _, prefix := range denyPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}

	// Only deny exact matches at the root level (not nested).
	if filepath.Dir(path) == "." {
		base := strings.ToLower(filepath.Base(path))
		for _, denied := range denyExact {
			if base == denied {
				return false
			}
		}
	}

	return true
}
```

- [ ] **Step 4: Run filter tests**

Run: `go test ./internal/sync/ -run TestShouldInclude -v`
Expected: All PASS

- [ ] **Step 5: Wire shouldInclude into syncer.apply()**

In `internal/sync/syncer.go`, add filtering before `WriteToStore`:

```go
// In apply(), after fetching files and before converting to tFiles:
var filtered []SkillFile
for _, f := range files {
	if shouldInclude(f.Path) {
		filtered = append(filtered, f)
	}
}

tFiles := make([]tools.SkillFile, len(filtered))
for i, f := range filtered {
	tFiles[i] = tools.SkillFile{Path: f.Path, Content: f.Content}
}
```

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`
Expected: All PASS

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 7: Commit**

Commit message: `fix: filter repo infrastructure files (LICENSE, .gitignore) from skill sync`

---

## Verification Checklist

After all tasks are complete, run these verification commands:

```bash
# Full build
go build ./...

# Full test suite
go test ./...

# Verify no references to old package
grep -r '"github.com/Naoray/scribe/internal/targets"' internal/ cmd/
# Expected: no matches

# Verify YAML config loads
go run ./cmd/scribe --help

# Verify config.yaml path
grep -r 'config\.toml' internal/paths/
# Expected: ConfigPath() still exists for migration, ConfigYAMLPath() is new primary
```
