package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// shortSHA returns the first 7 chars of CommitSHA, or "" if not set.
func (s InstalledSkill) shortSHA() string {
	if len(s.CommitSHA) >= 7 {
		return s.CommitSHA[:7]
	}
	return s.CommitSHA
}

// DisplayVersion returns the version string shown in `scribe list`.
// For branch refs: "main@a3f2c1b". For tags: "v1.0.0".
func (s InstalledSkill) DisplayVersion() string {
	if s.CommitSHA != "" {
		return s.Version + "@" + s.shortSHA()
	}
	return s.Version
}

// Load reads state from disk. Returns an empty state if the file doesn't exist yet.
// A shared advisory lock is held while reading to prevent torn reads.
func Load() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}

	// Ensure the state directory exists so the lockfile can be created.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	lf, err := lockFile(path+".lock", false)
	if err != nil {
		return nil, fmt.Errorf("lock state: %w", err)
	}
	defer unlockFile(lf)

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
// An exclusive advisory lock is held during the write to prevent concurrent corruption.
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	lf, err := lockFile(path+".lock", true)
	if err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer unlockFile(lf)

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

// AddRegistry appends a registry to a skill's Registries list (dedup, case-insensitive).
func (s *State) AddRegistry(name, registry string) {
	skill, ok := s.Installed[name]
	if !ok {
		return
	}
	for _, r := range skill.Registries {
		if strings.EqualFold(r, registry) {
			return
		}
	}
	skill.Registries = append(skill.Registries, registry)
	s.Installed[name] = skill
}

// RemoveRegistry removes a registry from a skill's Registries list.
func (s *State) RemoveRegistry(name, registry string) {
	skill, ok := s.Installed[name]
	if !ok {
		return
	}
	filtered := skill.Registries[:0]
	for _, r := range skill.Registries {
		if !strings.EqualFold(r, registry) {
			filtered = append(filtered, r)
		}
	}
	skill.Registries = filtered
	s.Installed[name] = skill
}

// MigrateRegistries backfills the Registries field for skills that predate
// multi-registry support. Called from the cmd layer with config.TeamRepos[0].
func (s *State) MigrateRegistries(defaultRegistry string) {
	for name, skill := range s.Installed {
		if len(skill.Registries) == 0 {
			skill.Registries = []string{defaultRegistry}
			s.Installed[name] = skill
		}
	}
}

func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".scribe", "state.json"), nil
}

// lockFile acquires an advisory flock on the given path.
// Use exclusive=true for writes, exclusive=false (shared) for reads.
func lockFile(path string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	lockType := syscall.LOCK_SH
	if exclusive {
		lockType = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), lockType); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// unlockFile releases the advisory lock and closes the file.
func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}
