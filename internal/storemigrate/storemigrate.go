package storemigrate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"github.com/Naoray/scribe/internal/state"
)

// migratedMarker is written at the store root once the v2 flat-store migration
// completes, so we don't rescan the filesystem on every command.
const migratedMarker = ".store-migrated-v2"

// AlreadyMigrated reports whether the v2 flat-store migration marker exists.
// Callers can use this to short-circuit before Load()ing state.
func AlreadyMigrated(storeDir string) bool {
	_, err := os.Stat(filepath.Join(storeDir, migratedMarker))
	return err == nil
}

// Migrate moves skills from ~/.scribe/skills/<slug>/<name>/ to ~/.scribe/skills/<name>/.
// It is idempotent — a marker file under storeDir gates repeat runs, and the
// underlying file operations are safe to retry if the marker is absent.
// Returns a list of warnings (e.g., conflicts quarantined).
func Migrate(storeDir string, st *state.State) (warnings []string, err error) {
	// Check filesystem marker — authoritative signal that migration ran.
	// state.SchemaVersion can't gate this: parseAndMigrate bumps it in memory
	// before file migration ever happens, so it's not a reliable signal.
	markerPath := filepath.Join(storeDir, migratedMarker)
	if _, statErr := os.Stat(markerPath); statErr == nil {
		return nil, nil
	}

	// movedSkills maps old SKILL.md path → new SKILL.md path for symlink updates.
	movedSkills := make(map[string]string)

	// Scan for two-level directories: <slug>/<name>/SKILL.md
	slugDirs, err := os.ReadDir(storeDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store dir: %w", err)
	}

	conflictsDir := filepath.Join(filepath.Dir(storeDir), "migration-conflicts")

	for _, slugEntry := range slugDirs {
		if !slugEntry.IsDir() {
			continue
		}
		slugName := slugEntry.Name()
		slugPath := filepath.Join(storeDir, slugName)

		skillDirs, err := os.ReadDir(slugPath)
		if err != nil {
			return nil, fmt.Errorf("read slug dir %s: %w", slugName, err)
		}

		for _, skillEntry := range skillDirs {
			if !skillEntry.IsDir() {
				continue
			}
			skillName := skillEntry.Name()
			oldDir := filepath.Join(slugPath, skillName)
			oldSkill := filepath.Join(oldDir, "SKILL.md")

			// Only process directories that actually contain a SKILL.md.
			if _, err := os.Stat(oldSkill); errors.Is(err, fs.ErrNotExist) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("stat %s: %w", oldSkill, err)
			}

			targetDir := filepath.Join(storeDir, skillName)
			targetSkill := filepath.Join(targetDir, "SKILL.md")

			if _, err := os.Stat(targetSkill); err == nil {
				// Target already exists — compare hashes.
				oldHash, hashErr := contentHash(oldSkill)
				if hashErr != nil {
					return nil, fmt.Errorf("hash %s: %w", oldSkill, hashErr)
				}
				newHash, hashErr := contentHash(targetSkill)
				if hashErr != nil {
					return nil, fmt.Errorf("hash %s: %w", targetSkill, hashErr)
				}

				if oldHash == newHash {
					// Identical — delete the slug copy.
					if err := os.RemoveAll(oldDir); err != nil {
						return nil, fmt.Errorf("remove identical slug copy %s: %w", oldDir, err)
					}
				} else {
					// Different — quarantine the slug copy.
					quarantineName := slugName + "-" + skillName
					quarantinePath := filepath.Join(conflictsDir, quarantineName)
					if err := os.MkdirAll(conflictsDir, 0o755); err != nil {
						return nil, fmt.Errorf("create conflicts dir: %w", err)
					}
					if err := os.Rename(oldDir, quarantinePath); err != nil {
						return nil, fmt.Errorf("quarantine %s: %w", oldDir, err)
					}
					warnings = append(warnings, fmt.Sprintf(
						"conflict: %s/%s differs from %s — quarantined to %s",
						slugName, skillName, skillName, quarantinePath,
					))
				}
			} else if errors.Is(err, fs.ErrNotExist) {
				// Target doesn't exist — move the directory.
				if err := os.Rename(oldDir, targetDir); err != nil {
					return nil, fmt.Errorf("rename %s → %s: %w", oldDir, targetDir, err)
				}
				movedSkills[oldSkill] = targetSkill
			} else {
				return nil, fmt.Errorf("stat target %s: %w", targetSkill, err)
			}
		}
	}

	// Create .scribe-base.md for each skill now in the flat store.
	flatDirs, err := os.ReadDir(storeDir)
	if err != nil {
		return nil, fmt.Errorf("read store dir for base copy: %w", err)
	}
	for _, entry := range flatDirs {
		if !entry.IsDir() {
			continue
		}
		skillMD := filepath.Join(storeDir, entry.Name(), "SKILL.md")
		baseMD := filepath.Join(storeDir, entry.Name(), ".scribe-base.md")

		// Only create base if SKILL.md exists and base doesn't.
		if _, err := os.Stat(skillMD); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, err := os.Stat(baseMD); err == nil {
			continue // base already exists
		}
		data, err := os.ReadFile(skillMD)
		if err != nil {
			return nil, fmt.Errorf("read %s for base copy: %w", skillMD, err)
		}
		if err := os.WriteFile(baseMD, data, 0o644); err != nil {
			return nil, fmt.Errorf("write base %s: %w", baseMD, err)
		}
	}

	// Update symlinks pointing to old slug paths.
	if len(movedSkills) > 0 {
		if err := updateSymlinks(movedSkills); err != nil {
			return warnings, fmt.Errorf("update symlinks: %w", err)
		}
	}

	// Remove empty slug directories.
	slugDirs2, _ := os.ReadDir(storeDir)
	for _, entry := range slugDirs2 {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(storeDir, entry.Name())
		children, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		// If the directory still contains subdirectories with SKILL.md, it's a slug dir
		// that wasn't fully emptied. Only remove if truly empty.
		if len(children) == 0 {
			_ = os.Remove(dir)
		}
	}

	// Mark migration complete so we don't rescan on every command.
	if err := os.WriteFile(markerPath, []byte("v2\n"), 0o644); err != nil {
		return warnings, fmt.Errorf("write migration marker: %w", err)
	}

	return warnings, nil
}

// contentHash computes SHA-256 of a file's contents, returning first 8 hex chars.
func contentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:8], nil
}

// updateSymlinks rewrites symlinks in tool dirs to point to the new flat paths.
func updateSymlinks(movedSkills map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	// Tool skill directories that may contain symlinks.
	toolDirs := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".cursor", "skills"),
	}

	for _, toolDir := range toolDirs {
		entries, err := os.ReadDir(toolDir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read tool dir %s: %w", toolDir, err)
		}

		for _, entry := range entries {
			link := filepath.Join(toolDir, entry.Name())
			target, err := os.Readlink(link)
			if err != nil {
				continue // Not a symlink.
			}

			// Check if this symlink points to any old path.
			if newPath, ok := movedSkills[target]; ok {
				// Replace symlink: remove then create.
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove symlink %s: %w", link, err)
				}
				if err := os.Symlink(newPath, link); err != nil {
					return fmt.Errorf("create symlink %s → %s: %w", link, newPath, err)
				}
			}
		}
	}

	return nil
}

