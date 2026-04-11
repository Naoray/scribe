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
// Packages have global identity by name (one shell install per machine), so
// source registry is informational. Compare the package against any recorded
// source SHA and preserve StatusModified when the package is otherwise current.
func compareEntry(entry manifest.Entry, installed *state.InstalledSkill, latestSHA, registryRepo string, locallyModified bool) Status {
	if installed == nil {
		return StatusMissing
	}

	if entry.IsPackage() {
		if latestSHA == "" {
			return StatusCurrent
		}
		knownSHA := false
		for _, src := range installed.Sources {
			if src.LastSHA == "" {
				continue
			}
			knownSHA = true
			if src.LastSHA == latestSHA {
				return StatusCurrent
			}
		}
		if !knownSHA {
			return StatusCurrent
		}
		return StatusOutdated
	}

	source := findSourceForRegistry(installed, registryRepo)
	if source == nil {
		// Installed but not from this registry — treat as missing from this registry's perspective.
		return StatusMissing
	}

	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return StatusMissing
	}

	var status Status
	switch {
	case src.IsBranch():
		status = compareBranchOrPackage(source, latestSHA)
	default:
		status = compareTag(source, src.Ref)
	}

	return applyLocalModificationOverlay(status, locallyModified)
}

func findSourceForRegistry(installed *state.InstalledSkill, registryRepo string) *state.SkillSource {
	for i := range installed.Sources {
		if installed.Sources[i].Registry == registryRepo {
			return &installed.Sources[i]
		}
	}
	return nil
}

func compareBranchOrPackage(source *state.SkillSource, latestSHA string) Status {
	// If the latest blob SHA is unavailable because the skill path no longer
	// exists in the registry, it should show as outdated.
	if latestSHA == missingSkillBlobSHA {
		return StatusOutdated
	}
	// If the API is unavailable, assume current to avoid spurious updates.
	if latestSHA == "" {
		return StatusCurrent
	}
	if source.LastSHA == latestSHA {
		return StatusCurrent
	}
	return StatusOutdated
}

func compareTag(source *state.SkillSource, desiredRef string) Status {
	// Semver tags: local ahead is acceptable.
	if semver.IsValid(desiredRef) && semver.IsValid(source.Ref) {
		if semver.Compare(source.Ref, desiredRef) >= 0 {
			return StatusCurrent
		}
		return StatusOutdated
	}
	// Non-semver tags: exact match only.
	if source.Ref == desiredRef {
		return StatusCurrent
	}
	return StatusOutdated
}

func applyLocalModificationOverlay(status Status, locallyModified bool) Status {
	if locallyModified && status == StatusCurrent {
		return StatusModified
	}
	return status
}
