package sync

import (
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
)

// missingSkillBlobSHA marks a successful tree lookup where the referenced
// SKILL.md does not exist in the upstream repo.
const missingSkillBlobSHA = "__missing_skill_blob__"

// resolveSkillBlobSHA returns the git blob SHA of SKILL.md for the given
// catalog entry. This is the identity signal we compare against state:
// commit SHAs flip on any repo activity, blob SHAs flip only when the file
// content actually changes. The boolean reports whether the skill file was
// found in the tree; a missing file is distinct from an API failure.
func resolveSkillBlobSHA(tree []provider.TreeEntry, entry manifest.Entry) (string, bool) {
	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	skillPath = strings.TrimSuffix(skillPath, "/")
	target := "SKILL.md"
	if skillPath != "" && skillPath != "." {
		target = skillPath + "/SKILL.md"
	}
	for _, e := range tree {
		if e.Type == "blob" && e.Path == target {
			return e.SHA, true
		}
	}
	return "", false
}
