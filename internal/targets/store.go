package targets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Naoray/scribe/internal/paths"
)

// WriteToStore writes all skill files to ~/.scribe/skills/<name>/.
// Returns the canonical directory path.
// Called once per skill before any target links are created.
func WriteToStore(skillName string, files []SkillFile) (string, error) {
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

	for _, f := range files {
		dest := filepath.Join(skillDir, f.Path)
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
