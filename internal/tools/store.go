package tools

import (
	"errors"
	"fmt"
	"io/fs"
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

	// Clean slate on update, but preserve version snapshots.
	// SnapshotVersion writes to versions/ before WriteToStore runs during sync,
	// so a blanket RemoveAll would destroy the snapshot the syncer just created.
	preserved := map[string]bool{"versions": true}
	entries, err := os.ReadDir(skillDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read store for %s: %w", skillName, err)
	}
	for _, entry := range entries {
		if preserved[entry.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(skillDir, entry.Name())); err != nil {
			return "", fmt.Errorf("clear %s: %w", entry.Name(), err)
		}
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

// WriteCanonicalSkill rewrites the canonical SKILL.md content and refreshes the
// stored merge base to match. Used by repair flows that promote a tool-local
// single-file projection back into the canonical store.
func WriteCanonicalSkill(canonicalDir string, skillMD []byte) error {
	if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	skillPath := filepath.Join(canonicalDir, "SKILL.md")
	if err := os.WriteFile(skillPath, skillMD, 0o644); err != nil {
		return fmt.Errorf("write canonical skill: %w", err)
	}
	basePath := filepath.Join(canonicalDir, ".scribe-base.md")
	if err := os.WriteFile(basePath, skillMD, 0o644); err != nil {
		return fmt.Errorf("write canonical base: %w", err)
	}
	return nil
}
