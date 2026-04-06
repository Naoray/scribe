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

// WriteToStore writes all skill files to ~/.scribe/skills/<registrySlug>/<name>/.
// Returns the canonical directory path.
// Called once per skill before any target links are created.
func WriteToStore(registrySlug, skillName string, files []SkillFile) (string, error) {
	// Validate inputs don't escape the store directory.
	if strings.Contains(registrySlug, "..") {
		return "", fmt.Errorf("invalid registry slug %q: contains path traversal", registrySlug)
	}
	if strings.Contains(skillName, "..") {
		return "", fmt.Errorf("invalid skill name %q: contains path traversal", skillName)
	}

	base, err := StoreDir()
	if err != nil {
		return "", err
	}

	skillDir := filepath.Join(base, registrySlug, skillName)

	// Clean slate on update — remove existing before writing.
	if err := os.RemoveAll(skillDir); err != nil {
		return "", fmt.Errorf("clear store for %s/%s: %w", registrySlug, skillName, err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("create store dir: %w", err)
	}

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
	}

	return skillDir, nil
}

// StoreDir returns the ~/.scribe/skills/ directory path.
func StoreDir() (string, error) {
	return paths.StoreDir()
}
