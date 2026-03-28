package sync

import (
	"golang.org/x/mod/semver"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

// compareSkill determines the status of one skill by comparing
// the team loadout entry against what's recorded in state.json.
//
// Tag refs (e.g. "v1.0.0", "v0.12.9.0"):
//   - local version >= loadout version → StatusCurrent  (ahead is fine)
//   - local version <  loadout version → StatusOutdated (team has moved on)
//   - non-semver tags: exact string match (no ordering known)
//
// Branch refs (e.g. "main"):
//   - latestSHA == installed.CommitSHA → StatusCurrent
//   - any mismatch                     → StatusOutdated
func compareSkill(skill manifest.Skill, installed *state.InstalledSkill, latestSHA string) Status {
	if installed == nil {
		return StatusMissing
	}

	src, err := manifest.ParseSource(skill.Source)
	if err != nil {
		return StatusMissing
	}

	if src.IsBranch() {
		if latestSHA != "" && installed.CommitSHA == latestSHA {
			return StatusCurrent
		}
		return StatusOutdated
	}

	// Tag ref: try semver comparison first.
	if semver.IsValid(src.Ref) && semver.IsValid(installed.Version) {
		if semver.Compare(installed.Version, src.Ref) >= 0 {
			return StatusCurrent // local is same or newer
		}
		return StatusOutdated
	}

	// Non-semver tag (e.g. "v0.12.9.0"): exact match only.
	if installed.Version == src.Ref {
		return StatusCurrent
	}
	return StatusOutdated
}
