package skillmd

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Doc is the parsed and normalized view of a SKILL.md file.
type Doc struct {
	Name        string
	Description string
	Body        string
	Changed     bool
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Normalize parses SKILL.md content and returns a canonical representation.
//
// It rewrites frontmatter deterministically, preserving unrelated keys while
// normalizing name and description. Missing name is filled from dirName and
// missing description is filled from the first prose paragraph in the body.
func Normalize(dirName string, content []byte) (Doc, []byte, error) {
	body := normalizeLineEndings(content)
	doc := Doc{Body: string(body)}

	fm, body, hasFrontmatter, err := splitFrontmatter(body)
	if err != nil {
		return Doc{}, nil, err
	}
	doc.Body = string(body)

	if hasFrontmatter {
		parsed, err := parseFrontmatter(fm)
		if err != nil {
			return Doc{}, nil, err
		}
		doc.Name = strings.TrimSpace(parsed.Name)
		doc.Description = strings.TrimSpace(parsed.Description)
	}

	if doc.Name == "" {
		doc.Name = dirName
	}
	if doc.Description == "" {
		doc.Description = ExtractFallbackDescription(doc.Body)
	}

	normalized, err := renderNormalized(doc.Name, doc.Description, doc.Body, hasFrontmatter, fm)
	if err != nil {
		return Doc{}, nil, err
	}
	doc.Changed = !bytes.Equal(content, normalized)
	return doc, normalized, nil
}

// ExtractFallbackDescription returns the first prose paragraph in body.
//
// It skips headings, list items, blockquotes, tables, and fenced code blocks.
// The returned paragraph has collapsed whitespace.
func ExtractFallbackDescription(body string) string {
	bodyBytes := normalizeLineEndings([]byte(body))
	lines := strings.Split(string(bodyBytes), "\n")

	inFence := false
	var paragraph []string

	flush := func() string {
		if len(paragraph) == 0 {
			return ""
		}
		text := strings.Join(paragraph, " ")
		return strings.Join(strings.Fields(text), " ")
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if isFenceLine(line) {
			inFence = !inFence
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		if inFence {
			continue
		}
		if line == "" {
			if desc := flush(); desc != "" {
				return desc
			}
			paragraph = nil
			continue
		}
		if isIgnorableBlockLine(line) {
			if len(paragraph) > 0 {
				if desc := flush(); desc != "" {
					return desc
				}
				paragraph = nil
			}
			continue
		}
		paragraph = append(paragraph, line)
	}

	return flush()
}

func normalizeLineEndings(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

type parsedFrontmatter struct {
	Name        string
	Description string
	Extra       *yaml.Node
}

func parseFrontmatter(fm []byte) (parsedFrontmatter, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(fm, &node); err != nil {
		return parsedFrontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	mapping, err := frontmatterMappingNode(&node)
	if err != nil {
		return parsedFrontmatter{}, err
	}

	parsed := parsedFrontmatter{Extra: mapping}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		value := mapping.Content[i+1]
		switch key.Value {
		case "name":
			parsed.Name = scalarString(value)
		case "description":
			parsed.Description = scalarString(value)
		}
	}
	return parsed, nil
}

func frontmatterMappingNode(node *yaml.Node) (*yaml.Node, error) {
	if node == nil {
		return nil, fmt.Errorf("parse frontmatter: empty frontmatter")
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return nil, fmt.Errorf("parse frontmatter: invalid document structure")
		}
		return frontmatterMappingNode(node.Content[0])
	case yaml.MappingNode:
		return node, nil
	default:
		return nil, fmt.Errorf("parse frontmatter: frontmatter must be a mapping")
	}
}

func scalarString(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func splitFrontmatter(content []byte) ([]byte, []byte, bool, error) {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return nil, content, false, nil
	}

	lines := bytes.Split(content, []byte("\n"))
	if len(lines) < 2 {
		return nil, nil, false, fmt.Errorf("parse frontmatter: unterminated frontmatter")
	}

	var fmLines [][]byte
	bodyStart := -1
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(string(line)) == "---" {
			bodyStart = i + 1
			break
		}
		fmLines = append(fmLines, line)
	}
	if bodyStart < 0 {
		return nil, nil, false, fmt.Errorf("parse frontmatter: unterminated frontmatter")
	}

	body := bytes.Join(lines[bodyStart:], []byte("\n"))
	body = bytes.TrimPrefix(body, []byte("\n"))

	return bytes.Join(fmLines, []byte("\n")), body, true, nil
}

func renderNormalized(name, description, body string, hasFrontmatter bool, fm []byte) ([]byte, error) {
	fmBytes, err := canonicalFrontmatter(name, description, hasFrontmatter, fm)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(fmBytes)
	if !bytes.HasSuffix(fmBytes, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.WriteString("---\n")
	if body != "" {
		out.WriteByte('\n')
		out.WriteString(strings.TrimLeft(body, "\n"))
	}

	return out.Bytes(), nil
}

func canonicalFrontmatter(name, description string, hasFrontmatter bool, fm []byte) ([]byte, error) {
	if !hasFrontmatter {
		return yaml.Marshal(frontmatter{Name: name, Description: description})
	}

	parsed, err := parseFrontmatter(fm)
	if err != nil {
		return nil, err
	}

	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "description"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: description},
	)

	if parsed.Extra != nil {
		for i := 0; i+1 < len(parsed.Extra.Content); i += 2 {
			key := parsed.Extra.Content[i]
			value := parsed.Extra.Content[i+1]
			if key.Value == "name" || key.Value == "description" {
				continue
			}
			mapping.Content = append(mapping.Content, key, value)
		}
	}

	return yaml.Marshal(mapping)
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func isIgnorableBlockLine(line string) bool {
	if line == "" {
		return false
	}
	switch {
	case strings.HasPrefix(line, "#"):
		return true
	case strings.HasPrefix(line, ">"):
		return true
	case strings.HasPrefix(line, "|"):
		return true
	case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "), strings.HasPrefix(line, "+ "):
		return true
	case isOrderedListItem(line):
		return true
	case line == "---", line == "***", line == "___":
		return true
	default:
		return false
	}
}

func isOrderedListItem(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return false
	}
	switch line[i] {
	case '.', ')':
		return line[i+1] == ' '
	default:
		return false
	}
}
