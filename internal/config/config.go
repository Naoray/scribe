package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds user preferences from ~/.scribe/config.toml.
type Config struct {
	TeamRepos []string `toml:"team_repos"`
	Token     string   `toml:"token"`

	// legacyTeamRepo is populated when the old team_repo key is present.
	// Load() migrates it into TeamRepos automatically.
	legacyTeamRepo string
}

// rawConfig is used to detect legacy team_repo during Load.
type rawConfig struct {
	TeamRepo  string   `toml:"team_repo"`
	TeamRepos []string `toml:"team_repos"`
	Token     string   `toml:"token"`
}

// Load reads ~/.scribe/config.toml. Returns an empty config if the file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		TeamRepos: raw.TeamRepos,
		Token:     raw.Token,
	}

	// Legacy migration: team_repo (singular) → team_repos (plural).
	if raw.TeamRepo != "" && len(cfg.TeamRepos) == 0 {
		cfg.TeamRepos = []string{raw.TeamRepo}
		cfg.legacyTeamRepo = raw.TeamRepo
	}

	return cfg, nil
}

// Save writes the config to ~/.scribe/config.toml atomically.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("encode config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Path returns the absolute path to the config file.
func Path() (string, error) {
	return configPath()
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".scribe", "config.toml"), nil
}
