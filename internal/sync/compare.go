package sync

import (
	"golang.org/x/mod/semver"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

// compareEntry determines status by comparing the catalog entry against what's installed.
// The registryRepo identifies which source entry to compare against.
//
// Tag refs (e.g. "v1.0.0", "v0.12.9.0"):
//   - local version >= loadout version → StatusCurrent  (ahead is fine)
//   - local version <  loadout version → StatusOutdated (team has moved on)
//   - non-semver tags: exact string match (no ordering known)
//
// Branch refs (e.g. "main"):
//   - latestSHA == source.LastSHA → StatusCurrent
//   - any mismatch               → StatusOutdated
//
// Packages always use SHA comparison (they track a branch).
func compareEntry(entry manifest.Entry, installed *state.InstalledSkill, latestSHA, registryRepo string) Status {
	if installed == nil {
		return StatusMissing
	}

	// Find the source entry for this registry.
	var source *state.SkillSource
	for i := range installed.Sources {
		if installed.Sources[i].Registry == registryRepo {
			source = &installed.Sources[i]
			break
		}
	}

	if source == nil {
		// Installed but not from this registry — treat as missing from this registry's perspective.
		return StatusMissing
	}

	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return StatusMissing
	}

	// Packages and branches use SHA comparison.
	// If latestSHA is empty (API unreachable), assume current to avoid spurious re-installs.
	if entry.IsPackage() || src.IsBranch() {
		if latestSHA == missingSkillBlobSHA {
			return StatusOutdated
		}
		if latestSHA == "" {
			return StatusCurrent
		}
		if source.LastSHA == latestSHA {
			return StatusCurrent
		}
		return StatusOutdated
	}

	// Tag ref: try semver comparison first.
	if semver.IsValid(src.Ref) && semver.IsValid(source.Ref) {
		if semver.Compare(source.Ref, src.Ref) >= 0 {
			return StatusCurrent // local is same or newer
		}
		return StatusOutdated
	}

	// Non-semver tag (e.g. "v0.12.9.0"): exact match only.
	if source.Ref == src.Ref {
		return StatusCurrent
	}
	return StatusOutdated
}
