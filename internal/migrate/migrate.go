package migrate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Naoray/scribe/internal/manifest"
)

type legacyManifest struct {
	Team    *legacyTeam            `toml:"team"`
	Package *legacyPackage         `toml:"package"`
	Skills  map[string]legacySkill `toml:"skills"`
	Targets *legacyTargets         `toml:"targets"`
}

type legacyTeam struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

type legacyPackage struct {
	Name        string   `toml:"name"`
	Version     string   `toml:"version"`
	Description string   `toml:"description"`
	License     string   `toml:"license"`
	Authors     []string `toml:"authors"`
	Repository  string   `toml:"repository"`
}

type legacySkill struct {
	Source  string `toml:"source"`
	Path    string `toml:"path"`
	Private bool   `toml:"private"`
}

func (s *legacySkill) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case string:
		s.Path = v
	case map[string]any:
		if src, ok := v["source"].(string); ok {
			s.Source = src
		}
		if path, ok := v["path"].(string); ok {
			s.Path = path
		}
		if private, ok := v["private"].(bool); ok {
			s.Private = private
		}
	default:
		return fmt.Errorf("skill entry must be a string or table, got %T", data)
	}
	return nil
}

type legacyTargets struct {
	Default []string `toml:"default"`
}

// Convert parses a scribe.toml byte slice and returns the equivalent Manifest.
func Convert(data []byte) (*manifest.Manifest, error) {
	var old legacyManifest
	if err := toml.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("parse legacy manifest: %w", err)
	}

	m := &manifest.Manifest{
		APIVersion: "scribe/v1",
	}

	if old.Team != nil {
		m.Kind = "Registry"
		m.Team = &manifest.Team{
			Name:        old.Team.Name,
			Description: old.Team.Description,
		}
	}

	if old.Package != nil {
		m.Kind = "Package"
		m.Package = &manifest.Package{
			Name:        old.Package.Name,
			Version:     old.Package.Version,
			Description: old.Package.Description,
			License:     old.Package.License,
			Authors:     old.Package.Authors,
			Repository:  old.Package.Repository,
		}
	}

	// Sort alphabetically for deterministic output.
	names := make([]string, 0, len(old.Skills))
	for name := range old.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		skill := old.Skills[name]
		entry := manifest.Entry{
			Name:   name,
			Source: skill.Source,
			Path:   skill.Path,
			Author: inferAuthor(skill),
		}
		m.Catalog = append(m.Catalog, entry)
	}

	if m.Catalog == nil {
		m.Catalog = []manifest.Entry{}
	}

	if old.Targets != nil {
		m.Targets = &manifest.Targets{Default: old.Targets.Default}
	}

	return m, nil
}

func inferAuthor(skill legacySkill) string {
	if skill.Path != "" {
		parts := strings.SplitN(skill.Path, "/", 2)
		if len(parts) == 2 {
			return parts[0]
		}
	}
	if skill.Source != "" {
		src, err := manifest.ParseSource(skill.Source)
		if err == nil {
			return src.Owner
		}
	}
	return ""
}
