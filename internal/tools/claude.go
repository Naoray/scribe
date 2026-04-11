package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// toolClaude is the identifier for the Claude Code tool.
const toolClaude = "claude"

// ClaudeTool symlinks ~/.claude/skills/<name> → ~/.scribe/skills/<name>/SKILL.md.
// Only the SKILL.md file is linked, so Claude Code does not see internal files
// like .scribe-base.md or versions/.
type ClaudeTool struct{}

func (t ClaudeTool) Name() string { return toolClaude }

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
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create claude skills dir: %w", err)
	}
	if err := replaceSymlink(link, filepath.Join(canonicalDir, "SKILL.md")); err != nil {
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
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
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
