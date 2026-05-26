package tools

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

const toolGemini = "gemini"

// GeminiTool exposes Scribe-managed skills to Gemini via .agents/skills,
// which Gemini's native skill discovery reads in addition to .gemini/skills.
type GeminiTool struct{}

func (t GeminiTool) Name() string { return toolGemini }

func (t GeminiTool) Detect() bool {
	_, err := exec.LookPath(toolGemini)
	return err == nil
}

func (t GeminiTool) Install(skillName, canonicalDir, projectRoot string) ([]string, error) {
	projectRoot = projectionProjectRoot(skillName, projectRoot)
	skillsDir, err := codexSkillsDir(projectRoot)
	if err != nil {
		return nil, err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, err
	}
	if err := replaceSymlink(link, canonicalDir); err != nil {
		return nil, err
	}
	return []string{link}, nil
}

func (t GeminiTool) SkillPath(skillName, projectRoot string) (string, error) {
	projectRoot = projectionProjectRoot(skillName, projectRoot)
	skillsDir, err := codexSkillsDir(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(skillsDir, skillName), nil
}

func (t GeminiTool) CanonicalTarget(canonicalDir string) (string, bool) {
	return canonicalDir, true
}

func (t GeminiTool) Uninstall(skillName string) error {
	skillsDir, err := codexSkillsDir("")
	if err != nil {
		return err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
