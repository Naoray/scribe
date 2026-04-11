package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/Naoray/scribe/internal/paths"
)

const (
	RegistryTypeGitHub    = "github"    // kind: legacy-migrated registry (no type info at migration time)
	RegistryTypeTeam      = "team"      // kind: org/team registry with scribe.yaml
	RegistryTypeCommunity = "community" // kind: community registry (marketplace or tree scan)
)

// RegistryConfig describes a connected skill registry.
type RegistryConfig struct {
	Repo     string `yaml:"repo"`
	Enabled  bool   `yaml:"enabled"`
	Builtin  bool   `yaml:"builtin,omitempty"`
	Type     string `yaml:"type,omitempty"`
	Writable bool   `yaml:"writable,omitempty"`
}

// ToolConfig describes an AI tool target for skill installation.
type ToolConfig struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	Type      string `yaml:"type,omitempty"`      // "builtin" or "custom"
	Detect    string `yaml:"detect,omitempty"`    // shell command used to detect custom tools
	Install   string `yaml:"install,omitempty"`   // shell command template for custom tools
	Uninstall string `yaml:"uninstall,omitempty"` // shell command template for custom tools
	Path      string `yaml:"path,omitempty"`      // optional installed-path template for custom tools
}

// AdoptionConfig holds settings for local skill adoption scanning.
type AdoptionConfig struct {
	Mode  string   `yaml:"mode,omitempty"`  // "auto" | "prompt" | "off"
	Paths []string `yaml:"paths,omitempty"` // optional extra dirs; builtins always included
}

// Config holds user preferences from ~/.scribe/config.yaml.
type Config struct {
	Registries []RegistryConfig `yaml:"registries,omitempty"`
	Token      string           `yaml:"token,omitempty"`
	Tools      []ToolConfig     `yaml:"tools,omitempty"`
	Editor     string           `yaml:"editor,omitempty"`
	Adoption   AdoptionConfig   `yaml:"adoption,omitempty"`
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

// AddRegistry adds or updates a registry in the config.
func (c *Config) AddRegistry(rc RegistryConfig) {
	for i := range c.Registries {
		if strings.EqualFold(c.Registries[i].Repo, rc.Repo) {
			c.Registries[i] = rc
			return
		}
	}
	c.Registries = append(c.Registries, rc)
}

// FindRegistry returns the RegistryConfig for a given repo, or nil if not found.
func (c *Config) FindRegistry(repo string) *RegistryConfig {
	for i := range c.Registries {
		if strings.EqualFold(c.Registries[i].Repo, repo) {
			return &c.Registries[i]
		}
	}
	return nil
}

// EnabledRegistries returns all registries that are enabled.
func (c *Config) EnabledRegistries() []RegistryConfig {
	var enabled []RegistryConfig
	for _, rc := range c.Registries {
		if rc.Enabled {
			enabled = append(enabled, rc)
		}
	}
	return enabled
}

// AdoptionMode returns the validated adoption mode, defaulting to "auto".
func (c *Config) AdoptionMode() string {
	switch c.Adoption.Mode {
	case "auto", "prompt", "off":
		return c.Adoption.Mode
	default:
		return "auto"
	}
}

// AdoptionPaths returns the full list of directories to scan for adoptable skills.
// Builtins (~/.claude/skills and ~/.codex/skills) are always first, followed by
// any user-configured paths. Tilde and relative paths are resolved against the
// user's home directory. Paths outside the home directory are rejected.
func (c *Config) AdoptionPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	builtins := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
	}

	result := make([]string, 0, len(builtins)+len(c.Adoption.Paths))
	result = append(result, builtins...)

	for _, p := range c.Adoption.Paths {
		var resolved string
		if strings.HasPrefix(p, "~/") {
			resolved = filepath.Join(home, p[2:])
		} else if filepath.IsAbs(p) {
			resolved = p
		} else {
			resolved = filepath.Join(home, p)
		}
		resolved = filepath.Clean(resolved)

		// Reject paths outside home dir.
		rel, err := filepath.Rel(home, resolved)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("adoption.paths entry %q is outside home", p)
		}

		result = append(result, resolved)
	}

	return result, nil
}

// IsTeam returns whether this is a team registry.
func (rc RegistryConfig) IsTeam() bool {
	return rc.Type == RegistryTypeTeam
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
		if _, err := cfg.AdoptionPaths(); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	// No YAML -- try TOML migration.
	tomlPath, err := paths.ConfigPath()
	if err != nil {
		return nil, err
	}

	var raw legacyTOML
	if _, err := toml.DecodeFile(tomlPath, &raw); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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
			Type:    RegistryTypeGitHub,
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
