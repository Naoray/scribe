package tools

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Naoray/scribe/internal/skillmd"
)

const toolCodex = "codex"

// CodexTool exposes Scribe-managed skills to Codex via ~/.agents/skills,
// which is the directory Codex's native skill discovery reads from.
// (Codex still keeps config/state under ~/.codex, but skill projections
// must land in ~/.agents/skills/<name> to be visible to Codex sessions.)
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

func (t CodexTool) Install(skillName, canonicalDir, projectRoot string) ([]string, error) {
	projectRoot = projectionProjectRoot(skillName, projectRoot)
	if err := ensureCodexCompatibleSkillMD(skillName, canonicalDir); err != nil {
		return nil, err
	}
	skillsDir, err := codexSkillsDir(projectRoot)
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
	// Remove stale symlinks left at the legacy .codex/skills/ path by older scribe versions.
	cleanupLegacyCodexLink(skillName, projectRoot)
	return []string{link}, nil
}

func (t CodexTool) Uninstall(skillName string) error {
	skillsDir, err := codexSkillsDir("")
	if err != nil {
		return err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove codex/%s: %w", skillName, err)
	}
	// Backwards-compat: also clean up stale projections at the legacy
	// ~/.codex/skills/<name> path that prior scribe versions created.
	if home, herr := os.UserHomeDir(); herr == nil {
		legacyLink := filepath.Join(home, ".codex", "skills", skillName)
		if err := os.Remove(legacyLink); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove legacy codex/%s: %w", skillName, err)
		}
	}
	return nil
}

func (t CodexTool) SkillPath(skillName, projectRoot string) (string, error) {
	projectRoot = projectionProjectRoot(skillName, projectRoot)
	skillsDir, err := codexSkillsDir(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(skillsDir, skillName), nil
}

func (t CodexTool) CanonicalTarget(canonicalDir string) (string, bool) {
	return canonicalDir, true
}

func codexSkillsDir(projectRoot string) (string, error) {
	if projectRoot != "" {
		return filepath.Join(projectRoot, ".agents", "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

// cleanupLegacyCodexLink removes symlinks that older scribe versions created under
// .codex/skills/ instead of .agents/skills/. Called after every successful Install
// so that the next sync transparently migrates existing installations.
func cleanupLegacyCodexLink(skillName, projectRoot string) {
	if projectRoot != "" {
		_ = os.Remove(filepath.Join(projectRoot, ".codex", "skills", skillName))
		return
	}
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, ".codex", "skills", skillName))
	}
}

func ensureCodexCompatibleSkillMD(skillName, canonicalDir string) error {
	skillPath := filepath.Join(canonicalDir, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read codex skill %q: %w", skillName, err)
	}
	_, normalized, err := skillmd.Normalize(skillName, content)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, normalized) {
		return WriteCanonicalSkill(canonicalDir, normalized)
	}
	return nil
}
