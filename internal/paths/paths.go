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

// PackagesDir returns the path to ~/.scribe/packages/.
// Packages are self-installing multi-skill bundles that Scribe tracks but
// never projects into tool skill dirs. They live alongside (not inside) the
// canonical skill store so tools like Codex can walk ~/.codex/skills/ without
// tripping on nested SKILL.md files from a package's inner skills.
func PackagesDir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".scribe", "packages"), nil
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
