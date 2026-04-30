package kit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Kit is a named bundle of Scribe skills.
type Kit struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Skills      []string `yaml:"skills"`
	Source      *Source  `yaml:"source,omitempty"`
}

// Source records the registry source for a kit.
type Source struct {
	Registry string `yaml:"registry"`
	Rev      string `yaml:"rev,omitempty"`
}

// Load reads a single kit YAML file from path.
func Load(path string) (*Kit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kit: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &Kit{}, nil
	}

	var kit Kit
	if err := yaml.Unmarshal(data, &kit); err != nil {
		return nil, fmt.Errorf("parse kit: %w", err)
	}
	if kit.Skills == nil {
		kit.Skills = []string{}
	}
	return &kit, nil
}

// LoadAll reads all *.yaml kit files in dir, keyed by kit name.
func LoadAll(dir string) (map[string]*Kit, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]*Kit{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read kits dir: %w", err)
	}

	kits := make(map[string]*Kit)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		kit, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		kits[kit.Name] = kit
	}
	return kits, nil
}

// Save writes a kit YAML file to path atomically.
func Save(path string, kit *Kit) error {
	if kit == nil {
		kit = &Kit{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create kit dir: %w", err)
	}

	data, err := yaml.Marshal(kit)
	if err != nil {
		return fmt.Errorf("encode kit: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write kit: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save kit: %w", err)
	}
	return nil
}
