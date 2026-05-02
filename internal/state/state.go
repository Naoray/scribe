package state

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/paths"
)

// State is the contents of ~/.scribe/state.json.
type State struct {
	SchemaVersion int                         `json:"schema_version"`
	LastSync      time.Time                   `json:"last_sync,omitempty"`
	Installed     map[string]InstalledSkill   `json:"installed"`
	Kits          map[string]InstalledKit     `json:"kits"`
	Snippets      map[string]InstalledSnippet `json:"snippets"`
	// Schema v5 is shared with the kits/snippets pivot; its projection, kit,
	// and snippet indexes are additive siblings of this deny-list.
	RemovedByUser      []RemovedSkill               `json:"removed_by_user"`
	Migrations         map[string]bool              `json:"migrations,omitempty"`
	RegistryFailures   map[string]RegistryFailure   `json:"registry_failures,omitempty"`
	BinaryUpdateChecks map[string]BinaryUpdateCheck `json:"binary_update_checks,omitempty"`
}

// ProjectionEntry records the set of tool projections currently linked for a
// project. An empty Project is the legacy global projection.
type ProjectionEntry struct {
	Project string   `json:"project"`
	Tools   []string `json:"tools"`
	Source  string   `json:"source,omitempty"`
}

const (
	SourceSync      = "sync"
	SourceMigration = "migration"
)

// InstalledKit indexes an installed kit definition in the local state file.
type InstalledKit struct {
	Source  string   `json:"source,omitempty"`
	Version string   `json:"version,omitempty"`
	Skills  []string `json:"skills,omitempty"`
}

// InstalledSnippet indexes an installed snippet definition in the local state file.
type InstalledSnippet struct {
	Source  string   `json:"source,omitempty"`
	Version string   `json:"version,omitempty"`
	Targets []string `json:"targets,omitempty"`
}

// RemovedSkill records a user's intent not to reinstall a registry skill.
type RemovedSkill struct {
	Name      string    `json:"name"`
	Registry  string    `json:"registry"`
	RemovedAt time.Time `json:"removed_at"`
}

type BinaryUpdateCheck struct {
	LastSucceededAt time.Time `json:"last_succeeded_at,omitempty"`
}

type RegistryFailure struct {
	Consecutive int       `json:"consecutive"`
	Muted       bool      `json:"muted,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	LastFailure time.Time `json:"last_failure,omitempty"`
}

// Origin describes how a skill was acquired.
type Origin string

const (
	// OriginRegistry is the zero value: skill came from a registry.
	// Existing state files without an "origin" field deserialize as OriginRegistry.
	OriginRegistry Origin = ""
	// OriginLocal means the skill was adopted or hand-written locally.
	OriginLocal Origin = "local"
	// OriginBootstrap means the skill was installed from the embedded scribe bootstrap.
	OriginBootstrap Origin = "bootstrap"
)

// ToolsMode controls how the Tools field is interpreted at sync time.
type ToolsMode string

const (
	// ToolsModeInherit is the zero value: the skill installs to whichever
	// tools are globally enabled. Tools is a cache of the most recent
	// effective list and may be rewritten by sync.
	ToolsModeInherit ToolsMode = ""
	// ToolsModePinned means the user explicitly chose this skill's tools.
	// Sync must respect Tools verbatim (intersected with availability) and
	// must not overwrite it based on global toggles.
	ToolsModePinned ToolsMode = "pinned"
)

// Kind classifies how a tracked entry is stored and projected.
//
// KindSkill (zero-value, also legacy default) is the canonical skill case:
// files in ~/.scribe/skills/<name>/ and dir-symlinked into tool skill dirs.
//
// KindPackage is a self-installing multi-skill bundle. Files live in
// ~/.scribe/packages/<name>/ and are NEVER projected into agent skill dirs.
// Reconcile, discovery, and the list TUI all treat packages as opaque.
type Kind string

const (
	KindSkill   Kind = ""
	KindPackage Kind = "package"
)

// InstalledSkill records everything needed to detect updates and uninstall.
type InstalledSkill struct {
	Revision      int           `json:"revision"`
	InstalledHash string        `json:"installed_hash"`
	Sources       []SkillSource `json:"sources,omitempty"`
	InstalledAt   time.Time     `json:"installed_at"`
	// Deprecated in schema v5: projections replace the single global tools set.
	// Kept through v5 for parsing and compatibility with existing commands.
	Tools     []string  `json:"tools"`
	ToolsMode ToolsMode `json:"tools_mode,omitempty"`
	// Deprecated in schema v5: managed paths are superseded by projections.
	// Kept through v5 for parsing and compatibility with existing commands.
	Paths        []string             `json:"paths"`
	Projections  []ProjectionEntry    `json:"projections,omitempty"`
	ManagedPaths []string             `json:"managed_paths,omitempty"`
	Conflicts    []ProjectionConflict `json:"projection_conflicts,omitempty"`
	Origin       Origin               `json:"origin,omitempty"`

	// Kind distinguishes auto-detected tree packages (files under
	// ~/.scribe/packages/<name>/) from regular skills. Missing value on
	// legacy entries defaults to KindSkill; the first sync after upgrade
	// may flip an entry to KindPackage via the reclassification pass.
	Kind Kind `json:"kind,omitempty"`

	// Package-specific fields (omitted for regular skills).
	Type       string    `json:"type,omitempty"`
	InstallCmd string    `json:"install_cmd,omitempty"`
	UpdateCmd  string    `json:"update_cmd,omitempty"`
	CmdHash    string    `json:"cmd_hash,omitempty"`
	Approval   string    `json:"approval,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}

// IsPackage reports whether this state entry is a tree-package (new kind
// field) or a legacy manifest-declared command-only package (Type field).
// Both flavours skip projection into tool skill dirs.
func (i InstalledSkill) IsPackage() bool {
	return i.Kind == KindPackage || i.Type == "package"
}

// SkillSource records a registry that provides this skill.
type SkillSource struct {
	Registry   string            `json:"registry"`
	SourceRepo string            `json:"source_repo,omitempty"`
	Path       string            `json:"path,omitempty"`
	Author     string            `json:"author,omitempty"`
	Ref        string            `json:"ref"`
	LastSHA    string            `json:"last_sha"`
	BlobSHAs   map[string]string `json:"blob_shas,omitempty"`
	LastSynced time.Time         `json:"last_synced"`
}

// PushRegistry returns the repository that should receive local edits.
func (s SkillSource) PushRegistry() string {
	if s.SourceRepo != "" {
		return s.SourceRepo
	}
	return s.Registry
}

// ProjectionConflict records a divergent tool-facing projection that Scribe
// intentionally preserved during reconcile.
type ProjectionConflict struct {
	Tool      string    `json:"tool"`
	Path      string    `json:"path"`
	FoundHash string    `json:"found_hash"`
	SeenAt    time.Time `json:"seen_at"`
}

// Legacy structs for migration from older state formats.
type legacyState struct {
	SchemaVersion      int                          `json:"schema_version,omitempty"`
	Team               *legacyTeamState             `json:"team,omitempty"`
	LastSync           *time.Time                   `json:"last_sync,omitempty"`
	Installed          map[string]json.RawMessage   `json:"installed"`
	Kits               map[string]InstalledKit      `json:"kits,omitempty"`
	Snippets           map[string]InstalledSnippet  `json:"snippets,omitempty"`
	RemovedByUser      []RemovedSkill               `json:"removed_by_user,omitempty"`
	BinaryUpdateChecks map[string]BinaryUpdateCheck `json:"binary_update_checks,omitempty"`
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

	// New v2 fields that may already exist in state (if re-loaded after partial migration)
	Revision      int               `json:"revision,omitempty"`
	InstalledHash string            `json:"installed_hash,omitempty"`
	Sources       []SkillSource     `json:"sources,omitempty"`
	Origin        Origin            `json:"origin,omitempty"`
	ToolsMode     ToolsMode         `json:"tools_mode,omitempty"`
	Kind          Kind              `json:"kind,omitempty"`
	Projections   []ProjectionEntry `json:"projections,omitempty"`
}

// DisplayVersion returns the version string shown in `scribe list`.
// Returns "rev N" based on the revision counter.
func (s InstalledSkill) DisplayVersion() string {
	return fmt.Sprintf("rev %d", s.Revision)
}

// parseSourceString extracts registry and ref from legacy source strings.
// Format: "github:owner/repo@ref" → registry="owner/repo", ref="ref"
// Returns empty strings if format doesn't match.
func parseSourceString(source string) (registry, ref string) {
	// Strip "github:" prefix
	after, ok := strings.CutPrefix(source, "github:")
	if !ok {
		return "", ""
	}
	// Split on "@" to get registry and ref
	parts := strings.SplitN(after, "@", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// Load reads state from disk. Returns an empty state if the file doesn't exist yet.
// A shared advisory lock is held while reading to prevent torn reads.
func Load() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, fmt.Errorf("resolve state path: %w", err)
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
	if errors.Is(err, fs.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return emptyState(), nil
	}
	return parseAndMigrate(data)
}

func emptyState() *State {
	return &State{
		SchemaVersion:      5,
		Installed:          make(map[string]InstalledSkill),
		Kits:               map[string]InstalledKit{},
		Snippets:           map[string]InstalledSnippet{},
		RemovedByUser:      []RemovedSkill{},
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]RegistryFailure{},
		BinaryUpdateChecks: map[string]BinaryUpdateCheck{},
	}
}

// parseAndMigrate handles migrations:
//  1. Promote team.last_sync to top-level LastSync
//  2. Rename targets → tools in each InstalledSkill
//  3. Namespace bare keys using Registries[0] owner prefix
//  4. Schema v2: convert to bare keys, populate Sources, set Revision
//  5. Schema v3: bump the state schema while preserving existing Sources.
//  6. Schema v4: normalize branch-backed LastSHA values to the locally cached
//     SKILL.md blob SHA so blob-SHA diffs do not force needless reinstalls.
//  7. Schema v5: initialize the user removal deny-list.
//  8. Schema v5: seed legacy global tools/paths into Projections.
func parseAndMigrate(data []byte) (*State, error) {
	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	s := &State{
		SchemaVersion:      legacy.SchemaVersion,
		Installed:          make(map[string]InstalledSkill, len(legacy.Installed)),
		Kits:               map[string]InstalledKit{},
		Snippets:           map[string]InstalledSnippet{},
		RemovedByUser:      append([]RemovedSkill(nil), legacy.RemovedByUser...),
		Migrations:         map[string]bool{},
		RegistryFailures:   map[string]RegistryFailure{},
		BinaryUpdateChecks: map[string]BinaryUpdateCheck{},
	}
	if len(legacy.Kits) > 0 {
		s.Kits = legacy.Kits
	}
	if len(legacy.Snippets) > 0 {
		s.Snippets = legacy.Snippets
	}
	if len(legacy.BinaryUpdateChecks) > 0 {
		s.BinaryUpdateChecks = legacy.BinaryUpdateChecks
	}

	// Migration 1: Promote team.last_sync to top-level.
	if legacy.LastSync != nil {
		s.LastSync = *legacy.LastSync
	} else if legacy.Team != nil && !legacy.Team.LastSync.IsZero() {
		s.LastSync = legacy.Team.LastSync
	}

	// Migration 2+3: Parse each installed skill, rename targets->tools, namespace keys.
	type parsedEntry struct {
		key   string
		skill legacyInstalledSkill
	}
	entries := make([]parsedEntry, 0, len(legacy.Installed))

	for name, raw := range legacy.Installed {
		var ls legacyInstalledSkill
		if err := json.Unmarshal(raw, &ls); err != nil {
			return nil, fmt.Errorf("parse installed skill %q: %w", name, err)
		}

		tools := ls.Tools
		if len(tools) == 0 && len(ls.Targets) > 0 {
			tools = ls.Targets
		}
		ls.Tools = tools

		// Migration 3 (for pre-v2 states): namespace bare keys
		key := name
		if s.SchemaVersion < 2 {
			key = namespaceKey(name, ls.Registries)
		}

		entries = append(entries, parsedEntry{key: key, skill: ls})
	}

	// Migration 4: Schema v2 — convert namespaced keys to bare names, populate Sources.
	if s.SchemaVersion < 2 {
		for _, e := range entries {
			key := e.key
			ls := e.skill

			// Extract bare name from key
			bareName := key
			if idx := strings.LastIndex(key, "/"); idx >= 0 {
				bareName = key[idx+1:]
			}

			// Build Sources from legacy Source field
			var sources []SkillSource
			if ls.Source != "" {
				registry, ref := parseSourceString(ls.Source)
				if registry != "" {
					sources = []SkillSource{{
						Registry: registry,
						Ref:      ref,
					}}
				}
			}

			// Carry forward v2 fields if they already exist (partial migration)
			revision := ls.Revision
			if revision == 0 {
				revision = 1
			}
			// InstalledHash left empty — computed during directory migration when files are moved.
			installedHash := ls.InstalledHash
			if len(ls.Sources) > 0 {
				sources = ls.Sources
			}

			skill := legacyToSkill(ls)
			skill.Revision = revision
			skill.InstalledHash = installedHash
			skill.Sources = sources

			// If bareName already exists (two qualified keys collapsed),
			// merge Sources from both entries and keep the newer one as base.
			if existing, ok := s.Installed[bareName]; ok {
				if existing.InstalledAt.After(skill.InstalledAt) {
					existing.Sources = appendUniqueSources(existing.Sources, skill.Sources)
					skill = existing
				} else {
					skill.Sources = appendUniqueSources(skill.Sources, existing.Sources)
				}
			}

			s.Installed[bareName] = skill
		}

		s.SchemaVersion = 2
	} else {
		// Already v2+ — pass through unchanged
		for _, e := range entries {
			skill := legacyToSkill(e.skill)
			s.Installed[e.key] = skill
		}
	}

	// Migration 5: Schema v3 — preserve existing entries and only bump the
	// schema version. The next sync can refresh branch/package LastSHA values
	// from commit SHAs to blob SHAs in place.
	if s.SchemaVersion < 3 {
		s.SchemaVersion = 3
	}
	if s.SchemaVersion < 4 {
		normalizeBranchSourceSHAs(s)
		s.SchemaVersion = 4
	}
	if s.SchemaVersion < 5 {
		if s.RemovedByUser == nil {
			s.RemovedByUser = []RemovedSkill{}
		}
		s.SchemaVersion = 5
	}
	seedLegacyProjections(s)
	seedManagedPaths(s)

	return s, nil
}

// LocalNamespace is the namespace prefix for skills without a registry.
const LocalNamespace = "local"

// namespaceKey ensures every skill key contains a namespace prefix.
// Already-namespaced keys (containing "/") pass through unchanged.
// Bare keys get prefixed with the slugified registry or "local/".
func namespaceKey(name string, registries []string) string {
	if strings.Contains(name, "/") {
		return name
	}
	if len(registries) > 0 {
		slug := strings.ReplaceAll(registries[0], "/", "-")
		return slug + "/" + name
	}
	return LocalNamespace + "/" + name
}

// Save writes state to disk atomically (write temp file, rename).
// An exclusive advisory lock is held during the write to prevent concurrent corruption.
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return fmt.Errorf("resolve state path: %w", err)
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
		os.Remove(tmp)
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

// RecordRemovedByUser records user removal intent for each registry source.
func (s *State) RecordRemovedByUser(name string, sources []SkillSource) {
	if s.RemovedByUser == nil {
		s.RemovedByUser = []RemovedSkill{}
	}
	now := time.Now().UTC()
	for _, src := range sources {
		if src.Registry == "" {
			continue
		}
		replaced := false
		for i := range s.RemovedByUser {
			if s.RemovedByUser[i].Name == name && s.RemovedByUser[i].Registry == src.Registry {
				s.RemovedByUser[i].RemovedAt = now
				replaced = true
				break
			}
		}
		if !replaced {
			s.RemovedByUser = append(s.RemovedByUser, RemovedSkill{
				Name:      name,
				Registry:  src.Registry,
				RemovedAt: now,
			})
		}
	}
}

// IsRemovedByUser reports whether the registry/name pair is deny-listed.
func (s *State) IsRemovedByUser(registry, name string) bool {
	if s == nil {
		return false
	}
	for _, removed := range s.RemovedByUser {
		if removed.Registry == registry && removed.Name == name {
			return true
		}
	}
	return false
}

// ClearRemovedByUser removes deny-list entries for name. If registry is empty,
// all registries for that name are cleared.
func (s *State) ClearRemovedByUser(name, registry string) bool {
	if s == nil || len(s.RemovedByUser) == 0 {
		return false
	}
	kept := s.RemovedByUser[:0]
	changed := false
	for _, removed := range s.RemovedByUser {
		matchName := removed.Name == name
		matchRegistry := registry == "" || removed.Registry == registry
		if matchName && matchRegistry {
			changed = true
			continue
		}
		kept = append(kept, removed)
	}
	s.RemovedByUser = kept
	return changed
}

// ClearRemovedByRegistry removes all deny-list entries scoped to registry.
func (s *State) ClearRemovedByRegistry(registry string) bool {
	if s == nil || len(s.RemovedByUser) == 0 {
		return false
	}
	kept := s.RemovedByUser[:0]
	changed := false
	for _, removed := range s.RemovedByUser {
		if removed.Registry == registry {
			changed = true
			continue
		}
		kept = append(kept, removed)
	}
	s.RemovedByUser = kept
	return changed
}

func (s *State) HasMigration(name string) bool {
	return s.Migrations != nil && s.Migrations[name]
}

func (s *State) MarkMigration(name string) {
	if s.Migrations == nil {
		s.Migrations = map[string]bool{}
	}
	s.Migrations[name] = true
}

func (s *State) RecordRegistryFailure(repo string, err error, muteAfter int) (RegistryFailure, bool) {
	if s.RegistryFailures == nil {
		s.RegistryFailures = map[string]RegistryFailure{}
	}
	failure := s.RegistryFailures[repo]
	failure.Consecutive++
	failure.LastFailure = time.Now().UTC()
	if err != nil {
		failure.LastError = err.Error()
	}
	if muteAfter > 0 && failure.Consecutive >= muteAfter {
		failure.Muted = true
	}
	s.RegistryFailures[repo] = failure
	return failure, true
}

func (s *State) ClearRegistryFailure(repo string) bool {
	if s.RegistryFailures == nil {
		return false
	}
	if _, ok := s.RegistryFailures[repo]; !ok {
		return false
	}
	delete(s.RegistryFailures, repo)
	return true
}

func (s *State) RegistryFailure(repo string) RegistryFailure {
	if s.RegistryFailures == nil {
		return RegistryFailure{}
	}
	return s.RegistryFailures[repo]
}

const scribeBinaryUpdateCheckKey = "scribe"

// ScribeBinaryUpdateCheck returns the cached upgrade-check entry for scribe.
func (s *State) ScribeBinaryUpdateCheck() BinaryUpdateCheck {
	if s == nil || s.BinaryUpdateChecks == nil {
		return BinaryUpdateCheck{}
	}
	return s.BinaryUpdateChecks[scribeBinaryUpdateCheckKey]
}

// ScribeBinaryUpdateCooldownFresh reports whether the last successful scribe
// binary check is still within the 24-hour cooldown window.
func (s *State) ScribeBinaryUpdateCooldownFresh(now time.Time) bool {
	check := s.ScribeBinaryUpdateCheck()
	if check.LastSucceededAt.IsZero() {
		return false
	}
	return now.Sub(check.LastSucceededAt) < 24*time.Hour
}

// RecordScribeBinaryUpdateSuccess records a successful scribe binary check using UTC now.
func (s *State) RecordScribeBinaryUpdateSuccess() {
	s.RecordScribeBinaryUpdateSuccessAt(time.Now().UTC())
}

// RecordScribeBinaryUpdateSuccessAt records a successful scribe binary check at a specific time.
func (s *State) RecordScribeBinaryUpdateSuccessAt(at time.Time) {
	if s.BinaryUpdateChecks == nil {
		s.BinaryUpdateChecks = map[string]BinaryUpdateCheck{}
	}
	s.BinaryUpdateChecks[scribeBinaryUpdateCheckKey] = BinaryUpdateCheck{LastSucceededAt: at.UTC()}
}

// legacyToSkill converts a legacyInstalledSkill to an InstalledSkill,
// carrying over all fields that map directly.
func legacyToSkill(ls legacyInstalledSkill) InstalledSkill {
	kind := ls.Kind
	// Legacy manifest-declared command-only packages used Type="package"
	// exclusively; treat them as KindPackage on load so downstream checks can
	// simply consult Kind. We intentionally do NOT clear Type — downstream
	// code may still rely on it for manifest-type (command-only) handling.
	if kind == KindSkill && ls.Type == "package" {
		kind = KindPackage
	}
	return InstalledSkill{
		Revision:      ls.Revision,
		InstalledHash: ls.InstalledHash,
		Sources:       ls.Sources,
		InstalledAt:   ls.InstalledAt,
		Tools:         ls.Tools,
		ToolsMode:     ls.ToolsMode,
		Paths:         ls.Paths,
		Projections:   append([]ProjectionEntry(nil), ls.Projections...),
		ManagedPaths:  append([]string(nil), ls.Paths...),
		Origin:        ls.Origin,
		Kind:          kind,
		Type:          ls.Type,
		InstallCmd:    ls.InstallCmd,
		UpdateCmd:     ls.UpdateCmd,
		CmdHash:       ls.CmdHash,
		Approval:      ls.Approval,
		ApprovedAt:    ls.ApprovedAt,
	}
}

// appendUniqueSources appends sources from extra into base, skipping
// any that share the same Registry as an existing entry.
func appendUniqueSources(base, extra []SkillSource) []SkillSource {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s.Registry] = true
	}
	for _, s := range extra {
		if !seen[s.Registry] {
			base = append(base, s)
			seen[s.Registry] = true
		}
	}
	return base
}

func statePath() (string, error) {
	return paths.StatePath()
}

func MigrationSnapshotsDir() (string, error) {
	dir, err := paths.ScribeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "migration-history"), nil
}

func FileExists() (bool, error) {
	path, err := statePath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

func normalizeBranchSourceSHAs(s *State) {
	storeDir, err := paths.StoreDir()
	if err != nil {
		return
	}

	for name, skill := range s.Installed {
		if skill.IsPackage() {
			continue
		}
		changed := false
		for i := range skill.Sources {
			if !isBranchRef(skill.Sources[i].Ref) {
				continue
			}
			blobSHA := installedSkillBlobSHA(storeDir, name)
			if blobSHA == "" || skill.Sources[i].LastSHA == blobSHA {
				continue
			}
			skill.Sources[i].LastSHA = blobSHA
			changed = true
		}
		if changed {
			s.Installed[name] = skill
		}
	}
}

func seedManagedPaths(s *State) {
	for name, skill := range s.Installed {
		if skill.IsPackage() {
			// Packages own their own install lifecycle and their Paths
			// (if any) are command-output, not tool projections.
			continue
		}
		if len(skill.ManagedPaths) == 0 && len(skill.Paths) > 0 {
			skill.ManagedPaths = append([]string(nil), skill.Paths...)
			s.Installed[name] = skill
		}
	}
}

func seedLegacyProjections(s *State) {
	for name, skill := range s.Installed {
		if len(skill.Projections) > 0 {
			continue
		}
		if len(skill.Tools) == 0 && len(skill.Paths) == 0 {
			continue
		}
		skill.Projections = []ProjectionEntry{{
			Project: "",
			Tools:   append([]string{}, skill.Tools...),
		}}
		s.Installed[name] = skill
	}
}

func isBranchRef(ref string) bool {
	return !strings.HasPrefix(ref, "v") || !strings.Contains(ref, ".")
}

func installedSkillBlobSHA(storeDir, skillName string) string {
	baseDir := filepath.Join(storeDir, skillName)
	for _, candidate := range []string{
		filepath.Join(baseDir, ".scribe-base.md"),
		filepath.Join(baseDir, "SKILL.md"),
	} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return gitBlobSHA(data)
	}
	return ""
}

func gitBlobSHA(data []byte) string {
	payload := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf("%x", sum)
}
