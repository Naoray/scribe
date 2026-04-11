package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// SkillFile is a single file within a downloaded skill directory.
type SkillFile = tools.SkillFile

// GitHubFetcher abstracts GitHub API operations needed by the sync engine.
type GitHubFetcher interface {
	FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]SkillFile, error)
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
	GetTree(ctx context.Context, owner, repo, ref string) ([]provider.TreeEntry, error)
}

// Syncer wires manifest, github, tools, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client   GitHubFetcher
	Provider provider.Provider // optional — if set, used for discovery and fetch
	Tools    []tools.Tool
	Emit     func(any) // receives events defined in events.go
	Executor CommandExecutor

	// ModifiedStrategy controls what to do when an outdated skill also has
	// unsynced local edits on disk.
	ModifiedStrategy ModifiedStrategy

	// TrustAll skips approval prompts for packages (--trust-all flag).
	TrustAll bool

	// ApprovalFunc is called when a package needs interactive approval.
	// Returns true if approved, false if denied.
	// If nil and TrustAll is false, packages needing approval are skipped.
	ApprovalFunc func(name, command, source string) bool
}

// ModifiedStrategy controls how sync treats outdated skills with local edits.
type ModifiedStrategy int

const (
	ModifiedStrategyMerge ModifiedStrategy = iota
	ModifiedStrategyPreferTheirs
)

// FetchManifest tries Provider.Discover first (if set), then falls back to
// direct file fetch with scribe.yaml → scribe.toml fallback.
func (s *Syncer) FetchManifest(ctx context.Context, owner, repo string) (*manifest.Manifest, error) {
	if s.Provider != nil {
		result, err := s.Provider.Discover(ctx, owner+"/"+repo)
		if err != nil {
			return nil, err
		}
		m := &manifest.Manifest{
			APIVersion: "scribe/v1",
			Kind:       "Registry",
			Catalog:    result.Entries,
		}
		if result.IsTeam {
			m.Team = &manifest.Team{Name: owner + "/" + repo}
		}
		return m, nil
	}

	// Legacy path: direct file fetch.
	m, isLegacy, err := manifest.FetchWithFallback(ctx, s.Client, owner, repo, migrate.Convert)
	if err != nil {
		return nil, err
	}
	if isLegacy {
		s.emit(LegacyFormatMsg{Repo: owner + "/" + repo})
	}
	return m, nil
}

// Diff fetches the team loadout and computes status for every skill
// without making any changes. Used by `scribe list`.
// Returns the parsed manifest alongside statuses so callers can reuse it.
func (s *Syncer) Diff(ctx context.Context, teamRepo string, st *state.State) ([]SkillStatus, *manifest.Manifest, error) {
	owner, repo, err := manifest.ParseOwnerRepo(teamRepo)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.FetchManifest(ctx, owner, repo)
	if err != nil {
		return nil, nil, fmt.Errorf("parse loadout: %w", err)
	}
	if !m.IsRegistry() {
		return nil, nil, fmt.Errorf("%s has no team section", teamRepo)
	}

	var statuses []SkillStatus
	storeDir, _ := tools.StoreDir()
	// Cache for package commit SHAs keyed by owner/repo/ref.
	commitSHACache := map[string]string{}
	// Cache for branch-skill tree listings keyed by owner/repo/ref.
	// A single GetTree call yields blob SHAs for every skill in the registry,
	// so we only hit the API once per (owner, repo, ref) tuple even across
	// dozens of catalog entries.
	treeCache := map[string][]provider.TreeEntry{}

	for i := range m.Catalog {
		entry := &m.Catalog[i]
		// Use bare name for lookup — flat storage model.
		installedPtr := lookupInstalled(st, entry.Name)
		locallyModified := false
		if installedPtr != nil && storeDir != "" {
			locallyModified = IsLocallyModified(filepath.Join(storeDir, entry.Name), installedPtr.InstalledHash)
		}

		latestSHA := ""
		src, err := manifest.ParseSource(entry.Source)
		// Resolve the latest identity signal for branch-pinned and package entries.
		// Branch skills compare blob SHAs (file content identity) via GetTree; packages
		// still compare commit SHAs via LatestCommitSHA (tree-based identity is a
		// follow-up PR). If the API is unavailable, latestSHA stays "" and
		// compareEntry handles that gracefully.
		if err == nil {
			switch {
			case entry.IsPackage():
				key := src.Owner + "/" + src.Repo + "/" + src.Ref
				if cached, ok := commitSHACache[key]; ok {
					latestSHA = cached
				} else {
					sha, cerr := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
					if cerr == nil {
						commitSHACache[key] = sha
						latestSHA = sha
					}
				}
			case src.IsBranch():
				key := src.Owner + "/" + src.Repo + "/" + src.Ref
				tree, ok := treeCache[key]
				if !ok {
					fetched, terr := s.Client.GetTree(ctx, src.Owner, src.Repo, src.Ref)
					if terr == nil {
						treeCache[key] = fetched
						tree = fetched
					}
				}
				if len(tree) > 0 {
					resolvedSHA, found := resolveSkillBlobSHA(tree, *entry)
					if found {
						latestSHA = resolvedSHA
					} else {
						latestSHA = missingSkillBlobSHA
					}
				}
			}
		}

		status := compareEntry(*entry, installedPtr, latestSHA, teamRepo, locallyModified)
		statuses = append(statuses, SkillStatus{
			Name:       entry.Name,
			Status:     status,
			Installed:  installedPtr,
			Entry:      entry,
			LoadoutRef: loadoutRef(*entry),
			Maintainer: entry.Maintainer(),
			IsPackage:  entry.IsPackage(),
			LatestSHA:  latestSHA,
		})
	}

	// Extra skills: installed locally with a source matching this registry,
	// but not present in the current catalog.
	catalogNames := make(map[string]bool, len(m.Catalog))
	for _, e := range m.Catalog {
		catalogNames[e.Name] = true
	}
	extraNames := make([]string, 0)
	for name, skill := range st.Installed {
		if catalogNames[name] {
			continue // already accounted for above
		}
		// Check if any source matches this registry.
		for _, src := range skill.Sources {
			if src.Registry == teamRepo {
				extraNames = append(extraNames, name)
				break
			}
		}
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		cp := st.Installed[name]
		statuses = append(statuses, SkillStatus{
			Name:      name,
			Status:    StatusExtra,
			Installed: &cp,
		})
	}

	return statuses, m, nil
}

// Run executes the full sync: diff, then install/update as needed.
// Emits events throughout. Updates state incrementally — a failed skill
// does not prevent successful skills from being recorded.
func (s *Syncer) Run(ctx context.Context, teamRepo string, st *state.State) error {
	statuses, _, err := s.Diff(ctx, teamRepo, st)
	if err != nil {
		return fmt.Errorf("sync %s: %w", teamRepo, err)
	}
	return s.apply(ctx, teamRepo, statuses, st)
}

func (s *Syncer) apply(ctx context.Context, teamRepo string, statuses []SkillStatus, st *state.State) error {
	// Emit resolved status for each skill (populates list view before downloads start).
	for _, sk := range statuses {
		s.emit(SkillResolvedMsg{sk})
	}

	summary := SyncCompleteMsg{}

	for _, sk := range statuses {
		switch sk.Status {
		case StatusCurrent, StatusExtra:
			s.emit(SkillSkippedMsg{Name: sk.Name})
			summary.Skipped++

		case StatusMissing, StatusOutdated:
			if sk.IsPackage {
				s.applyPackage(ctx, sk, teamRepo, st, &summary)
				continue
			}

			s.emit(SkillDownloadingMsg{Name: sk.Name})

			var tFiles []tools.SkillFile

			if s.Provider != nil {
				// Use Provider.Fetch — returns []tools.SkillFile directly.
				files, err := s.Provider.Fetch(ctx, *sk.Entry)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}
				// Apply the same infrastructure-file filter as the legacy path.
				for _, f := range files {
					if shouldInclude(f.Path) {
						tFiles = append(tFiles, f)
					}
				}
			} else {
				// Legacy path: direct FetchDirectory.
				src, err := manifest.ParseSource(sk.Entry.Source)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}

				skillPath := sk.Entry.Path
				if skillPath == "" {
					skillPath = sk.Name
				}

				files, err := s.Client.FetchDirectory(ctx, src.Owner, src.Repo, skillPath, src.Ref)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}

				for _, f := range files {
					if shouldInclude(f.Path) {
						tFiles = append(tFiles, f)
					}
				}
			}

			// Check for local modifications before writing.
			installed := lookupInstalled(st, sk.Name)
			if installed != nil && sk.Status == StatusOutdated {
				storeDir, sdErr := tools.StoreDir()
				if sdErr == nil {
					skillDir := filepath.Join(storeDir, sk.Name)
					if IsLocallyModified(skillDir, installed.InstalledHash) && s.ModifiedStrategy != ModifiedStrategyPreferTheirs {
						// Find the new upstream SKILL.md content for merge.
						var upstreamContent []byte
						for _, f := range tFiles {
							if f.Path == "SKILL.md" {
								upstreamContent = f.Content
								break
							}
						}

						if upstreamContent != nil {
							// Snapshot current version before merge overwrites SKILL.md.
							if snapErr := SnapshotVersion(skillDir, installed.Revision); snapErr != nil {
								s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("snapshot: %w", snapErr)})
								summary.Failed++
								continue
							}
							result, mergeErr := ThreeWayMerge(skillDir, upstreamContent)
							switch {
							case mergeErr != nil:
								s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("merge: %w", mergeErr)})
								summary.Failed++
								continue
							case result == MergeConflict:
								// Update state with new source info but flag as conflicted.
								s.updateSourceEntry(st, sk.Name, teamRepo, sk, installed)
								if err := st.Save(); err != nil {
									s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
								}
								s.emit(MergeConflictMsg{Name: sk.Name})
								summary.Failed++
								continue
							case result == MergeClean:
								// Clean merge — read the merged content back and update the hash.
								merged, readErr := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
								if readErr != nil {
									s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("read merged: %w", readErr)})
									summary.Failed++
									continue
								}
								s.updateSourceEntry(st, sk.Name, teamRepo, sk, installed)
								existing := st.Installed[sk.Name]
								existing.InstalledHash = ComputeFileHash(merged)
								existing.Revision = nextRevision(installed)
								st.Installed[sk.Name] = existing
								if err := st.Save(); err != nil {
									s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
								}
								_ = EnforceRetention(skillDir, DefaultMaxVersions)
								s.emit(SkillInstalledMsg{
									Name:    sk.Name,
									Updated: true,
									Merged:  true,
								})
								summary.Updated++
								continue
							}
						}
						// No SKILL.md in upstream or merge not applicable — fall through to overwrite.
					}
				}
			}

			// Snapshot current version before overwrite (no-op for new installs).
			if installed != nil && sk.Status == StatusOutdated {
				storeDir, sdErr := tools.StoreDir()
				if sdErr == nil {
					skillDir := filepath.Join(storeDir, sk.Name)
					if snapErr := SnapshotVersion(skillDir, installed.Revision); snapErr != nil {
						s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("snapshot: %w", snapErr)})
						summary.Failed++
						continue
					}
				}
			}

			// Write files to canonical store once, then symlink per target.
			canonicalDir, err := tools.WriteToStore(sk.Name, tFiles)
			if err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("write store: %w", err)})
				summary.Failed++
				continue
			}

			// Filter to the skill's effective tools (pinned mode respects user
			// selection; inherit mode uses every globally-enabled tool).
			effectiveTools := selectEffectiveTools(s.Tools, installed)

			var paths []string
			var toolNames []string
			toolFailed := false
			for _, t := range effectiveTools {
				links, err := t.Install(sk.Name, canonicalDir)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("link to %s: %w", t.Name(), err)})
					summary.Failed++
					toolFailed = true
					break
				}
				paths = append(paths, links...)
				toolNames = append(toolNames, t.Name())
			}
			if toolFailed {
				continue
			}

			// Compute hash of the installed SKILL.md content.
			var installedHash string
			for _, f := range tFiles {
				if f.Path == "SKILL.md" {
					installedHash = ComputeFileHash(f.Content)
					break
				}
			}

			// Parse source for ref.
			src, err := manifest.ParseSource(sk.Entry.Source)
			ref := ""
			if err == nil {
				ref = src.Ref
			}

			// Build sources: merge with existing sources from other registries.
			newSource := state.SkillSource{
				Registry:   teamRepo,
				Ref:        ref,
				LastSHA:    sk.LatestSHA,
				LastSynced: time.Now().UTC(),
			}
			sources := mergeSources(installed, newSource)

			// Preserve pinned ToolsMode across re-syncs. New skills default to
			// inherit; for pinned skills we still overwrite Tools with the
			// resolved names so stale entries (tool uninstalled globally)
			// don't linger in state.
			toolsMode := state.ToolsModeInherit
			if installed != nil {
				toolsMode = installed.ToolsMode
			}

			st.RecordInstall(sk.Name, state.InstalledSkill{
				Revision:      nextRevision(installed),
				InstalledHash: installedHash,
				Sources:       sources,
				Tools:         toolNames,
				ToolsMode:     toolsMode,
				Paths:         paths,
				ManagedPaths:  append([]string(nil), paths...),
			})
			// Save after each successful install — partial sync is safe.
			if err := st.Save(); err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
			}

			// Enforce version retention after successful write.
			if sk.Status == StatusOutdated {
				_ = EnforceRetention(canonicalDir, DefaultMaxVersions)
			}

			s.emit(SkillInstalledMsg{
				Name:    sk.Name,
				Updated: sk.Status == StatusOutdated,
			})
			if sk.Status == StatusOutdated {
				summary.Updated++
			} else {
				summary.Installed++
			}
		}
	}

	st.RecordSync()
	if err := st.Save(); err != nil {
		s.emit(summary)
		return fmt.Errorf("save final state: %w", err)
	}

	s.emit(summary)
	return nil
}

func (s *Syncer) emit(msg any) {
	if s.Emit != nil {
		s.Emit(msg)
	}
}

// RunWithDiff applies a pre-computed diff (statuses) directly.
// Used by tests and callers that already have statuses from Diff().
func (s *Syncer) RunWithDiff(ctx context.Context, teamRepo string, statuses []SkillStatus, st *state.State) error {
	return s.apply(ctx, teamRepo, statuses, st)
}

const defaultPackageTimeout = 5 * time.Minute

// toolCmd holds the resolved install/update commands for one active tool.
type toolCmd struct {
	toolName   string
	installCmd string
	updateCmd  string
}

// resolveToolCmds returns the install/update commands for each active tool.
// A tool is included if it has either an install or update command. Falls
// back to the global Install/Update fields when no per-tool entry exists.
// If no tools have any command, returns a single global entry (empty toolName).
func resolveToolCmds(entry *manifest.Entry, activeTools []tools.Tool) []toolCmd {
	var cmds []toolCmd
	for _, t := range activeTools {
		install := entry.InstallFor(t.Name())
		update := entry.UpdateFor(t.Name())
		if install != "" || update != "" {
			cmds = append(cmds, toolCmd{toolName: t.Name(), installCmd: install, updateCmd: update})
		}
	}
	// If no tool-specific commands, fall back to global (one unnamed entry).
	if len(cmds) == 0 && (entry.Install != "" || entry.Update != "") {
		cmds = append(cmds, toolCmd{installCmd: entry.Install, updateCmd: entry.Update})
	}
	return cmds
}

// selectEffectiveTools filters the globally-enabled tools down to the subset
// that should receive this skill, honoring ToolsMode. Order comes from the
// installed record for pinned mode (preserves user intent); from the global
// list for inherit mode (preserves tool config order).
func selectEffectiveTools(global []tools.Tool, installed *state.InstalledSkill) []tools.Tool {
	if installed == nil || installed.ToolsMode != state.ToolsModePinned {
		return global
	}
	byName := make(map[string]tools.Tool, len(global))
	for _, t := range global {
		byName[t.Name()] = t
	}
	out := make([]tools.Tool, 0, len(installed.Tools))
	for _, name := range installed.Tools {
		if t, ok := byName[name]; ok {
			out = append(out, t)
		}
	}
	return out
}

// formatCmds formats tool commands for display in approval prompts.
func formatCmds(cmds []toolCmd) string {
	if len(cmds) == 1 && cmds[0].toolName == "" {
		return cmds[0].installCmd
	}
	parts := make([]string, 0, len(cmds))
	for _, c := range cmds {
		parts = append(parts, fmt.Sprintf("[%s] %s", c.toolName, c.installCmd))
	}
	return strings.Join(parts, "\n")
}

func (s *Syncer) applyPackage(ctx context.Context, sk SkillStatus, teamRepo string, st *state.State, summary *SyncCompleteMsg) {
	if s.Executor == nil {
		s.emit(PackageErrorMsg{
			Name: sk.Name,
			Err:  fmt.Errorf("no command executor configured"),
		})
		summary.Failed++
		return
	}

	cmds := resolveToolCmds(sk.Entry, s.Tools)
	if len(cmds) == 0 {
		s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "no install command"})
		summary.Skipped++
		return
	}

	newHash := CommandHash(sk.Entry.Install, sk.Entry.Update, sk.Entry.Installs, sk.Entry.Updates)
	// Packages use bare name in state (flat storage model).
	stateName := sk.Name
	displayCmd := formatCmds(cmds)

	timeout := time.Duration(sk.Entry.Timeout) * time.Second
	if timeout == 0 {
		timeout = defaultPackageTimeout
	}

	switch sk.Status {
	case StatusMissing:
		approved := s.checkApproval(sk, stateName, st, newHash, displayCmd)
		if !approved {
			s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "approval_required"})
			summary.Skipped++
			return
		}

		s.emit(PackageInstallingMsg{Name: sk.Name})

		for _, c := range cmds {
			if c.installCmd == "" {
				continue
			}
			_, stderr, err := s.Executor.Execute(ctx, c.installCmd, timeout)
			if err != nil {
				s.emit(PackageErrorMsg{Name: sk.Name, Err: err, Stderr: stderr})
				summary.Failed++
				return
			}
		}

		src, parseErr := manifest.ParseSource(sk.Entry.Source)
		ref := ""
		if parseErr == nil {
			ref = src.Ref
		}

		installed := lookupInstalled(st, stateName)
		newSource := state.SkillSource{
			Registry:   teamRepo,
			Ref:        ref,
			LastSHA:    sk.LatestSHA,
			LastSynced: time.Now().UTC(),
		}

		st.RecordInstall(stateName, state.InstalledSkill{
			Revision:   nextRevision(installed),
			Sources:    mergeSources(installed, newSource),
			Type:       "package",
			InstallCmd: sk.Entry.Install,
			UpdateCmd:  sk.Entry.Update,
			CmdHash:    newHash,
			Approval:   "approved",
			ApprovedAt: time.Now().UTC(),
		})
		if err := st.Save(); err != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
		}

		s.emit(PackageInstalledMsg{Name: sk.Name})
		summary.Installed++

	case StatusOutdated:
		hasUpdate := false
		for _, c := range cmds {
			if c.updateCmd != "" {
				hasUpdate = true
				break
			}
		}
		if !hasUpdate {
			s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "no update command"})
			summary.Skipped++
			return
		}

		installed, ok := st.Installed[stateName]
		if ok && installed.CmdHash != "" && installed.CmdHash != newHash {
			s.emit(PackageHashMismatchMsg{
				Name:       sk.Name,
				OldCommand: installed.InstallCmd,
				NewCommand: displayCmd,
				Source:     sk.Entry.Source,
			})
			approved := s.checkApproval(sk, stateName, st, newHash, displayCmd)
			if !approved {
				s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "approval_required"})
				summary.Skipped++
				return
			}
		}

		s.emit(PackageUpdateMsg{Name: sk.Name})

		for _, c := range cmds {
			if c.updateCmd == "" {
				continue
			}
			_, stderr, err := s.Executor.Execute(ctx, c.updateCmd, timeout)
			if err != nil {
				s.emit(PackageErrorMsg{Name: sk.Name, Err: err, Stderr: stderr})
				summary.Failed++
				return
			}
		}

		existing := st.Installed[stateName]
		// Update source entry for this registry.
		src, parseErr := manifest.ParseSource(sk.Entry.Source)
		ref := ""
		if parseErr == nil {
			ref = src.Ref
		}
		newSource := state.SkillSource{
			Registry:   teamRepo,
			Ref:        ref,
			LastSHA:    sk.LatestSHA,
			LastSynced: time.Now().UTC(),
		}
		existing.Sources = mergeSources(&existing, newSource)
		existing.InstallCmd = sk.Entry.Install
		existing.UpdateCmd = sk.Entry.Update
		existing.CmdHash = newHash
		existing.Approval = "approved"
		existing.ApprovedAt = time.Now().UTC()
		st.Installed[stateName] = existing
		if err := st.Save(); err != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
		}

		s.emit(PackageUpdatedMsg{Name: sk.Name})
		summary.Updated++
	}
}

// checkApproval is pure: it returns whether the command is authorized to run,
// without mutating state. The caller persists approval ONLY after the command
// actually succeeds (see applyPackage).
func (s *Syncer) checkApproval(sk SkillStatus, stateName string, st *state.State, newHash, installCmd string) bool {
	if s.TrustAll {
		return true
	}

	if installed, ok := st.Installed[stateName]; ok {
		if installed.Approval == "approved" && installed.CmdHash == newHash {
			return true
		}
	}

	if s.ApprovalFunc != nil {
		s.emit(PackageInstallPromptMsg{
			Name:    sk.Name,
			Command: installCmd,
			Source:  sk.Entry.Source,
		})
		if s.ApprovalFunc(sk.Name, installCmd, sk.Entry.Source) {
			s.emit(PackageApprovedMsg{Name: sk.Name})
			return true
		}
		s.emit(PackageDeniedMsg{Name: sk.Name})
		return false
	}

	return false
}

// loadoutRef extracts the human-readable version ref from a catalog entry.
func loadoutRef(entry manifest.Entry) string {
	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return "?"
	}
	return src.Ref
}

// lookupInstalled returns a pointer to the installed skill, or nil if not found.
func lookupInstalled(st *state.State, name string) *state.InstalledSkill {
	installed, ok := st.Installed[name]
	if !ok {
		return nil
	}
	return &installed
}

// nextRevision returns the next revision number (existing + 1, or 1 for new installs).
func nextRevision(installed *state.InstalledSkill) int {
	if installed == nil {
		return 1
	}
	return installed.Revision + 1
}

// mergeSources combines existing sources with a new source entry.
// If the registry already exists in sources, it is updated. Otherwise appended.
func mergeSources(installed *state.InstalledSkill, newSource state.SkillSource) []state.SkillSource {
	if installed == nil {
		return []state.SkillSource{newSource}
	}
	sources := make([]state.SkillSource, 0, len(installed.Sources)+1)
	found := false
	for _, s := range installed.Sources {
		if s.Registry == newSource.Registry {
			sources = append(sources, newSource)
			found = true
		} else {
			sources = append(sources, s)
		}
	}
	if !found {
		sources = append(sources, newSource)
	}
	return sources
}

// updateSourceEntry updates the source entry for a registry in the installed skill state.
// Used after merge operations where we don't want to fully overwrite the install record.
func (s *Syncer) updateSourceEntry(st *state.State, skillName, teamRepo string, sk SkillStatus, installed *state.InstalledSkill) {
	src, parseErr := manifest.ParseSource(sk.Entry.Source)
	ref := ""
	if parseErr == nil {
		ref = src.Ref
	}
	newSource := state.SkillSource{
		Registry:   teamRepo,
		Ref:        ref,
		LastSHA:    sk.LatestSHA,
		LastSynced: time.Now().UTC(),
	}

	existing := st.Installed[skillName]
	existing.Sources = mergeSources(installed, newSource)
	st.Installed[skillName] = existing
}
