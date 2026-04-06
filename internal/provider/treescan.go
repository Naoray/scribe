package provider

import (
	"fmt"
	"path"

	"github.com/Naoray/scribe/internal/manifest"
)

const skillFileName = "SKILL.md"

// ScanTreeForSkills finds all SKILL.md files in a tree listing and returns
// catalog entries for each. The skill name is derived from the parent directory.
// A root-level SKILL.md uses the repo name.
func ScanTreeForSkills(tree []TreeEntry, owner, repo string) []manifest.Entry {
	source := fmt.Sprintf("github:%s/%s@HEAD", owner, repo)

	var entries []manifest.Entry
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
			// Root-level SKILL.md — use repo name.
			name = repo
			skillPath = "."
		} else {
			// Name is the immediate parent dir.
			name = path.Base(dir)
			skillPath = dir
		}

		entries = append(entries, manifest.Entry{
			Name:   name,
			Source: source,
			Path:   skillPath,
			Author: owner,
		})
	}

	return entries
}
