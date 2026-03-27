package manifest

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// Manifest represents a parsed scribe.toml.
// A file with [team] is a loadout; [package] is a skill package.
type Manifest struct {
	Team    *Team            `toml:"team"`
	Package *Package         `toml:"package"`
	Skills  map[string]Skill `toml:"skills"`
	Targets *Targets         `toml:"targets"`
}

type Team struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

type Package struct {
	Name        string   `toml:"name"`
	Version     string   `toml:"version"`
	Description string   `toml:"description"`
	License     string   `toml:"license"`
	Authors     []string `toml:"authors"`
	Repository  string   `toml:"repository"`
}

// Skill represents one entry in the [skills] table.
// In a team loadout: { source = "github:owner/repo@ref", path = "user/skill-name" }
// In a package manifest: plain string "skills/review/SKILL.md" (decoded via UnmarshalTOML)
type Skill struct {
	Source  string `toml:"source"`
	Path    string `toml:"path"`
	Private bool   `toml:"private"`
}

// UnmarshalTOML handles both plain strings (package manifests) and
// inline tables (team loadouts) for skill entries.
func (s *Skill) UnmarshalTOML(data any) error {
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

// Maintainer infers who owns this skill:
// - Team-repo paths like "krishan/deploy" → "krishan"
// - External sources like "github:garrytan/gstack@v1" → "garrytan"
func (s Skill) Maintainer() string {
	if s.Path != "" {
		parts := strings.SplitN(s.Path, "/", 2)
		if len(parts) == 2 {
			return parts[0]
		}
	}
	if s.Source != "" {
		src, err := ParseSource(s.Source)
		if err == nil {
			return src.Owner
		}
	}
	return ""
}

type Targets struct {
	Default []string `toml:"default"`
}

// Load parses a scribe.toml file from disk.
func Load(path string) (*Manifest, error) {
	var m Manifest
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}

// Parse parses scribe.toml content from a byte slice (e.g. fetched from GitHub API).
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

func (m *Manifest) IsLoadout() bool  { return m.Team != nil }
func (m *Manifest) IsPackage() bool  { return m.Package != nil }
