package snippet

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Snippet is a named rules block that can be injected into agent rule files.
type Snippet struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Targets     []string `yaml:"targets,omitempty"`
	Source      *Source  `yaml:"source,omitempty"`
	Body        string   `yaml:"-"`
}

// Source records the registry source for a snippet.
type Source struct {
	Registry string `yaml:"registry"`
	Rev      string `yaml:"rev,omitempty"`
}

type snippetFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Targets     []string `yaml:"targets,omitempty"`
	Source      *Source  `yaml:"source,omitempty"`
}

func (fm *snippetFrontmatter) UnmarshalYAML(value *yaml.Node) error {
	type noTargets struct {
		Name        string  `yaml:"name"`
		Description string  `yaml:"description,omitempty"`
		Source      *Source `yaml:"source,omitempty"`
	}
	var raw noTargets
	if err := value.Decode(&raw); err != nil {
		return err
	}

	fm.Name = raw.Name
	fm.Description = raw.Description
	fm.Source = raw.Source

	targetsNode := mappingValue(value, "targets")
	if targetsNode == nil {
		return nil
	}
	switch targetsNode.Kind {
	case yaml.ScalarNode:
		if targetsNode.Value != "all" {
			return fmt.Errorf("targets scalar must be %q", "all")
		}
		fm.Targets = []string{"all"}
	case yaml.SequenceNode:
		targets := make([]string, 0, len(targetsNode.Content))
		for _, item := range targetsNode.Content {
			if item.Kind != yaml.ScalarNode {
				return errors.New("targets entries must be strings")
			}
			targets = append(targets, item.Value)
		}
		fm.Targets = targets
	default:
		return errors.New("targets must be a list or all")
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// Load reads a single snippet markdown file from path.
func Load(path string) (*Snippet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snippet: %w", err)
	}

	fmData, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var fm snippetFrontmatter
	if err := yaml.Unmarshal(fmData, &fm); err != nil {
		return nil, fmt.Errorf("parse snippet frontmatter: %w", err)
	}

	return &Snippet{
		Name:        fm.Name,
		Description: fm.Description,
		Targets:     fm.Targets,
		Source:      fm.Source,
		Body:        string(body),
	}, nil
}

// LoadAll reads all *.md snippet files in dir, keyed by snippet name.
func LoadAll(dir string) (map[string]*Snippet, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]*Snippet{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snippets dir: %w", err)
	}

	snippets := make(map[string]*Snippet)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		s, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		snippets[s.Name] = s
	}
	return snippets, nil
}

// Save writes a snippet markdown file to path atomically.
func Save(path string, s *Snippet) error {
	if s == nil {
		s = &Snippet{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create snippet dir: %w", err)
	}

	fm := snippetFrontmatter{
		Name:        s.Name,
		Description: s.Description,
		Targets:     s.Targets,
		Source:      s.Source,
	}
	fmData, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("encode snippet frontmatter: %w", err)
	}

	var data bytes.Buffer
	data.WriteString("---\n")
	data.Write(fmData)
	data.WriteString("---\n")
	data.WriteString(s.Body)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write snippet: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save snippet: %w", err)
	}
	return nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, nil, errors.New("parse snippet: missing frontmatter")
	}

	firstLineEnd := bytes.IndexByte(data, '\n')
	if firstLineEnd < 0 {
		return nil, nil, errors.New("parse snippet: missing frontmatter")
	}
	cursor := firstLineEnd + 1
	for cursor <= len(data) {
		next := bytes.IndexByte(data[cursor:], '\n')
		lineEnd := len(data)
		afterLine := len(data)
		if next >= 0 {
			lineEnd = cursor + next
			afterLine = lineEnd + 1
		}

		line := bytes.TrimSuffix(data[cursor:lineEnd], []byte("\r"))
		if bytes.Equal(line, []byte("---")) {
			return data[firstLineEnd+1 : cursor], data[afterLine:], nil
		}
		if next < 0 {
			break
		}
		cursor = afterLine
	}

	return nil, nil, errors.New("parse snippet: missing closing frontmatter")
}
