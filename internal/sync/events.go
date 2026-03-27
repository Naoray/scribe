package sync

import "github.com/Naoray/scribe/internal/state"

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
	Installed  *state.InstalledSkill // nil if not installed
	LoadoutRef string                // the ref from scribe.toml (e.g. "v1.0.0", "main")
	Maintainer string
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
