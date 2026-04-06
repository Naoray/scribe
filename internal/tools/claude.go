package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

// ToolClaude is the identifier for the Claude Code tool.
const ToolClaude = "claude"

// ClaudeTool symlinks ~/.claude/skills/<name> → ~/.scribe/skills/<name>.
// The whole skill directory is linked, so Claude Code sees all files
// (SKILL.md, scripts/, references/, etc.) without duplication.
type ClaudeTool struct{}

func (t ClaudeTool) Name() string { return ToolClaude }

func (t ClaudeTool) Detect() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude"))
	return err == nil
}

func (t ClaudeTool) Install(skillName, canonicalDir string) ([]string, error) {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}

	link := filepath.Join(skillsDir, skillName)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return nil, fmt.Errorf("create claude skills subdir: %w", err)
	}
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, fmt.Errorf("symlink claude/%s: %w", skillName, err)
	}
	return []string{link}, nil
}

func (t ClaudeTool) Uninstall(skillName string) error {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove claude/%s: %w", skillName, err)
	}
	// Clean up empty parent directories left after removing namespaced symlinks.
	parent := filepath.Dir(link)
	if parent != skillsDir {
		_ = os.Remove(parent) // ignore error if not empty
	}
	return nil
}

func claudeSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}
