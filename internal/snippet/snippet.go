package snippet

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Snippet struct {
	Name        string
	Description string
	Targets     []string
	Source      string
	Body        []byte
	Path        string
}

type frontmatter struct {
	Name        string  `yaml:"name,omitempty"`
	Description string  `yaml:"description,omitempty"`
	Targets     targets `yaml:"targets,omitempty"`
	Source      string  `yaml:"source,omitempty"`
}

func Dir(home string) string {
	return filepath.Join(home, ".scribe", "snippets")
}

func Load(dir, name string) (Snippet, error) {
	path := filepath.Join(dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Snippet{}, fmt.Errorf("snippet %q not found at %s", name, path)
		}
		return Snippet{}, fmt.Errorf("read snippet %q: %w", name, err)
	}
	sn, err := Parse(path, data)
	if err != nil {
		return Snippet{}, err
	}
	if sn.Name == "" {
		sn.Name = name
	}
	if sn.Name != name {
		return Snippet{}, fmt.Errorf("snippet %q declares name %q", name, sn.Name)
	}
	return sn, nil
}

func LoadProject(dir string, names []string) ([]Snippet, error) {
	out := make([]Snippet, 0, len(names))
	for _, name := range names {
		sn, err := Load(dir, name)
		if err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, nil
}

func Parse(path string, data []byte) (Snippet, error) {
	fm, body, err := split(data)
	if err != nil {
		return Snippet{}, fmt.Errorf("parse snippet %s: %w", path, err)
	}
	if fm.Name == "" {
		return Snippet{}, fmt.Errorf("parse snippet %s: missing name", path)
	}
	targets := normalizeTargets(fm.Targets)
	if len(targets) == 0 {
		return Snippet{}, fmt.Errorf("parse snippet %s: missing targets", path)
	}
	return Snippet{
		Name:        fm.Name,
		Description: strings.TrimSpace(fm.Description),
		Targets:     targets,
		Source:      strings.TrimSpace(fm.Source),
		Body:        bytes.TrimLeft(body, "\n"),
		Path:        path,
	}, nil
}

func ContentForBudget(sn Snippet) []byte {
	data, err := yaml.Marshal(frontmatter{Description: sn.Description})
	if err != nil {
		return sn.Body
	}
	return bytes.Join([][]byte{[]byte("---\n"), data, []byte("---\n"), sn.Body}, nil)
}

type ProjectOptions struct {
	CreateTargets bool
}

func Project(projectRoot string, snippets []Snippet, activeTools []string) ([]string, error) {
	return ProjectWithOptions(projectRoot, snippets, activeTools, ProjectOptions{})
}

func ProjectWithOptions(projectRoot string, snippets []Snippet, activeTools []string, opts ProjectOptions) ([]string, error) {
	byTarget := map[string][]Snippet{}
	for _, sn := range snippets {
		for _, target := range expandTargets(projectRoot, sn.Targets, activeTools, opts) {
			byTarget[target] = append(byTarget[target], sn)
		}
	}

	var paths []string
	for _, target := range projectTargets(projectRoot, byTarget) {
		written, err := projectTarget(projectRoot, target, byTarget[target])
		if err != nil {
			return paths, err
		}
		paths = append(paths, written...)
	}
	sort.Strings(paths)
	return paths, nil
}

func TargetsAgent(targets []string, agent string) bool {
	agent = strings.ToLower(strings.TrimSpace(agent))
	for _, target := range normalizeTargets(targets) {
		if target == agent || target == "all" {
			return true
		}
	}
	return false
}

func TargetPath(projectRoot, name, target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "claude":
		return filepath.Join(projectRoot, "CLAUDE.md")
	case "codex":
		return filepath.Join(projectRoot, "AGENTS.md")
	case "cursor":
		return filepath.Join(projectRoot, ".cursorrules")
	default:
		return ""
	}
}

func HasProjection(path string, sn Snippet, target string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	start := "<!-- scribe:start name=" + sn.Name + " "
	end := "<!-- scribe:end name=" + sn.Name + " -->"
	return strings.Contains(string(data), start) && strings.Contains(string(data), end)
}

func split(data []byte) (frontmatter, []byte, error) {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return frontmatter{}, nil, fmt.Errorf("missing YAML frontmatter")
	}
	rest := normalized[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return frontmatter{}, nil, fmt.Errorf("unterminated YAML frontmatter")
	}
	var fm frontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return frontmatter{}, nil, err
	}
	body := rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))
	return fm, body, nil
}

type targets []string

func (t *targets) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*t = targets{value.Value}
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, node := range value.Content {
			if node.Kind != yaml.ScalarNode {
				return fmt.Errorf("targets entries must be strings")
			}
			out = append(out, node.Value)
		}
		*t = out
	default:
		return fmt.Errorf("targets must be a string or list")
	}
	return nil
}

func normalizeTargets(targets []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, target := range targets {
		target = strings.ToLower(strings.TrimSpace(target))
		if target == "" || seen[target] {
			continue
		}
		switch target {
		case "claude", "codex", "cursor", "all":
			seen[target] = true
			out = append(out, target)
		}
	}
	return out
}

func expandTargets(projectRoot string, targets, activeTools []string, opts ProjectOptions) []string {
	seen := map[string]bool{}
	var out []string
	add := func(target string) {
		if seen[target] {
			return
		}
		seen[target] = true
		out = append(out, target)
	}
	for _, target := range targets {
		if strings.EqualFold(target, "all") {
			for _, detected := range detectedTargets(projectRoot, activeTools, opts.CreateTargets) {
				add(detected)
			}
			continue
		}
		add(target)
	}
	return out
}

func projectTarget(projectRoot string, target string, snippets []Snippet) ([]string, error) {
	switch target {
	case "claude", "codex", "cursor":
		written, err := writeManagedBlocks(TargetPath(projectRoot, "", target), snippets)
		if err != nil || written == "" {
			return nil, err
		}
		return []string{written}, nil
	default:
		return nil, nil
	}
}

func RemoveLegacyCursorRule(projectRoot string, sn Snippet) (string, bool, error) {
	path := filepath.Join(projectRoot, ".cursor", "rules", slug(sn.Name)+".mdc")
	if path == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read legacy cursor snippet %s: %w", path, err)
	}
	if bytes.Contains(data, []byte("<!-- scribe-snippet:cursor -->")) {
		return "", false, nil
	}
	_, body, err := split(data)
	if err != nil {
		return "", false, nil
	}
	if !bytes.Equal(bytes.TrimSpace(body), bytes.TrimSpace(sn.Body)) {
		return "", false, nil
	}
	if err := os.Remove(path); err != nil {
		return "", false, fmt.Errorf("remove legacy cursor snippet %s: %w", path, err)
	}
	return path, true, nil
}

func writeManagedBlocks(path string, snippets []Snippet) (string, error) {
	if path == "" {
		return "", nil
	}
	if len(snippets) == 0 && !fileExists(path) {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create snippet target dir: %w", err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read snippet target %s: %w", path, err)
	}
	next := renderManagedBlocks(existing, snippets)
	if len(next) == 0 && len(existing) == 0 {
		return "", nil
	}
	if bytes.Equal(existing, next) {
		return "", nil
	}
	if err := os.WriteFile(path, next, 0o644); err != nil {
		return "", fmt.Errorf("write snippet target %s: %w", path, err)
	}
	return path, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func renderManagedBlocks(existing []byte, snippets []Snippet) []byte {
	cleaned := stripManagedBlocks(existing)
	var blocks bytes.Buffer
	for _, sn := range snippets {
		blocks.Write(managedBlock(sn))
	}
	if blocks.Len() == 0 {
		return cleaned
	}
	if len(bytes.TrimSpace(cleaned)) == 0 {
		return blocks.Bytes()
	}
	switch {
	case bytes.HasSuffix(cleaned, []byte("\n\n")):
	case bytes.HasSuffix(cleaned, []byte("\n")):
		cleaned = append(cleaned, '\n')
	default:
		cleaned = append(cleaned, '\n')
		cleaned = append(cleaned, '\n')
	}
	cleaned = append(cleaned, blocks.Bytes()...)
	return cleaned
}

func stripManagedBlocks(existing []byte) []byte {
	blocks := parseManagedBlocks(existing)
	if len(blocks) == 0 {
		return append([]byte(nil), existing...)
	}
	out := make([]byte, 0, len(existing))
	pos := 0
	for _, block := range blocks {
		out = append(out, existing[pos:block.start]...)
		pos = block.end
		if pos < len(existing) && existing[pos] == '\n' {
			pos++
		}
	}
	out = append(out, existing[pos:]...)
	return out
}

func managedBlock(sn Snippet) []byte {
	sum := sha256.Sum256(sn.Body)
	var b bytes.Buffer
	fmt.Fprintf(&b, "<!-- scribe:start name=%s hash=%x -->\n", sn.Name, sum)
	b.Write(sn.Body)
	if !bytes.HasSuffix(sn.Body, []byte("\n")) {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "<!-- scribe:end name=%s -->\n", sn.Name)
	return b.Bytes()
}

func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func projectTargets(projectRoot string, byTarget map[string][]Snippet) []string {
	seen := map[string]bool{}
	for _, target := range supportedTargets() {
		if len(byTarget[target]) > 0 || fileExists(TargetPath(projectRoot, "", target)) {
			seen[target] = true
		}
	}
	out := make([]string, 0, len(seen))
	for _, target := range supportedTargets() {
		if seen[target] {
			out = append(out, target)
		}
	}
	return out
}

func detectedTargets(projectRoot string, activeTools []string, createTargets bool) []string {
	if !createTargets {
		var out []string
		for _, target := range supportedTargets() {
			if fileExists(TargetPath(projectRoot, "", target)) {
				out = append(out, target)
			}
		}
		return out
	}
	if len(activeTools) == 0 {
		return supportedTargets()
	}
	seen := map[string]bool{}
	var out []string
	for _, tool := range activeTools {
		tool = strings.ToLower(strings.TrimSpace(tool))
		switch tool {
		case "claude", "codex", "cursor":
			if !seen[tool] {
				seen[tool] = true
				out = append(out, tool)
			}
		}
	}
	return out
}

func supportedTargets() []string {
	return []string{"claude", "codex", "cursor"}
}

type managedBlockSpan struct {
	start int
	end   int
}

func parseManagedBlocks(data []byte) []managedBlockSpan {
	var spans []managedBlockSpan
	searchFrom := 0
	for {
		startRel := bytes.Index(data[searchFrom:], []byte("<!-- scribe:start "))
		if startRel < 0 {
			break
		}
		start := searchFrom + startRel
		startCloseRel := bytes.Index(data[start:], []byte("-->"))
		if startCloseRel < 0 {
			break
		}
		startMarker := string(data[start : start+startCloseRel+len("-->")])
		name, ok := markerField(startMarker, "name")
		if !ok {
			searchFrom = start + len("<!-- scribe:start ")
			continue
		}
		endMarker := []byte("<!-- scribe:end name=" + name + " -->")
		bodyStart := start + startCloseRel + len("-->")
		endRel := bytes.Index(data[bodyStart:], endMarker)
		if endRel < 0 {
			break
		}
		end := bodyStart + endRel + len(endMarker)
		spans = append(spans, managedBlockSpan{start: start, end: end})
		searchFrom = end
	}
	return spans
}

func markerField(marker, key string) (string, bool) {
	prefix := key + "="
	for _, field := range strings.Fields(marker) {
		field = strings.TrimSuffix(field, "-->")
		if strings.HasPrefix(field, prefix) {
			value := strings.TrimPrefix(field, prefix)
			return strings.TrimSpace(value), value != ""
		}
	}
	return "", false
}
