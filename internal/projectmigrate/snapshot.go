package projectmigrate

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

const snapshotVersion = 1
const snapshotTimeLayout = "2006-01-02T15:04:05.000Z07:00"
const snapshotRetention = 10

type Snapshot struct {
	Version              int                                `json:"version"`
	Timestamp            time.Time                          `json:"timestamp"`
	Discovery            Discovery                          `json:"discovery"`
	Plan                 MigrationPlan                      `json:"plan"`
	PreviousProjectFiles map[string][]byte                  `json:"previous_project_files"`
	PreviousProjections  map[string][]state.ProjectionEntry `json:"previous_projections"`
	StateHash            string                             `json:"state_hash,omitempty"`
}

func WriteSnapshot(snapshot Snapshot) (string, error) {
	if snapshot.Version == 0 {
		snapshot.Version = snapshotVersion
	}
	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now().UTC()
	} else {
		snapshot.Timestamp = snapshot.Timestamp.UTC()
	}
	dir, err := state.MigrationSnapshotsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create migration history dir: %w", err)
	}
	path := filepath.Join(dir, snapshot.Timestamp.Format(snapshotTimeLayout)+".json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal migration snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write migration snapshot: %w", err)
	}
	if err := pruneSnapshots(dir); err != nil {
		return "", err
	}
	return path, nil
}

func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read migration snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("parse migration snapshot: %w", err)
	}
	return &snapshot, nil
}

func LatestSnapshotPath() (string, error) {
	dir, err := state.MigrationSnapshotsDir()
	if err != nil {
		return "", err
	}
	paths, err := snapshotPaths(dir)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no migration to undo")
	}
	return paths[len(paths)-1], nil
}

func captureSnapshot(discovery Discovery, plan MigrationPlan) (Snapshot, error) {
	previousFiles := make(map[string][]byte, len(plan.ProjectFiles))
	for _, change := range plan.ProjectFiles {
		data, err := os.ReadFile(change.File)
		if errors.Is(err, fs.ErrNotExist) {
			previousFiles[change.File] = nil
			continue
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("read previous project file %s: %w", change.File, err)
		}
		previousFiles[change.File] = data
	}

	st, err := state.Load()
	if err != nil {
		return Snapshot{}, err
	}
	previousProjections := map[string][]state.ProjectionEntry{}
	for _, skill := range uniqueSkills(plan.GlobalLinks) {
		if installed, ok := st.Installed[skill]; ok {
			previousProjections[skill] = append([]state.ProjectionEntry(nil), installed.Projections...)
		}
	}
	expected := cloneState(st)
	applyMigrationProjections(expected, plan, true)

	return Snapshot{
		Version:              snapshotVersion,
		Timestamp:            time.Now().UTC(),
		Discovery:            discovery,
		Plan:                 plan,
		PreviousProjectFiles: previousFiles,
		PreviousProjections:  previousProjections,
		StateHash:            hashCurrentProjections(expected, &Snapshot{PreviousProjections: previousProjections}),
	}, nil
}

func cloneState(st *state.State) *state.State {
	data, _ := json.Marshal(st)
	var cloned state.State
	_ = json.Unmarshal(data, &cloned)
	return &cloned
}

func hashCurrentProjections(st *state.State, snapshot *Snapshot) string {
	projections := map[string][]state.ProjectionEntry{}
	for skill := range snapshot.PreviousProjections {
		if installed, ok := st.Installed[skill]; ok {
			projections[skill] = installed.Projections
		}
	}
	return hashProjections(projections)
}

func hashProjections(projections map[string][]state.ProjectionEntry) string {
	data, _ := json.Marshal(projections)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func snapshotPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read migration history dir: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func pruneSnapshots(dir string) error {
	paths, err := snapshotPaths(dir)
	if err != nil {
		return err
	}
	for len(paths) > snapshotRetention {
		if err := os.Remove(paths[0]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("prune migration snapshot: %w", err)
		}
		paths = paths[1:]
	}
	return nil
}
