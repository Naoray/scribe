package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is the contents of ~/.scribe/state.json.
type State struct {
	Team      TeamState                `json:"team"`
	Installed map[string]InstalledSkill `json:"installed"`
}

type TeamState struct {
	LastSync time.Time `json:"last_sync,omitempty"`
}

// InstalledSkill records everything needed to detect updates and uninstall.
type InstalledSkill struct {
	Version     string    `json:"version"`               // tag (v1.0.0) or branch@sha (main@a3f2c1b)
	CommitSHA   string    `json:"commit_sha,omitempty"`  // populated for branch refs
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Targets     []string  `json:"targets"`
	Paths       []string  `json:"paths"`                 // absolute paths written on disk
	Registries  []string  `json:"registries,omitempty"`
}

// ShortSHA returns the first 7 chars of CommitSHA, or "" if not set.
func (s InstalledSkill) ShortSHA() string {
	if len(s.CommitSHA) >= 7 {
		return s.CommitSHA[:7]
	}
	return s.CommitSHA
}

// DisplayVersion returns the version string shown in `scribe list`.
// For branch refs: "main@a3f2c1b". For tags: "v1.0.0".
func (s InstalledSkill) DisplayVersion() string {
	if s.CommitSHA != "" {
		return s.Version + "@" + s.ShortSHA()
	}
	return s.Version
}

// Load reads state from disk. Returns an empty state if the file doesn't exist yet.
func Load() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{Installed: make(map[string]InstalledSkill)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Installed == nil {
		s.Installed = make(map[string]InstalledSkill)
	}
	return &s, nil
}

// Save writes state to disk atomically (write temp file, rename).
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

// RecordSync updates the last sync timestamp.
func (s *State) RecordSync() {
	s.Team.LastSync = time.Now().UTC()
}

// RecordInstall records a successful skill install. Safe to call mid-sync —
// the state file is only written when Save() is called.
func (s *State) RecordInstall(name string, skill InstalledSkill) {
	skill.InstalledAt = time.Now().UTC()
	s.Installed[name] = skill
}

// Remove deletes a skill from state (does not touch disk files).
func (s *State) Remove(name string) {
	delete(s.Installed, name)
}

func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".scribe", "state.json"), nil
}

// Dir returns the ~/.scribe/ directory path.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".scribe"), nil
}
