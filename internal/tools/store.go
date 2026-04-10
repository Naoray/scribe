package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/paths"
)

// SlugifyRegistry converts "owner/repo" to "owner-repo" for filesystem paths.
func SlugifyRegistry(repo string) string {
	return strings.ReplaceAll(repo, "/", "-")
}

// reservedNames are skill names that conflict with internal paths or OS artifacts.
var reservedNames = map[string]bool{
	"versions":  true,
	".git":      true,
	".DS_Store": true,
}

// WriteToStore writes all skill files to ~/.scribe/skills/<skillName>/.
// Returns the canonical directory path.
// Called once per skill before any target links are created.
// After writing, if SKILL.md is among the files, a .scribe-base.md copy is
// created alongside it to serve as the merge base for 3-way merge on updates.
func WriteToStore(skillName string, files []SkillFile) (string, error) {
	// Block reserved names.
	if reservedNames[skillName] {
		return "", fmt.Errorf("invalid skill name %q: reserved name", skillName)
	}

	// Validate skillName doesn't escape the store directory.
	if strings.Contains(skillName, "..") || filepath.IsAbs(skillName) {
		return "", fmt.Errorf("invalid skill name %q: contains path traversal", skillName)
	}

	base, err := StoreDir()
	if err != nil {
		return "", err
	}

	skillDir := filepath.Join(base, skillName)

	// Clean slate on update — remove existing before writing.
	if err := os.RemoveAll(skillDir); err != nil {
		return "", fmt.Errorf("clear store for %s: %w", skillName, err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("create store dir: %w", err)
	}

	var skillMDContent []byte

	for _, f := range files {
		// Validate file path doesn't escape the skill directory.
		dest := filepath.Join(skillDir, f.Path)
		if !strings.HasPrefix(filepath.Clean(dest), filepath.Clean(skillDir)+string(filepath.Separator)) && filepath.Clean(dest) != filepath.Clean(skillDir) {
			return "", fmt.Errorf("invalid file path %q: escapes skill directory", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", f.Path, err)
		}
		if f.Path == "SKILL.md" {
			skillMDContent = f.Content
		}
	}

	// Write .scribe-base.md as a copy of SKILL.md for 3-way merge.
	if skillMDContent != nil {
		basePath := filepath.Join(skillDir, ".scribe-base.md")
		if err := os.WriteFile(basePath, skillMDContent, 0o644); err != nil {
			return "", fmt.Errorf("write .scribe-base.md: %w", err)
		}
	}

	return skillDir, nil
}

// StoreDir returns the ~/.scribe/skills/ directory path.
func StoreDir() (string, error) {
	return paths.StoreDir()
}
