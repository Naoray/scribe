package targets

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeTarget symlinks ~/.claude/skills/<name> → ~/.scribe/skills/<name>.
// The whole skill directory is linked, so Claude Code sees all files
// (SKILL.md, scripts/, references/, etc.) without duplication.
type ClaudeTarget struct{}

func (t ClaudeTarget) Name() string { return "claude" }

func (t ClaudeTarget) Install(skillName, canonicalDir string) ([]string, error) {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create claude skills dir: %w", err)
	}

	link := filepath.Join(skillsDir, skillName)
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, fmt.Errorf("symlink claude/%s: %w", skillName, err)
	}
	return []string{link}, nil
}

func claudeSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}
