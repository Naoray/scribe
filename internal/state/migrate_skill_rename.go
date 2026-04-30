package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/paths"
)

const (
	OldEmbeddedSkillName = "scribe-agent"
	EmbeddedSkillName    = "scribe"
)

type EmbeddedSkillRenameMigrationResult struct {
	Changed  bool
	Conflict bool
	Warnings []string
}

// MigrateEmbeddedSkillRename renames the v1.0 embedded bootstrap skill state
// from "scribe-agent" to "scribe". It is intentionally idempotent.
func MigrateEmbeddedSkillRename(s *State) (EmbeddedSkillRenameMigrationResult, error) {
	var result EmbeddedSkillRenameMigrationResult
	if s == nil || s.Installed == nil {
		return result, nil
	}
	oldSkill, hasOld := s.Installed[OldEmbeddedSkillName]
	_, hasNew := s.Installed[EmbeddedSkillName]
	if !hasOld {
		return result, nil
	}
	if hasNew {
		result.Conflict = true
		return result, nil
	}

	storeDir, err := paths.StoreDir()
	if err != nil {
		return result, fmt.Errorf("resolve store dir: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return result, fmt.Errorf("home dir: %w", err)
	}

	oldCanonical := filepath.Join(storeDir, OldEmbeddedSkillName)
	newCanonical := filepath.Join(storeDir, EmbeddedSkillName)
	warnings, err := renameCanonicalStore(oldCanonical, newCanonical)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	projectionMoves := embeddedProjectionMoves(home, oldSkill, oldCanonical, newCanonical)
	for _, move := range projectionMoves {
		changed, warning, err := repointManagedProjection(move)
		if err != nil {
			return result, err
		}
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		if changed {
			result.Changed = true
		}
	}

	delete(s.Installed, OldEmbeddedSkillName)
	oldSkill.Paths = renameSkillPaths(oldSkill.Paths)
	oldSkill.ManagedPaths = renameSkillPaths(oldSkill.ManagedPaths)
	for i := range oldSkill.Conflicts {
		oldSkill.Conflicts[i].Path = renameSkillPath(oldSkill.Conflicts[i].Path)
	}
	for i := range oldSkill.Sources {
		oldSkill.Sources[i].Path = renameSkillPath(oldSkill.Sources[i].Path)
	}
	s.Installed[EmbeddedSkillName] = oldSkill

	result.Changed = true
	return result, nil
}

func renameCanonicalStore(oldCanonical, newCanonical string) ([]string, error) {
	if _, err := os.Lstat(oldCanonical); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat old embedded skill store: %w", err)
	}
	if _, err := os.Lstat(newCanonical); err == nil {
		return []string{fmt.Sprintf("embedded skill store %s already exists; leaving %s in place", newCanonical, oldCanonical)}, nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat new embedded skill store: %w", err)
	}
	if err := os.Rename(oldCanonical, newCanonical); err != nil {
		return nil, fmt.Errorf("rename embedded skill store: %w", err)
	}
	return nil, nil
}

type projectionMove struct {
	OldPath string
	NewPath string
	Target  string
}

func embeddedProjectionMoves(home string, skill InstalledSkill, oldCanonical, newCanonical string) []projectionMove {
	moves := []projectionMove{
		{
			OldPath: filepath.Join(home, ".claude", "skills", OldEmbeddedSkillName),
			NewPath: filepath.Join(home, ".claude", "skills", EmbeddedSkillName),
			Target:  newCanonical,
		},
		{
			OldPath: filepath.Join(home, ".codex", "skills", OldEmbeddedSkillName),
			NewPath: filepath.Join(home, ".codex", "skills", EmbeddedSkillName),
			Target:  newCanonical,
		},
		{
			OldPath: filepath.Join(home, ".cursor", "rules", OldEmbeddedSkillName+".mdc"),
			NewPath: filepath.Join(home, ".cursor", "rules", EmbeddedSkillName+".mdc"),
			Target:  filepath.Join(newCanonical, ".cursor.mdc"),
		},
	}
	for _, projection := range skill.Projections {
		if projection.Project == "" {
			continue
		}
		moves = append(moves,
			projectionMove{
				OldPath: filepath.Join(projection.Project, ".claude", "skills", OldEmbeddedSkillName),
				NewPath: filepath.Join(projection.Project, ".claude", "skills", EmbeddedSkillName),
				Target:  newCanonical,
			},
			projectionMove{
				OldPath: filepath.Join(projection.Project, ".codex", "skills", OldEmbeddedSkillName),
				NewPath: filepath.Join(projection.Project, ".codex", "skills", EmbeddedSkillName),
				Target:  newCanonical,
			},
			projectionMove{
				OldPath: filepath.Join(projection.Project, ".cursor", "rules", OldEmbeddedSkillName+".mdc"),
				NewPath: filepath.Join(projection.Project, ".cursor", "rules", EmbeddedSkillName+".mdc"),
				Target:  filepath.Join(newCanonical, ".cursor.mdc"),
			},
		)
	}
	for _, path := range append(append([]string{}, skill.Paths...), skill.ManagedPaths...) {
		newPath := renameSkillPath(path)
		if newPath == path {
			continue
		}
		moves = append(moves, projectionMove{
			OldPath: path,
			NewPath: newPath,
			Target:  migrationProjectionTarget(path, oldCanonical, newCanonical),
		})
	}
	return dedupeProjectionMoves(moves)
}

func migrationProjectionTarget(path, oldCanonical, newCanonical string) string {
	if strings.HasSuffix(path, OldEmbeddedSkillName+".mdc") {
		return filepath.Join(newCanonical, ".cursor.mdc")
	}
	if strings.HasPrefix(path, oldCanonical) {
		return renameSkillPath(path)
	}
	return newCanonical
}

func dedupeProjectionMoves(moves []projectionMove) []projectionMove {
	seen := map[string]bool{}
	unique := moves[:0]
	for _, move := range moves {
		key := move.OldPath + "\x00" + move.NewPath
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, move)
	}
	return unique
}

func repointManagedProjection(move projectionMove) (bool, string, error) {
	info, err := os.Lstat(move.OldPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("stat old embedded skill projection %s: %w", move.OldPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Sprintf("embedded skill projection %s is not a symlink; leaving it in place", move.OldPath), nil
	}
	if _, err := os.Lstat(move.NewPath); err == nil {
		return false, fmt.Sprintf("embedded skill projection %s already exists; leaving %s in place", move.NewPath, move.OldPath), nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, "", fmt.Errorf("stat new embedded skill projection %s: %w", move.NewPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(move.NewPath), 0o755); err != nil {
		return false, "", fmt.Errorf("create embedded skill projection dir: %w", err)
	}
	if err := os.Remove(move.OldPath); err != nil {
		return false, "", fmt.Errorf("remove old embedded skill projection %s: %w", move.OldPath, err)
	}
	if err := os.Symlink(move.Target, move.NewPath); err != nil {
		return false, "", fmt.Errorf("symlink embedded skill projection %s -> %s: %w", move.NewPath, move.Target, err)
	}
	return true, "", nil
}

func renameSkillPaths(values []string) []string {
	for i := range values {
		values[i] = renameSkillPath(values[i])
	}
	return values
}

func renameSkillPath(value string) string {
	value = strings.ReplaceAll(value, OldEmbeddedSkillName+".mdc", EmbeddedSkillName+".mdc")
	return strings.ReplaceAll(value, OldEmbeddedSkillName, EmbeddedSkillName)
}
