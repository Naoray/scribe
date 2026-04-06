package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

func homeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return home, nil
}

// ScribeDir returns the path to ~/.scribe/.
func ScribeDir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe"), nil
}

// StoreDir returns the path to ~/.scribe/skills/.
func StoreDir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "skills"), nil
}

// ConfigPath returns the path to ~/.scribe/config.toml.
func ConfigPath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "config.toml"), nil
}

// ConfigYAMLPath returns the path to ~/.scribe/config.yaml.
func ConfigYAMLPath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "config.yaml"), nil
}

// StatePath returns the path to ~/.scribe/state.json.
func StatePath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "state.json"), nil
}
