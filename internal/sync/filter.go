package sync

import (
	"path/filepath"
	"strings"
)

// denyPrefixes are path prefixes that should never be synced into the skill store.
var denyPrefixes = []string{
	".git/",
}

// denyExact are exact filenames (case-insensitive) denied at the root of a skill.
var denyExact = []string{
	".gitignore",
	".gitkeep",
	"license",
	"license.md",
	"license.txt",
	"license.mit",
	"license.apache",
}

// shouldInclude reports whether a file should be synced into the skill store.
// Filters out repo infrastructure files that leak when skill path == repo root.
func shouldInclude(path string) bool {
	for _, prefix := range denyPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}

	// Only deny exact matches at the root level (not nested).
	if filepath.Dir(path) == "." {
		base := strings.ToLower(filepath.Base(path))
		for _, denied := range denyExact {
			if base == denied {
				return false
			}
		}
	}

	return true
}
