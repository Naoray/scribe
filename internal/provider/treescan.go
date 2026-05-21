package provider

import (
	"fmt"
	"path"
	"regexp"
	"sort"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
)

const skillFileName = "SKILL.md"

var treeScanSkillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ScanTreeForSkills finds SKILL.md files in a tree listing and returns one
// catalog entry per skill name. The skill name is derived from the parent
// directory. A root-level SKILL.md uses the repo name.
func ScanTreeForSkills(tree []TreeEntry, owner, repo string) []manifest.Entry {
	return scanTreeForSkillsWithSource(tree, owner, repo, fmt.Sprintf("github:%s/%s@HEAD", owner, repo))
}

func scanTreeForSkillsWithSource(tree []TreeEntry, owner, repo, source string) []manifest.Entry {
	candidatesByName := make(map[string][]string)
	for _, entry := range tree {
		if entry.Type != "blob" {
			continue
		}
		if path.Base(entry.Path) != skillFileName {
			continue
		}

		// Derive skill name from parent directory.
		dir := path.Dir(entry.Path)
		var name, skillPath string
		if dir == "." {
			// Root-level SKILL.md — use repo name and record the file path.
			name = repo
			skillPath = skillFileName
		} else {
			// Name is the immediate parent dir.
			name = path.Base(dir)
			skillPath = dir
		}

		candidatesByName[name] = append(candidatesByName[name], skillPath)
	}

	names := make([]string, 0, len(candidatesByName))
	for name := range candidatesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]manifest.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, manifest.Entry{
			Name:   name,
			Source: source,
			Path:   selectCanonicalSkillPath(candidatesByName[name]),
			Author: owner,
		})
	}

	return entries
}

func selectCanonicalSkillPath(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	paths := append([]string(nil), candidates...)
	sort.Strings(paths)

	for _, candidate := range paths {
		if path.Dir(candidate) == ".agents/skills" {
			return candidate
		}
	}
	for _, candidate := range paths {
		if path.Dir(candidate) == "." {
			return candidate
		}
	}
	for _, candidate := range paths {
		if path.Dir(candidate) == "skills" {
			return candidate
		}
	}
	return paths[0]
}

// EnrichTreeSkillEntry applies SKILL.md frontmatter metadata to a tree-scan
// entry while preserving source and path fields used for install and lock pins.
func EnrichTreeSkillEntry(entry manifest.Entry, content []byte) (manifest.Entry, error) {
	meta, err := discovery.ParseSkillMetadata(content)
	if err != nil {
		return entry, err
	}
	if meta.Name != "" {
		if !treeScanSkillName.MatchString(meta.Name) || path.Base(meta.Name) != meta.Name {
			return entry, fmt.Errorf("invalid frontmatter name %q", meta.Name)
		}
		entry.Name = meta.Name
	}
	if meta.Description != "" {
		entry.Description = meta.Description
	}
	if meta.Author != "" {
		entry.Author = meta.Author
	}
	if meta.Source.Author != "" {
		entry.Author = meta.Source.Author
	}
	return entry, nil
}
