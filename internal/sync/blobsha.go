package sync

import (
	"path"
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
	blobs := resolveSkillBlobSHAs(tree, entry)
	sha, found := blobs[skillBlobTarget(entry)]
	return sha, found
}

func resolveSkillBlobSHAs(tree []provider.TreeEntry, entry manifest.Entry) map[string]string {
	base := skillTreeBase(entry)
	blobs := map[string]string{}
	for _, e := range tree {
		if e.Type != "blob" || !isUnderSkillBase(e.Path, base) {
			continue
		}
		blobs[e.Path] = e.SHA
	}
	return blobs
}

func skillBlobTarget(entry manifest.Entry) string {
	base := skillTreeBase(entry)
	if base == "" || base == "." {
		return "SKILL.md"
	}
	return path.Join(base, "SKILL.md")
}

func skillTreeBase(entry manifest.Entry) string {
	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	skillPath = strings.TrimSuffix(skillPath, "/")
	if skillPath == "SKILL.md" {
		return "."
	}
	return skillPath
}

func isUnderSkillBase(filePath, base string) bool {
	if base == "" || base == "." {
		return true
	}
	return filePath == base || strings.HasPrefix(filePath, base+"/")
}
