package targets

import (
	"fmt"
	"os"
	"path/filepath"

	gh "github.com/Naoray/scribe/internal/github"
)

// WriteToStore writes all skill files to ~/.scribe/skills/<name>/ and
// generates a .cursor.mdc file there. Returns the canonical directory path.
// Called once per skill before any target links are created.
func WriteToStore(skillName string, files []gh.SkillFile) (string, error) {
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

	var skillMD []byte
	for _, f := range files {
		dest := filepath.Join(skillDir, f.Path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", f.Path, err)
		}
		if isSkillMD(f.Path) {
			skillMD = f.Content
		}
	}

	// Generate .cursor.mdc into the store so Cursor target can symlink it.
	if skillMD != nil {
		mdc := generateMDC(skillMD)
		if err := os.WriteFile(filepath.Join(skillDir, ".cursor.mdc"), mdc, 0o644); err != nil {
			return "", fmt.Errorf("write .cursor.mdc: %w", err)
		}
	}

	return skillDir, nil
}

// StoreDir returns the ~/.scribe/skills/ directory path.
func StoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".scribe", "skills"), nil
}
