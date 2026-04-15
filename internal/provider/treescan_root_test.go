package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/provider"
)

func TestScanTreeForSkills_RootLevelPath(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "SKILL.md", Type: "blob", SHA: "abc"},
	}

	entries := provider.ScanTreeForSkills(tree, "Naoray", "scribe")
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Path != "SKILL.md" {
		t.Errorf("root-level entry should record Path=\"SKILL.md\", got %q", entries[0].Path)
	}
	if entries[0].Name != "scribe" {
		t.Errorf("root-level entry Name should fall back to repo name, got %q", entries[0].Name)
	}
}

func TestScanTreeForSkills_NoEntryWithDotPath(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "SKILL.md", Type: "blob", SHA: "abc"},
		{Path: "sub/SKILL.md", Type: "blob", SHA: "def"},
	}

	entries := provider.ScanTreeForSkills(tree, "Naoray", "scribe")
	for _, entry := range entries {
		if entry.Path == "." {
			t.Errorf("no catalog entry should use Path=\".\"; got entry %+v", entry)
		}
	}
}
