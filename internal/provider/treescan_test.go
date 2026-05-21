package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
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

func TestScanTreeForSkillsAgentsLayout(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".agents/skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", ".agents/skills/foo")
}

func TestScanTreeForSkillsDedupsMirroredAgentToolDirs(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".claude/skills/foo/SKILL.md", Type: "blob"},
		{Path: ".codex/skills/foo/SKILL.md", Type: "blob"},
		{Path: ".agents/skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", ".agents/skills/foo")
}

func TestScanTreeForSkillsKeepsSingleMirrorWithoutCanonical(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".claude/skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", ".claude/skills/foo")
}

func TestScanTreeForSkillsPrefersTopLevelSkillDirectory(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".agents/skills/foo/SKILL.md", Type: "blob"},
		{Path: "foo/SKILL.md", Type: "blob"},
		{Path: "skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", ".agents/skills/foo")

	entries = provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".claude/skills/foo/SKILL.md", Type: "blob"},
		{Path: "foo/SKILL.md", Type: "blob"},
		{Path: "skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", "foo")
}

func TestScanTreeForSkillsPrefersRootSkillFileOverMirror(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".claude/skills/repo/SKILL.md", Type: "blob"},
		{Path: "SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "repo", "SKILL.md")
}

func TestScanTreeForSkillsDedupsDistinctSkills(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".codex/skills/foo/SKILL.md", Type: "blob"},
		{Path: ".claude/skills/bar/SKILL.md", Type: "blob"},
		{Path: ".agents/skills/foo/SKILL.md", Type: "blob"},
		{Path: "skills/bar/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	if len(entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(entries))
	}

	byName := entriesByName(entries)
	if byName["foo"].Path != ".agents/skills/foo" {
		t.Fatalf("foo path = %q", byName["foo"].Path)
	}
	if byName["bar"].Path != "skills/bar" {
		t.Fatalf("bar path = %q", byName["bar"].Path)
	}
}

func TestScanTreeForSkillsMirrorTieBreaksByPath(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: ".codex/skills/foo/SKILL.md", Type: "blob"},
		{Path: ".claude/skills/foo/SKILL.md", Type: "blob"},
	}, "acme", "repo")

	assertTreeSkill(t, entries, "foo", ".claude/skills/foo")
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

func TestEnrichTreeSkillEntryPreservesSourceAndPath(t *testing.T) {
	entries := provider.ScanTreeForSkills([]provider.TreeEntry{
		{Path: "skills/nextjs/SKILL.md", Type: "blob"},
	}, "vercel-labs", "agent-skills")
	enriched, err := provider.EnrichTreeSkillEntry(entries[0], []byte(`---
name: next-js
description: Build Next.js apps.
author: vercel
---
# Next.js
`))
	if err != nil {
		t.Fatalf("EnrichTreeSkillEntry: %v", err)
	}
	if enriched.Name != "next-js" {
		t.Fatalf("Name = %q", enriched.Name)
	}
	if enriched.Description != "Build Next.js apps." {
		t.Fatalf("Description = %q", enriched.Description)
	}
	if enriched.Author != "vercel" {
		t.Fatalf("Author = %q", enriched.Author)
	}
	if enriched.Source != "github:vercel-labs/agent-skills@HEAD" {
		t.Fatalf("Source = %q", enriched.Source)
	}
	if enriched.Path != "skills/nextjs" {
		t.Fatalf("Path = %q", enriched.Path)
	}
}

func assertTreeSkill(t *testing.T, entries []manifest.Entry, name, skillPath string) {
	t.Helper()
	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if entries[0].Name != name {
		t.Fatalf("Name = %q, want %q", entries[0].Name, name)
	}
	if entries[0].Path != skillPath {
		t.Fatalf("Path = %q, want %q", entries[0].Path, skillPath)
	}
}

func entriesByName(entries []manifest.Entry) map[string]manifest.Entry {
	byName := make(map[string]manifest.Entry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	return byName
}
