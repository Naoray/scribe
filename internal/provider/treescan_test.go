package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/provider"
)

func TestScanTreeForSkills(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "skills/deploy/SKILL.md", Type: "blob"},
		{Path: "skills/deploy/scripts/run.sh", Type: "blob"},
		{Path: "skills/lint/SKILL.md", Type: "blob"},
		{Path: "docs/README.md", Type: "blob"},
		{Path: "SKILL.md", Type: "blob"}, // root-level SKILL.md
		{Path: "nested/deep/tool/SKILL.md", Type: "blob"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "community-skills")

	if len(entries) != 4 {
		t.Fatalf("entries: got %d, want 4", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
		if e.Source != "github:acme/community-skills@HEAD" {
			t.Errorf("source for %s: got %q", e.Name, e.Source)
		}
		if e.Author != "acme" {
			t.Errorf("author for %s: got %q", e.Name, e.Author)
		}
	}

	if !names["deploy"] {
		t.Error("expected deploy skill")
	}
	if !names["lint"] {
		t.Error("expected lint skill")
	}
	if !names["community-skills"] {
		t.Error("expected root-level skill named after repo")
	}
	if !names["tool"] {
		t.Error("expected nested/deep/tool skill")
	}
}

func TestScanTreeForSkillsEmpty(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "README.md", Type: "blob"},
		{Path: "src/main.go", Type: "blob"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "no-skills")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestScanTreeSkipsTreeEntryType(t *testing.T) {
	// SKILL.md entry with type "tree" should be ignored.
	tree := []provider.TreeEntry{
		{Path: "skills/deploy/SKILL.md", Type: "tree"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "repo")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for tree type, got %d", len(entries))
	}
}
