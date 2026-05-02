package projectmigrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Naoray/scribe/internal/state"
)

type UndoResult struct {
	RestoredLinks        int    `json:"restored_links"`
	RestoredProjectFiles int    `json:"restored_project_files"`
	DeletedProjectFiles  int    `json:"deleted_project_files"`
	Snapshot             string `json:"snapshot"`
}

func Undo(snapshot *Snapshot, snapshotPath string) (UndoResult, error) {
	result := UndoResult{Snapshot: snapshotPath}
	if snapshot == nil {
		return result, fmt.Errorf("no migration to undo")
	}
	st, err := state.Load()
	if err != nil {
		return result, err
	}
	if snapshot.StateHash != "" && hashCurrentProjections(st, snapshot) != snapshot.StateHash {
		return result, fmt.Errorf("state changed since migration; resolve manually")
	}

	for _, link := range snapshot.Discovery.GlobalSymlinks {
		if err := os.MkdirAll(filepath.Dir(link.Path), 0o755); err != nil {
			return result, fmt.Errorf("create global link dir: %w", err)
		}
		if err := os.Remove(link.Path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("remove current global path %s: %w", link.Path, err)
		}
		if err := os.Symlink(link.CanonicalPath, link.Path); err != nil {
			return result, fmt.Errorf("restore global symlink %s: %w", link.Path, err)
		}
		result.RestoredLinks++
	}

	for path, data := range snapshot.PreviousProjectFiles {
		if data == nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("delete project file %s: %w", path, err)
			}
			result.DeletedProjectFiles++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, fmt.Errorf("create project file dir: %w", err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return result, fmt.Errorf("restore project file %s: %w", path, err)
		}
		result.RestoredProjectFiles++
	}

	for skill, projections := range snapshot.PreviousProjections {
		installed, ok := st.Installed[skill]
		if !ok {
			continue
		}
		installed.Projections = append([]state.ProjectionEntry(nil), projections...)
		st.Installed[skill] = installed
	}
	if err := st.Save(); err != nil {
		return result, err
	}
	if snapshotPath != "" {
		if err := os.Remove(snapshotPath); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("delete migration snapshot: %w", err)
		}
	}
	return result, nil
}
