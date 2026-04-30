package sync

import (
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/reconcile"
	"github.com/Naoray/scribe/internal/state"
)

// Status describes how a skill compares against the team loadout.
type Status int

const (
	StatusMissing    Status = iota // in loadout, not installed locally
	StatusCurrent                  // installed, matches loadout exactly
	StatusOutdated                 // installed, but loadout specifies a different version
	StatusExtra                    // installed locally, not in the team loadout
	StatusModified                 // installed, locally modified, upstream unchanged
	StatusConflicted               // merge produced conflicts, needs resolution
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
	case StatusModified:
		return "modified"
	case StatusConflicted:
		return "conflicted"
	default:
		return "unknown"
	}
}

// SkillStatus is the result of comparing one skill against the loadout.
type SkillStatus struct {
	Name       string
	Status     Status
	Installed  *state.InstalledSkill // nil if not installed
	Entry      *manifest.Entry       // catalog entry, nil for StatusExtra
	LoadoutRef string                // the ref from the manifest (e.g. "v1.0.0", "main")
	Maintainer string
	IsPackage  bool
	LatestSHA  string // resolved SHA for branch-pinned skills; empty if unavailable
}

// DisplayVersion returns the best human-readable version for this skill.
// Prefers semver tags as-is, else a short SHA when the ref is a branch/HEAD,
// else the installed revision counter.
func (sk SkillStatus) DisplayVersion() string {
	if sk.LoadoutRef != "" {
		if isVersionTag(sk.LoadoutRef) {
			return sk.LoadoutRef
		}
		if sk.LatestSHA != "" {
			return ShortSHA(sk.LatestSHA)
		}
		if sk.LoadoutRef != "HEAD" {
			return sk.LoadoutRef
		}
	}
	if sk.Installed != nil {
		return sk.Installed.DisplayVersion()
	}
	return ""
}

// isVersionTag reports whether a ref looks like a semver tag (v1.2.3).
func isVersionTag(ref string) bool {
	return strings.HasPrefix(ref, "v") && strings.ContainsRune(ref, '.')
}

// ShortSHA truncates a commit SHA to 7 characters for display.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
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
	if sk.Installed != nil && len(sk.Installed.Tools) > 0 {
		return strings.Join(sk.Installed.Tools, ", ")
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
	case StatusModified:
		return StatusDisplay{"✎", "modified"}
	case StatusConflicted:
		return StatusDisplay{"⚡", "conflict"}
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
	Updated bool // true = update, false = fresh install
	Merged  bool // true if 3-way merge was used
}

// SkillSkippedMsg is sent when a skill is already current — no action needed.
type SkillSkippedMsg struct{ Name string }

// SkillSkippedByDenyListMsg is sent when a user removed a registry skill and
// sync is preserving that removal intent.
type SkillSkippedByDenyListMsg struct {
	Name     string
	Registry string
}

// SkillErrorMsg is sent when a skill fails to install. Sync continues.
type SkillErrorMsg struct {
	Name string
	Err  error
}

// BudgetWarningMsg is emitted when a post-change projected skill set enters
// an agent's warning band but remains below the hard refusal limit.
type BudgetWarningMsg struct {
	Agent   string
	Message string
}

// SkillAdoptionNeededMsg is sent when install refused to overwrite a real
// (non-Scribe) directory at a tool projection path. Run `scribe adopt <name>`
// to import the existing content, then re-sync.
type SkillAdoptionNeededMsg struct {
	Name string
	Tool string
	Path string
}

// MergeConflictMsg is sent when a 3-way merge produces conflict markers.
type MergeConflictMsg struct {
	Name string
}

// SyncCompleteMsg is sent when all skills have been processed.
type SyncCompleteMsg struct {
	Installed int
	Updated   int
	Skipped   int
	Failed    int
}

type ReconcileConflictMsg struct {
	Name     string
	Conflict state.ProjectionConflict
}

type ReconcileCompleteMsg struct {
	Summary reconcile.Summary
}

// LegacyFormatMsg is sent when a registry still uses scribe.toml (TOML format).
type LegacyFormatMsg struct{ Repo string }

// --- Package events ---

// PackageSkippedMsg is sent when a package is skipped (e.g. already approved).
type PackageSkippedMsg struct {
	Name   string
	Reason string
}

// PackageInstallPromptMsg is sent when a package requires user approval.
type PackageInstallPromptMsg struct {
	Name    string
	Command string
	Source  string
}

// PackageApprovedMsg is sent when a user approves a package install.
type PackageApprovedMsg struct{ Name string }

// PackageDeniedMsg is sent when a user denies a package install.
type PackageDeniedMsg struct{ Name string }

// PackageInstallingMsg is sent when a package install command begins.
type PackageInstallingMsg struct{ Name string }

// PackageInstalledMsg is sent when a package install completes successfully.
type PackageInstalledMsg struct{ Name string }

// PackageErrorMsg is sent when a package install or update fails.
type PackageErrorMsg struct {
	Name   string
	Err    error
	Stderr string
}

// PackageHashMismatchMsg is sent when a previously approved command has changed.
type PackageHashMismatchMsg struct {
	Name       string
	OldCommand string
	NewCommand string
	Source     string
}

// PackageUpdateMsg is sent when a package update command begins.
type PackageUpdateMsg struct{ Name string }

// PackageUpdatedMsg is sent when a package update completes successfully.
type PackageUpdatedMsg struct{ Name string }

// PackageDetectedMsg is emitted when a fetched payload was classified as a
// tree-package (nested SKILL.md or install script). Lets the UI show a
// "stored as package" hint before the install command runs.
type PackageDetectedMsg struct {
	Name   string
	Dir    string
	Source string // where the install command came from (scribe.yaml, ./setup, …)
}

// PackageOutputMsg streams a package install/uninstall command's combined
// output to the UI. Emitted once per command, after it finishes.
type PackageOutputMsg struct {
	Name   string
	Stdout string
	Stderr string
}

// PackageReclassifiedMsg is emitted by the migration pass when a legacy
// skills/ install is moved into packages/ because its tree shape identifies
// it as a package. InstallHint carries a note about whether setup should
// be re-run (we don't auto-run during migration).
type PackageReclassifiedMsg struct {
	Name        string
	OldPath     string
	NewPath     string
	InstallHint string
}
