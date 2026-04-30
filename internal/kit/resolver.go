package kit

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/projectfile"
)

// Resolve returns the final skill set for a project.
// Algorithm:
//   - union all skills declared by projectFile.Kits
//   - apply projectFile.Add
//   - apply projectFile.Remove
//   - expand globs against installedSkills
//   - return a deduplicated, sorted skill name slice
func Resolve(pf *projectfile.ProjectFile, availableKits map[string]*Kit, installedSkills []string) ([]string, error) {
	if pf == nil {
		pf = &projectfile.ProjectFile{}
	}

	patterns := make([]string, 0, len(pf.Add))
	for _, kitName := range pf.Kits {
		kit, ok := availableKits[kitName]
		if !ok {
			return nil, fmt.Errorf("kit %q not found", kitName)
		}
		patterns = append(patterns, kit.Skills...)
	}
	patterns = append(patterns, pf.Add...)

	result := make(map[string]struct{})
	for _, pattern := range patterns {
		matches, err := expandSkillPattern(pattern, installedSkills)
		if err != nil {
			return nil, err
		}
		for _, skill := range matches {
			result[skill] = struct{}{}
		}
	}

	for _, skill := range pf.Remove {
		delete(result, skill)
	}

	skills := make([]string, 0, len(result))
	for skill := range result {
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	return skills, nil
}

func expandSkillPattern(pattern string, installedSkills []string) ([]string, error) {
	if !hasGlobMeta(pattern) {
		return []string{pattern}, nil
	}

	matches := make([]string, 0)
	for _, skill := range installedSkills {
		match, err := filepath.Match(pattern, skill)
		if err != nil {
			return nil, fmt.Errorf("match skill pattern %q: %w", pattern, err)
		}
		if match {
			matches = append(matches, skill)
		}
	}
	return matches, nil
}

func hasGlobMeta(pattern string) bool {
	for _, r := range pattern {
		switch r {
		case '*', '?', '[':
			return true
		}
	}
	return false
}
