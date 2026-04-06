package add

// --- Events (tea.Msg-compatible) emitted during add ---

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

// SkillAddDeniedMsg is sent when author enforcement blocks a skill modification.
type SkillAddDeniedMsg struct {
	Name   string
	Author string
}

