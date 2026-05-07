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
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Targets     []string `yaml:"targets"`
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
	var b strings.Builder
	b.WriteString("---\n")
	if sn.Description != "" {
		b.WriteString("description: ")
		b.WriteString(sn.Description)
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	b.Write(sn.Body)
	return []byte(b.String())
}

func Project(projectRoot string, snippets []Snippet, activeTools []string) ([]string, error) {
	var paths []string
	for _, sn := range snippets {
		for _, target := range expandTargets(sn.Targets, activeTools) {
			written, err := projectOne(projectRoot, sn, target)
			if err != nil {
				return paths, err
			}
			if written != "" {
				paths = append(paths, written)
			}
		}
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
		return bytes.Contains(data, sn.Body)
	}
	start := "<!-- scribe-snippet:" + sn.Name + ":start "
	end := "<!-- scribe-snippet:" + sn.Name + ":end -->"
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

func projectOne(projectRoot string, sn Snippet, target string) (string, error) {
	switch target {
	case "claude":
		return writeManagedBlock(TargetPath(projectRoot, sn.Name, target), sn)
	case "codex":
		return writeManagedBlock(TargetPath(projectRoot, sn.Name, target), sn)
	case "gemini":
		return writeManagedBlock(TargetPath(projectRoot, sn.Name, target), sn)
	case "cursor":
		return writeCursorRule(projectRoot, sn)
	default:
		return "", nil
	}
}

func writeCursorRule(projectRoot string, sn Snippet) (string, error) {
	path := TargetPath(projectRoot, sn.Name, "cursor")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create cursor snippets dir: %w", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	if sn.Description != "" {
		b.WriteString("description: ")
		b.WriteString(sn.Description)
		b.WriteString("\n")
	}
	b.WriteString("alwaysApply: false\n")
	b.WriteString("---\n\n")
	b.Write(sn.Body)
	if !bytes.HasSuffix(sn.Body, []byte("\n")) {
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write cursor snippet %q: %w", sn.Name, err)
	}
	return path, nil
}

func writeManagedBlock(path string, sn Snippet) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create snippet target dir: %w", err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read snippet target %s: %w", path, err)
	}
	next := replaceBlock(string(existing), sn)
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return "", fmt.Errorf("write snippet target %s: %w", path, err)
	}
	return path, nil
}

func replaceBlock(existing string, sn Snippet) string {
	block := managedBlock(sn)
	re := regexp.MustCompile(`(?s)<!-- scribe-snippet:` + regexp.QuoteMeta(sn.Name) + `:start [^>]*-->.*?<!-- scribe-snippet:` + regexp.QuoteMeta(sn.Name) + `:end -->\n?`)
	if re.MatchString(existing) {
		return re.ReplaceAllString(existing, block)
	}
	if strings.TrimSpace(existing) == "" {
		return block
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + block
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
