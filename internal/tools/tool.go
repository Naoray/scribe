// Package tools manages AI agent install targets (e.g., Claude, Cursor).
// Each Tool implementation links skills from the canonical store into
// the agent's expected directory structure.
package tools

import "os"

const bootstrapSkillName = "scribe-agent"

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
	// (~/.scribe/skills/<name>). projectRoot scopes tools that support
	// project-local projections; an empty projectRoot preserves legacy global
	// projection behavior. Returns the paths of the links created.
	Install(skillName, canonicalDir, projectRoot string) (paths []string, err error)
	// Uninstall removes the links for a skill.
	Uninstall(skillName string) error
	// Detect reports whether this tool is installed on the machine.
	Detect() bool
	// SkillPath returns the absolute path where this tool expects the skill
	// symlink (or link file) to live. Used by adoption to remove the old
	// real directory before Install replaces it with a symlink.
	SkillPath(skillName string) (string, error)
	// CanonicalTarget returns the path inside canonicalDir that this tool's
	// on-disk projection mirrors (e.g. claude → canonicalDir/SKILL.md; codex
	// → canonicalDir itself). When ok is false, the tool manages its skills
	// opaquely (e.g. via a CLI) and reconcile skips drift inspection.
	CanonicalTarget(canonicalDir string) (path string, ok bool)
}

// DefaultTools returns the standard set of supported AI tools.
func DefaultTools() []Tool {
	return []Tool{
		ClaudeTool{},
		CursorTool{},
		GeminiTool{},
		CodexTool{},
	}
}

// DetectTools returns tools that are actually installed on this machine.
func DetectTools() []Tool {
	var detected []Tool
	for _, t := range DefaultTools() {
		if t.Detect() {
			detected = append(detected, t)
		}
	}
	return detected
}

func homeDirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func projectionProjectRoot(skillName, projectRoot string) string {
	if skillName == bootstrapSkillName {
		return ""
	}
	return projectRoot
}
