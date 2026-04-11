package sync

import (
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
)

// resolveSkillBlobSHA returns the git blob SHA of SKILL.md for the given
// catalog entry. This is the identity signal we compare against state:
// commit SHAs flip on any repo activity, blob SHAs flip only when the file
// content actually changes. Returns "" if the skill file is not present in
// the tree (deleted, typo, or wrong path).
func resolveSkillBlobSHA(tree []provider.TreeEntry, entry manifest.Entry) string {
	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	target := strings.TrimSuffix(skillPath, "/") + "/SKILL.md"
	for _, e := range tree {
		if e.Type == "blob" && e.Path == target {
			return e.SHA
		}
	}
	return ""
}
