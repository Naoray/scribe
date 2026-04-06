package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Naoray/scribe/internal/paths"
)

// State is the contents of ~/.scribe/state.json.
type State struct {
	LastSync  time.Time                `json:"last_sync,omitempty"`
	Installed map[string]InstalledSkill `json:"installed"`
}

// InstalledSkill records everything needed to detect updates and uninstall.
type InstalledSkill struct {
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Tools       []string  `json:"tools"`
	Paths       []string  `json:"paths"`

	// Package-specific fields (omitted for regular skills).
	Type       string    `json:"type,omitempty"`
	InstallCmd string    `json:"install_cmd,omitempty"`
	UpdateCmd  string    `json:"update_cmd,omitempty"`
	CmdHash    string    `json:"cmd_hash,omitempty"`
	Approval   string    `json:"approval,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}

// Legacy structs for migration from older state formats.
type legacyState struct {
	Team      *legacyTeamState           `json:"team,omitempty"`
	LastSync  *time.Time                 `json:"last_sync,omitempty"`
	Installed map[string]json.RawMessage `json:"installed"`
}

type legacyTeamState struct {
	LastSync time.Time `json:"last_sync,omitempty"`
}

type legacyInstalledSkill struct {
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Targets     []string  `json:"targets,omitempty"`
	Tools       []string  `json:"tools,omitempty"`
	Paths       []string  `json:"paths"`
	Registries  []string  `json:"registries,omitempty"`
	Type        string    `json:"type,omitempty"`
	InstallCmd  string    `json:"install_cmd,omitempty"`
	UpdateCmd   string    `json:"update_cmd,omitempty"`
	CmdHash     string    `json:"cmd_hash,omitempty"`
	Approval    string    `json:"approval,omitempty"`
	ApprovedAt  time.Time `json:"approved_at,omitempty"`
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
	return parseAndMigrate(data)
}

// parseAndMigrate handles 3 migrations:
// 1. Promote team.last_sync to top-level LastSync
// 2. Rename targets → tools in each InstalledSkill
// 3. Namespace bare keys using Registries[0] owner prefix
func parseAndMigrate(data []byte) (*State, error) {
	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	s := &State{
		Installed: make(map[string]InstalledSkill, len(legacy.Installed)),
	}

	// Migration 1: Promote team.last_sync to top-level.
	if legacy.LastSync != nil {
		s.LastSync = *legacy.LastSync
	} else if legacy.Team != nil && !legacy.Team.LastSync.IsZero() {
		s.LastSync = legacy.Team.LastSync
	}

	// Migration 2+3: Parse each installed skill, rename targets->tools, namespace keys.
	for name, raw := range legacy.Installed {
		var ls legacyInstalledSkill
		if err := json.Unmarshal(raw, &ls); err != nil {
			return nil, fmt.Errorf("parse installed skill %q: %w", name, err)
		}

		tools := ls.Tools
		if len(tools) == 0 && len(ls.Targets) > 0 {
			tools = ls.Targets
		}

		skill := InstalledSkill{
			Version:     ls.Version,
			CommitSHA:   ls.CommitSHA,
			Source:      ls.Source,
			InstalledAt: ls.InstalledAt,
			Tools:       tools,
			Paths:       ls.Paths,
			Type:        ls.Type,
			InstallCmd:  ls.InstallCmd,
			UpdateCmd:   ls.UpdateCmd,
			CmdHash:     ls.CmdHash,
			Approval:    ls.Approval,
			ApprovedAt:  ls.ApprovedAt,
		}

		nsKey := namespaceKey(name, ls.Registries)
		s.Installed[nsKey] = skill
	}

	if s.Installed == nil {
		s.Installed = make(map[string]InstalledSkill)
	}
	return s, nil
}

// namespaceKey ensures every skill key contains an owner prefix.
// Already-namespaced keys (containing "/") pass through unchanged.
// Bare keys get prefixed with the first registry's owner or "local/".
func namespaceKey(name string, registries []string) string {
	if strings.Contains(name, "/") {
		return name
	}
	if len(registries) > 0 {
		owner, _, _ := strings.Cut(registries[0], "/")
		return owner + "/" + name
	}
	return "local/" + name
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
	s.LastSync = time.Now().UTC()
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
	return paths.StatePath()
}

// Dir returns the path to the ~/.scribe directory.
func Dir() (string, error) {
	return paths.ScribeDir()
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
