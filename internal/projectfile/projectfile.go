package projectfile

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Filename is the conventional name for a Scribe project file.
const Filename = ".scribe.yaml"

// ProjectFile declares which Scribe assets a project wants projected locally.
type ProjectFile struct {
	Kits     []string `yaml:"kits,omitempty"`
	Snippets []string `yaml:"snippets,omitempty"`
	Add      []string `yaml:"add,omitempty"`
	Remove   []string `yaml:"remove,omitempty"`
}

// Load reads a .scribe.yaml file from path.
// Missing or empty files are treated as no Scribe activity for the project.
func Load(path string) (*ProjectFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &ProjectFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &ProjectFile{}, nil
	}

	var pf ProjectFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse project file: %w", err)
	}
	return &pf, nil
}

// Save writes a ProjectFile to path atomically.
func Save(path string, pf *ProjectFile) error {
	if pf == nil {
		pf = &ProjectFile{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create project file dir: %w", err)
	}

	data, err := yaml.Marshal(pf)
	if err != nil {
		return fmt.Errorf("encode project file: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save project file: %w", err)
	}
	return nil
}

// Find walks upward from startDir looking for .scribe.yaml.
// It returns an empty string when no project file exists.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat start dir: %w", err)
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		candidate := filepath.Join(dir, Filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat project file: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
