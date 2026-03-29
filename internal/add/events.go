package add

// --- Events (tea.Msg-compatible) emitted during add ---

// SkillDiscoveredMsg is sent once per candidate skill during discovery.
type SkillDiscoveredMsg struct {
	Name      string
	Origin    string // "local" or "registry:owner/repo"
	Source    string // "github:owner/repo@ref" or empty
	LocalPath string
}

// RegistrySelectedMsg is sent when the target registry is chosen.
type RegistrySelectedMsg struct {
	Registry string
}

// SkillAddingMsg is sent when a skill push to GitHub begins.
type SkillAddingMsg struct {
	Name   string
	Upload bool
}

// SkillAddedMsg is sent when a skill is successfully added to the registry.
type SkillAddedMsg struct {
	Name     string
	Registry string
	Source   string
	Upload   bool
}

// SkillAddErrorMsg is sent when adding a skill fails.
type SkillAddErrorMsg struct {
	Name string
	Err  error
}

// AddCompleteMsg is sent when all skills have been processed.
type AddCompleteMsg struct {
	Added       int
	Failed      int
	SyncStarted bool
}
