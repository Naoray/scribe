package sync

import (
	"golang.org/x/mod/semver"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

// compareEntry determines the status of one catalog entry by comparing
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
//
// Packages always use SHA comparison (they track a branch).
func compareEntry(entry manifest.Entry, installed *state.InstalledSkill, latestSHA string) Status {
	if installed == nil {
		return StatusMissing
	}

	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return StatusMissing
	}

	// Packages always use SHA comparison.
	// If latestSHA is empty (API unreachable), assume current to avoid spurious re-installs.
	if entry.IsPackage() {
		if latestSHA == "" {
			return StatusCurrent
		}
		if installed.CommitSHA == latestSHA {
			return StatusCurrent
		}
		return StatusOutdated
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
