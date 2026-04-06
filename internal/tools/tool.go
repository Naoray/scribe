package tools

// SkillFile represents a file to be written to the skill store.
type SkillFile struct {
	Path    string // relative to the skill root (e.g. "scripts/deploy.sh")
	Content []byte
}

// Tool links a skill from the canonical store into a specific AI tool's directory.
type Tool interface {
	// Name returns the tool identifier (e.g. "claude", "cursor").
	Name() string
	// Install creates a link from the agent's expected directory into canonicalDir
	// (~/.scribe/skills/<name>). Returns the paths of the links created.
	Install(skillName, canonicalDir string) (paths []string, err error)
	// Uninstall removes the links for a skill.
	Uninstall(skillName string) error
	// Detect reports whether this tool is installed on the machine.
	Detect() bool
}

// DetectTools returns tools that are actually installed on this machine.
func DetectTools() []Tool {
	all := []Tool{ClaudeTool{}, CursorTool{}}
	var detected []Tool
	for _, t := range all {
		if t.Detect() {
			detected = append(detected, t)
		}
	}
	return detected
}
