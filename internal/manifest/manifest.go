package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestFilename is the conventional name for a scribe manifest file.
const ManifestFilename = "scribe.toml"

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

// Parse parses scribe.toml content from a byte slice (e.g. fetched from GitHub API).
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Skills == nil {
		m.Skills = make(map[string]Skill)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Encode serializes the manifest to TOML bytes.
func (m *Manifest) Encode() ([]byte, error) {
	// Build an encodable shadow struct. The Skill type has custom UnmarshalTOML
	// which toml.Marshal doesn't invert — use a map[string]any for skills.
	type encodable struct {
		Team    *Team          `toml:"team,omitempty"`
		Package *Package       `toml:"package,omitempty"`
		Skills  map[string]any `toml:"skills"`
		Targets *Targets       `toml:"targets,omitempty"`
	}

	skills := make(map[string]any, len(m.Skills))
	for name, skill := range m.Skills {
		if m.IsLoadout() {
			entry := map[string]any{"source": skill.Source}
			if skill.Path != "" {
				entry["path"] = skill.Path
			}
			if skill.Private {
				entry["private"] = skill.Private
			}
			skills[name] = entry
		} else {
			// Package manifest: plain string path.
			skills[name] = skill.Path
		}
	}

	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(encodable{
		Team:    m.Team,
		Package: m.Package,
		Skills:  skills,
		Targets: m.Targets,
	}); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return buf.Bytes(), nil
}

// Validate checks invariants on a parsed manifest.
func (m *Manifest) Validate() error {
	if m.Team != nil && m.Package != nil {
		return errors.New("manifest cannot have both [team] and [package] sections")
	}
	return nil
}

func (m *Manifest) IsLoadout() bool { return m.Team != nil }
func (m *Manifest) IsPackage() bool { return m.Package != nil }

// ParseOwnerRepo splits an "owner/repo" string and validates it.
func ParseOwnerRepo(s string) (owner, repo string, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo", s)
	}
	return parts[0], parts[1], nil
}
