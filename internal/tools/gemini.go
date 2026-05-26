package tools

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// Remove stale scribe-managed symlinks left at the legacy .gemini/skills/
	// path by older scribe versions. User-authored Gemini skills stay untouched.
	_ = cleanupLegacyGeminiLink(skillName, projectRoot)
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
	if err := cleanupLegacyGeminiLink(skillName, ""); err != nil {
		return err
	}
	return nil
}

func cleanupLegacyGeminiLink(skillName, projectRoot string) error {
	storeRoot, err := StoreDir()
	if err != nil {
		return err
	}
	resolvedStoreRoot, err := filepath.EvalSymlinks(storeRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, link := range legacyGeminiSkillLinks(skillName, projectRoot) {
		if err := removeLegacyGeminiLinkIfScribeManaged(link, resolvedStoreRoot); err != nil {
			return err
		}
	}
	return nil
}

func legacyGeminiSkillLinks(skillName, projectRoot string) []string {
	var links []string
	if home, err := os.UserHomeDir(); err == nil {
		links = append(links, filepath.Join(home, ".gemini", "skills", skillName))
	}
	if projectRoot != "" {
		links = append(links, filepath.Join(projectRoot, ".gemini", "skills", skillName))
	}
	return links
}

func removeLegacyGeminiLinkIfScribeManaged(link, resolvedStoreRoot string) error {
	info, err := os.Lstat(link)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	target, err := os.Readlink(link)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !pathWithin(resolvedTarget, resolvedStoreRoot) {
		return nil
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
