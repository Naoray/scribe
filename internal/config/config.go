package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds user preferences from ~/.scribe/config.toml.
type Config struct {
	TeamRepo string `toml:"team_repo"`
	Token    string `toml:"token"`
}

// Load reads ~/.scribe/config.toml. Returns an empty config if the file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	return &cfg, nil
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
