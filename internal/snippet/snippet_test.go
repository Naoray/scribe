package snippet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadParsesFrontmatterAndBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terse-output.md")
	content := `---
name: terse-output
description: Brief description
targets: [claude, codex, cursor]
source:
  registry: owner/repo
  rev: abc123
---
# Rules

Keep output terse.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Name != "terse-output" {
		t.Fatalf("Name = %q, want terse-output", got.Name)
	}
	if got.Description != "Brief description" {
		t.Fatalf("Description = %q", got.Description)
	}
	if !reflect.DeepEqual(got.Targets, []string{"claude", "codex", "cursor"}) {
		t.Fatalf("Targets = %#v", got.Targets)
	}
	if got.Source == nil || got.Source.Registry != "owner/repo" || got.Source.Rev != "abc123" {
		t.Fatalf("Source = %#v", got.Source)
	}
	if got.Body != "# Rules\n\nKeep output terse.\n" {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestLoadParsesAllTargetSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "all.md")
	content := `---
name: all-targets
targets: all
---
Body
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.Targets, []string{"all"}) {
		t.Fatalf("Targets = %#v, want all sentinel", got.Targets)
	}
}

func TestLoadRequiresFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.md")
	if err := os.WriteFile(path, []byte("# No frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "missing frontmatter") {
		t.Fatalf("Load err = %v, want missing frontmatter", err)
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.md")
	content := `---
name: [unterminated
---
Body
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "parse snippet frontmatter") {
		t.Fatalf("Load err = %v, want parse snippet frontmatter", err)
	}
}

func TestLoadAllReadsMarkdownSnippetsByName(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"one.md":   "---\nname: one\n---\nOne\n",
		"two.md":   "---\nname: two\n---\nTwo\n",
		"skip.txt": "ignored",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	got, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 2 || got["one"].Body != "One\n" || got["two"].Body != "Two\n" {
		t.Fatalf("LoadAll = %#v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "round-trip.md")
	want := &Snippet{
		Name:        "round-trip",
		Description: "Round trip snippet",
		Targets:     []string{"claude", "codex"},
		Source:      &Source{Registry: "owner/repo", Rev: "abc123"},
		Body:        "Line one\nLine two\n",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load after Save = %#v, want %#v", got, want)
	}
}
