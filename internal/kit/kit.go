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
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description,omitempty"`
	Skills       []string          `yaml:"-"`
	SkillAliases map[string]string `yaml:"-"`
	MCPServers   []string          `yaml:"mcp_servers,omitempty"`
	Source       *Source           `yaml:"source,omitempty"`
}

type kitYAML struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description,omitempty"`
	Skills      []skillRef `yaml:"skills"`
	MCPServers  []string   `yaml:"mcp_servers,omitempty"`
	Source      *Source    `yaml:"source,omitempty"`
}

type skillRef struct {
	Ref   string `yaml:"ref"`
	Alias string `yaml:"alias,omitempty"`
}

func (s *skillRef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = skillRef{Ref: value.Value}
		return nil
	case yaml.MappingNode:
		var raw struct {
			Ref   string `yaml:"ref"`
			Alias string `yaml:"alias,omitempty"`
		}
		if err := value.Decode(&raw); err != nil {
			return err
		}
		if raw.Ref == "" {
			return errors.New("skill mapping must include ref")
		}
		*s = skillRef{Ref: raw.Ref, Alias: raw.Alias}
		return nil
	default:
		return fmt.Errorf("skill ref must be a string or mapping, got YAML kind %d", value.Kind)
	}
}

func (s skillRef) MarshalYAML() (any, error) {
	if s.Alias == "" {
		return s.Ref, nil
	}
	return struct {
		Ref   string `yaml:"ref"`
		Alias string `yaml:"alias,omitempty"`
	}{Ref: s.Ref, Alias: s.Alias}, nil
}

func (k *Kit) UnmarshalYAML(value *yaml.Node) error {
	var raw kitYAML
	if err := value.Decode(&raw); err != nil {
		return err
	}
	k.Name = raw.Name
	k.Description = raw.Description
	k.Skills = make([]string, 0, len(raw.Skills))
	k.SkillAliases = nil
	for _, ref := range raw.Skills {
		k.Skills = append(k.Skills, ref.Ref)
		if ref.Alias != "" {
			if k.SkillAliases == nil {
				k.SkillAliases = map[string]string{}
			}
			k.SkillAliases[ref.Ref] = ref.Alias
		}
	}
	k.MCPServers = raw.MCPServers
	k.Source = raw.Source
	return nil
}

func (k Kit) MarshalYAML() (any, error) {
	raw := kitYAML{
		Name:        k.Name,
		Description: k.Description,
		Skills:      make([]skillRef, 0, len(k.Skills)),
		MCPServers:  k.MCPServers,
		Source:      k.Source,
	}
	for _, ref := range k.Skills {
		raw.Skills = append(raw.Skills, skillRef{Ref: ref, Alias: k.SkillAliases[ref]})
	}
	return raw, nil
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
	return Parse(data)
}

// Parse reads a kit YAML document from bytes.
func Parse(data []byte) (*Kit, error) {
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
	if kit.MCPServers == nil {
		kit.MCPServers = []string{}
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
