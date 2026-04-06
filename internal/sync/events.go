package sync

import (
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

// Status describes how a skill compares against the team loadout.
type Status int

const (
	StatusMissing  Status = iota // in loadout, not installed locally
	StatusCurrent                // installed, matches loadout exactly
	StatusOutdated               // installed, but loadout specifies a different version
	StatusExtra                  // installed locally, not in the team loadout
)

func (s Status) String() string {
	switch s {
	case StatusMissing:
		return "missing"
	case StatusCurrent:
		return "current"
	case StatusOutdated:
		return "outdated"
	case StatusExtra:
		return "extra"
	default:
		return "unknown"
	}
}

// SkillStatus is the result of comparing one skill against the loadout.
type SkillStatus struct {
	Name       string
	Status     Status
	Installed  *state.InstalledSkill  // nil if not installed
	Entry      *manifest.Entry        // catalog entry, nil for StatusExtra
	LoadoutRef string                 // the ref from the manifest (e.g. "v1.0.0", "main")
	Maintainer string
	IsPackage  bool
}

// DisplayVersion returns the best human-readable version for this skill.
func (sk SkillStatus) DisplayVersion() string {
	if sk.LoadoutRef != "" {
		return sk.LoadoutRef
	}
	if sk.Installed != nil {
		return sk.Installed.DisplayVersion()
	}
	return ""
}

// DisplayAuthor returns the author or "—" if unknown.
func (sk SkillStatus) DisplayAuthor() string {
	if sk.Maintainer != "" {
		return sk.Maintainer
	}
	return "—"
}

// DisplayAgents returns the comma-separated list of installed targets.
func (sk SkillStatus) DisplayAgents() string {
	if sk.Installed != nil && len(sk.Installed.Targets) > 0 {
		return strings.Join(sk.Installed.Targets, ", ")
	}
	return ""
}

// StatusDisplay holds the icon and label for a status value.
type StatusDisplay struct {
	Icon  string
	Label string
}

// Display returns the icon and label for rendering this status.
func (s Status) Display() StatusDisplay {
	switch s {
	case StatusCurrent:
		return StatusDisplay{"✓", "current"}
	case StatusOutdated:
		return StatusDisplay{"↑", "update"}
	case StatusMissing:
		return StatusDisplay{"○", "missing"}
	case StatusExtra:
		return StatusDisplay{"?", "extra"}
	default:
		return StatusDisplay{"?", "unknown"}
	}
}

// --- Events (tea.Msg) emitted during sync ---

// SkillResolvedMsg is sent once per skill after the diff is computed,
// before any downloads start. Powers the initial list render.
type SkillResolvedMsg struct{ SkillStatus }

// SkillDownloadingMsg is sent when a skill download begins.
type SkillDownloadingMsg struct{ Name string }

// SkillInstalledMsg is sent when a skill is successfully installed or updated.
type SkillInstalledMsg struct {
	Name    string
	Version string
	Updated bool // true = update, false = fresh install
}

// SkillSkippedMsg is sent when a skill is already current — no action needed.
type SkillSkippedMsg struct{ Name string }

// SkillErrorMsg is sent when a skill fails to install. Sync continues.
type SkillErrorMsg struct {
	Name string
	Err  error
}

// SyncCompleteMsg is sent when all skills have been processed.
type SyncCompleteMsg struct {
	Installed int
	Updated   int
	Skipped   int
	Failed    int
}

// LegacyFormatMsg is sent when a registry still uses scribe.toml (TOML format).
type LegacyFormatMsg struct{ Repo string }

// --- Package events ---

// PackageInstallPromptMsg asks the user to approve a package's install command.
type PackageInstallPromptMsg struct {
	Name    string
	Command string
	Source  string
}

// PackageApprovedMsg is sent when the user approves a package install.
type PackageApprovedMsg struct{ Name string }

// PackageDeniedMsg is sent when the user denies a package install.
type PackageDeniedMsg struct{ Name string }

// PackageSkippedMsg is sent when a package is skipped (e.g. already approved).
type PackageSkippedMsg struct {
	Name   string
	Reason string
}

// PackageInstallingMsg is sent when a package install command begins.
type PackageInstallingMsg struct{ Name string }

// PackageInstalledMsg is sent when a package is successfully installed.
type PackageInstalledMsg struct{ Name string }

// PackageErrorMsg is sent when a package install fails.
type PackageErrorMsg struct {
	Name   string
	Err    error
	Stderr string
}
