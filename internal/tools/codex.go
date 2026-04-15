package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const toolCodex = "codex"

// CodexTool exposes Scribe-managed skills to Codex via ~/.codex/skills.
type CodexTool struct{}

func (t CodexTool) Name() string { return toolCodex }

func (t CodexTool) Detect() bool {
	if _, err := exec.LookPath(toolCodex); err == nil {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return homeDirExists(filepath.Join(home, ".codex"))
}

func (t CodexTool) Install(skillName, canonicalDir string) ([]string, error) {
	if err := ensureCodexCompatibleSkillMD(skillName, canonicalDir); err != nil {
		return nil, err
	}
	skillsDir, err := codexSkillsDir()
	if err != nil {
		return nil, err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create codex skills dir: %w", err)
	}
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, fmt.Errorf("symlink codex/%s: %w", skillName, err)
	}
	return []string{link}, nil
}

func (t CodexTool) Uninstall(skillName string) error {
	skillsDir, err := codexSkillsDir()
	if err != nil {
		return err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove codex/%s: %w", skillName, err)
	}
	return nil
}

func (t CodexTool) SkillPath(skillName string) (string, error) {
	skillsDir, err := codexSkillsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(skillsDir, skillName), nil
}

func (t CodexTool) CanonicalTarget(canonicalDir string) (string, bool) {
	return canonicalDir, true
}

func codexSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".codex", "skills"), nil
}

func ensureCodexCompatibleSkillMD(skillName, canonicalDir string) error {
	skillPath := filepath.Join(canonicalDir, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read codex skill %q: %w", skillName, err)
	}
	if strings.HasPrefix(string(content), "---\n") {
		return nil
	}

	description := firstBodyParagraph(content)
	if description == "" {
		description = skillName
	}
	normalized := []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s", skillName, description, strings.TrimLeft(string(content), "\n")))
	if err := WriteCanonicalSkill(canonicalDir, normalized); err != nil {
		return fmt.Errorf("normalize codex skill %q: %w", skillName, err)
	}
	return nil
}

func firstBodyParagraph(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}
