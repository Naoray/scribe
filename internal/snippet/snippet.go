package snippet

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Snippet struct {
	Name        string
	Description string
	Targets     []string
	Body        []byte
	Path        string
}

type frontmatter struct {
	Name        string   `yaml:"name,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Targets     []string `yaml:"targets,omitempty"`
}

type cursorFrontmatter struct {
	Description string `yaml:"description,omitempty"`
	AlwaysApply bool   `yaml:"alwaysApply"`
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

func Project(projectRoot string, snippets []Snippet, activeTools []string) ([]string, error) {
	byTarget := map[string][]Snippet{}
	for _, sn := range snippets {
		for _, target := range expandTargets(sn.Targets, activeTools) {
			byTarget[target] = append(byTarget[target], sn)
		}
	}

	var paths []string
	for _, target := range concreteTargets(activeTools) {
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
	case "gemini":
		return filepath.Join(projectRoot, "GEMINI.md")
	case "cursor":
		return filepath.Join(projectRoot, ".cursor", "rules", slug(name)+".mdc")
	default:
		return ""
	}
}

func HasProjection(path string, sn Snippet, target string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if strings.EqualFold(target, "cursor") {
		return bytes.Contains(data, []byte("<!-- scribe-snippet:cursor -->")) && bytes.Contains(data, sn.Body)
	}
	start := "<!-- scribe-snippet:" + sn.Name + ":start "
	end := "<!-- scribe-snippet:" + sn.Name + ":end -->"
	return strings.Contains(string(data), start) && strings.Contains(string(data), end)
}

func concreteTargets(activeTools []string) []string {
	seen := map[string]bool{
		"claude": true,
		"codex":  true,
		"cursor": true,
		"gemini": true,
	}
	for _, tool := range activeTools {
		tool = strings.ToLower(strings.TrimSpace(tool))
		switch tool {
		case "claude", "codex", "cursor", "gemini":
			seen[tool] = true
		}
	}
	out := make([]string, 0, len(seen))
	for target := range seen {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
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

func normalizeTargets(targets []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, target := range targets {
		target = strings.ToLower(strings.TrimSpace(target))
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

func expandTargets(targets, activeTools []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(target string) {
		target = strings.ToLower(strings.TrimSpace(target))
		if target == "" || seen[target] {
			return
		}
		switch target {
		case "claude", "codex", "cursor", "gemini":
			seen[target] = true
			out = append(out, target)
		}
	}
	for _, target := range targets {
		if strings.EqualFold(target, "all") {
			for _, tool := range activeTools {
				add(tool)
			}
			continue
		}
		add(target)
	}
	sort.Strings(out)
	return out
}

func projectTarget(projectRoot string, target string, snippets []Snippet) ([]string, error) {
	switch target {
	case "claude", "codex", "gemini":
		written, err := writeManagedBlocks(TargetPath(projectRoot, "", target), snippets)
		if err != nil || written == "" {
			return nil, err
		}
		return []string{written}, nil
	case "cursor":
		return writeCursorRules(projectRoot, snippets)
	default:
		return nil, nil
	}
}

func writeCursorRules(projectRoot string, snippets []Snippet) ([]string, error) {
	rulesDir := filepath.Join(projectRoot, ".cursor", "rules")
	current := map[string]bool{}
	var paths []string
	for _, sn := range snippets {
		path := TargetPath(projectRoot, sn.Name, "cursor")
		current[path] = true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return paths, fmt.Errorf("create cursor snippets dir: %w", err)
		}
		content, err := cursorRule(sn)
		if err != nil {
			return paths, err
		}
		existing, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return paths, fmt.Errorf("read cursor snippet %q: %w", sn.Name, err)
		}
		if bytes.Equal(existing, content) {
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return paths, fmt.Errorf("write cursor snippet %q: %w", sn.Name, err)
		}
		paths = append(paths, path)
	}
	entries, err := os.ReadDir(rulesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return paths, nil
	}
	if err != nil {
		return paths, fmt.Errorf("read cursor snippets dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".mdc" {
			continue
		}
		path := filepath.Join(rulesDir, entry.Name())
		if current[path] {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return paths, fmt.Errorf("read cursor snippet %s: %w", path, err)
		}
		if bytes.Contains(data, []byte("<!-- scribe-snippet:cursor -->")) {
			if err := os.Remove(path); err != nil {
				return paths, fmt.Errorf("remove stale cursor snippet %s: %w", path, err)
			}
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func RemoveLegacyCursorRule(projectRoot string, sn Snippet) (string, bool, error) {
	path := TargetPath(projectRoot, sn.Name, "cursor")
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

func cursorRule(sn Snippet) ([]byte, error) {
	fm, err := yaml.Marshal(cursorFrontmatter{
		Description: sn.Description,
		AlwaysApply: false,
	})
	if err != nil {
		return nil, fmt.Errorf("encode cursor snippet %q: %w", sn.Name, err)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n")
	b.WriteString("<!-- scribe-snippet:cursor -->\n\n")
	b.Write(sn.Body)
	if !bytes.HasSuffix(sn.Body, []byte("\n")) {
		b.WriteString("\n")
	}
	return b.Bytes(), nil
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
	next := renderManagedBlocks(string(existing), snippets)
	if next == "" && len(existing) == 0 {
		return "", nil
	}
	if string(existing) == next {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return "", fmt.Errorf("write snippet target %s: %w", path, err)
	}
	return path, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func renderManagedBlocks(existing string, snippets []Snippet) string {
	cleaned := stripManagedBlocks(existing)
	var blocks strings.Builder
	for _, sn := range snippets {
		blocks.WriteString(managedBlock(sn))
	}
	if blocks.Len() == 0 {
		return cleaned
	}
	if strings.TrimSpace(cleaned) == "" {
		return blocks.String()
	}
	if !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}
	return cleaned + "\n" + blocks.String()
}

func stripManagedBlocks(existing string) string {
	re := regexp.MustCompile(`(?s)<!-- scribe-snippet:[^:]+:start [^>]*-->.*?<!-- scribe-snippet:[^:]+:end -->\n?`)
	return strings.TrimRight(re.ReplaceAllString(existing, ""), "\n")
}

func managedBlock(sn Snippet) string {
	sum := sha256.Sum256(sn.Body)
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- scribe-snippet:%s:start sha256=%x -->\n", sn.Name, sum)
	b.Write(sn.Body)
	if !bytes.HasSuffix(sn.Body, []byte("\n")) {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "<!-- scribe-snippet:%s:end -->\n", sn.Name)
	return b.String()
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
