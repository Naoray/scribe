package manifest

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFilename is the conventional name for a scribe manifest file.
const ManifestFilename = "scribe.yaml"

// LegacyManifestFilename is the old TOML-based manifest filename.
const LegacyManifestFilename = "scribe.toml"

// Manifest represents a parsed scribe.yaml.
// A file with team: is a registry; package: is a skill package.
type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Team       *Team    `yaml:"team,omitempty"`
	Package    *Package `yaml:"package,omitempty"`
	Catalog    []Entry  `yaml:"catalog"`
	Targets    *Targets `yaml:"targets,omitempty"`
}

type Team struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type Package struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description,omitempty"`
	License     string   `yaml:"license,omitempty"`
	Authors     []string `yaml:"authors,omitempty"`
	Repository  string   `yaml:"repository,omitempty"`
}

// EntryTypePackage is the type value for package catalog entries.
const EntryTypePackage = "package"

// Entry represents one item in the catalog list.
type Entry struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source,omitempty"`
	Path        string `yaml:"path,omitempty"`
	Type        string `yaml:"type,omitempty"`
	Install     string `yaml:"install,omitempty"`
	Update      string `yaml:"update,omitempty"`
	Author      string `yaml:"author,omitempty"`
	Description string `yaml:"description,omitempty"`
	Timeout     int    `yaml:"timeout,omitempty"`
	Group       string `yaml:"-"` // display-only, set by marketplace discovery
}

// Maintainer returns the entry's author.
func (e Entry) Maintainer() string {
	return e.Author
}

// IsPackage reports whether this entry is a package-type entry.
func (e Entry) IsPackage() bool {
	return e.Type == EntryTypePackage
}

type Targets struct {
	Default []string `yaml:"default"`
}

// Parse parses scribe.yaml content from a byte slice.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Catalog == nil {
		m.Catalog = []Entry{}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Encode serializes the manifest to YAML bytes.
func (m *Manifest) Encode() ([]byte, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return data, nil
}

// Validate checks invariants on a parsed manifest.
func (m *Manifest) Validate() error {
	if m.APIVersion != "scribe/v1" {
		return fmt.Errorf("unsupported apiVersion %q (expected scribe/v1)", m.APIVersion)
	}
	if m.Kind != "Registry" && m.Kind != "Package" {
		return fmt.Errorf("unknown kind %q (expected Registry or Package)", m.Kind)
	}
	if m.Team != nil && m.Package != nil {
		return errors.New("manifest cannot have both team and package sections")
	}
	if m.Kind == "Registry" && m.Team == nil {
		return errors.New("kind is Registry but no team section present")
	}
	if m.Kind == "Package" && m.Package == nil {
		return errors.New("kind is Package but no package section present")
	}

	seen := make(map[string]bool, len(m.Catalog))
	for _, e := range m.Catalog {
		if e.Name == "" {
			return errors.New("catalog entry has empty name")
		}
		if seen[e.Name] {
			return fmt.Errorf("duplicate catalog entry name %q", e.Name)
		}
		seen[e.Name] = true
		if e.Type != "" && e.Type != EntryTypePackage {
			return fmt.Errorf("unknown entry type %q for %q (expected \"\" or %q)", e.Type, e.Name, EntryTypePackage)
		}
	}

	return nil
}

// IsRegistry reports whether this manifest describes a team registry.
func (m *Manifest) IsRegistry() bool { return m.Team != nil }

// IsLoadout is an alias for IsRegistry (backward compatibility).
func (m *Manifest) IsLoadout() bool { return m.IsRegistry() }

// IsPackage reports whether this manifest describes a skill package.
func (m *Manifest) IsPackage() bool { return m.Package != nil }

// FindByName returns the first catalog entry with the given name, or nil.
func (m *Manifest) FindByName(name string) *Entry {
	for i := range m.Catalog {
		if m.Catalog[i].Name == name {
			return &m.Catalog[i]
		}
	}
	return nil
}

// ParseOwnerRepo splits an "owner/repo" string and validates it.
func ParseOwnerRepo(s string) (owner, repo string, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo", s)
	}
	return parts[0], parts[1], nil
}
