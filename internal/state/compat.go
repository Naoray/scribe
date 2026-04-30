package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const projectFileName = ".scribe.yaml"

// LegacyGlobalProjectionCompat describes whether Scribe should preserve the
// pre-project global projection behavior for this invocation.
type LegacyGlobalProjectionCompat struct {
	Enabled              bool
	CWD                  string
	HasGlobalProjections bool
	ProjectFile          string
}

// DetectLegacyGlobalProjectionCompat enables compatibility mode only when
// state still records global projections and the current directory is not
// inside a project with .scribe.yaml.
//
// TODO(v1.0): remove this legacy global-projection compatibility path and
// direct users to clean up orphaned global symlinks via migration.
func DetectLegacyGlobalProjectionCompat(st *State, cwd string) (LegacyGlobalProjectionCompat, error) {
	result := LegacyGlobalProjectionCompat{CWD: cwd}
	if st == nil {
		return result, nil
	}

	result.HasGlobalProjections = st.HasLegacyGlobalProjections()
	if !result.HasGlobalProjections {
		return result, nil
	}

	projectFile, err := findProjectFile(cwd)
	if err != nil {
		return result, err
	}
	result.ProjectFile = projectFile
	result.Enabled = projectFile == ""
	return result, nil
}

// HasLegacyGlobalProjections reports whether any installed skill still has an
// empty-project projection entry, the state shape used before project-local
// projection was introduced.
func (s *State) HasLegacyGlobalProjections() bool {
	if s == nil {
		return false
	}
	for _, installed := range s.Installed {
		for _, projection := range installed.Projections {
			if projection.Project == "" && len(projection.Tools) > 0 {
				return true
			}
		}
	}
	return false
}

func findProjectFile(startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat start dir: %w", err)
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		candidate := filepath.Join(dir, projectFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat project file: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
