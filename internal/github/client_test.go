package github_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/github"
)

func TestTreeEntryStruct(t *testing.T) {
	// Verify the TreeEntry struct has the fields we need.
	entry := github.TreeEntry{
		Path: "skills/deploy/SKILL.md",
		Type: "blob",
		SHA:  "abc123",
	}
	if entry.Path != "skills/deploy/SKILL.md" {
		t.Errorf("Path: got %q", entry.Path)
	}
	if entry.Type != "blob" {
		t.Errorf("Type: got %q", entry.Type)
	}
}
