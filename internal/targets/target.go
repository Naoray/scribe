package targets

// SkillFile represents a file to be written to the skill store.
type SkillFile struct {
	Path    string // relative to the skill root (e.g. "scripts/deploy.sh")
	Content []byte
}

// Target links a skill from the canonical store into a specific AI tool's directory.
type Target interface {
	// Name returns the tool identifier (e.g. "claude", "cursor").
	Name() string
	// Install creates a link from the agent's expected directory into canonicalDir
	// (~/.scribe/skills/<name>). Returns the paths of the links created.
	Install(skillName, canonicalDir string) (paths []string, err error)
}
