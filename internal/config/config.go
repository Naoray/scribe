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
