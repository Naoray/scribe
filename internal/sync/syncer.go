package sync

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
}

// Syncer wires manifest, github, tools, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client   GitHubFetcher
	Provider provider.Provider // optional — if set, used for discovery and fetch
	Tools    []tools.Tool
	Emit     func(any) // receives events defined in events.go
}

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
	raw, err := s.Client.FetchFile(ctx, owner, repo, manifest.ManifestFilename, "HEAD")
	if err == nil {
		return manifest.Parse(raw)
	}

	raw, legacyErr := s.Client.FetchFile(ctx, owner, repo, manifest.LegacyManifestFilename, "HEAD")
	if legacyErr != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	s.emit(LegacyFormatMsg{Repo: owner + "/" + repo})
	return migrate.Convert(raw)
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

	registrySlug := tools.SlugifyRegistry(teamRepo)
	var statuses []SkillStatus
	shaCache := map[string]string{}

	for i := range m.Catalog {
		entry := &m.Catalog[i]
		qualifiedName := registrySlug + "/" + entry.Name
		installedPtr := lookupInstalled(st, qualifiedName)

		latestSHA := ""
		src, err := manifest.ParseSource(entry.Source)
		// Only resolve the latest SHA when using the real GitHub client.
		// When Provider is set, the Client may be a NoopFetcher that cannot make
		// API calls — silently skipping SHA resolution is correct here.
		if err == nil && (src.IsBranch() || entry.IsPackage()) && s.Provider == nil {
			key := src.Owner + "/" + src.Repo + "/" + src.Ref
			if cached, ok := shaCache[key]; ok {
				latestSHA = cached
			} else {
				sha, err := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
				if err == nil {
					shaCache[key] = sha
					latestSHA = sha
				}
			}
		}

		status := compareEntry(*entry, installedPtr, latestSHA)
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

	// Extra skills (installed but not in catalog) — scoped to this registry only.
	catalogNames := make(map[string]bool, len(m.Catalog))
	for _, e := range m.Catalog {
		catalogNames[e.Name] = true
	}
	extraNames := make([]string, 0)
	for name := range st.Installed {
		if !strings.HasPrefix(name, registrySlug+"/") {
			continue // belongs to a different registry
		}
		baseName := strings.TrimPrefix(name, registrySlug+"/")
		if !catalogNames[baseName] {
			extraNames = append(extraNames, name)
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
		return err
	}
	return s.apply(ctx, teamRepo, statuses, st)
}

// RunWithDiff is like Run but uses pre-computed diff results, avoiding a
// redundant manifest fetch when the caller already called Diff.
func (s *Syncer) RunWithDiff(ctx context.Context, teamRepo string, statuses []SkillStatus, st *state.State) error {
	return s.apply(ctx, teamRepo, statuses, st)
}

func (s *Syncer) apply(ctx context.Context, teamRepo string, statuses []SkillStatus, st *state.State) error {
	// Emit resolved status for each skill (populates list view before downloads start).
	for _, sk := range statuses {
		s.emit(SkillResolvedMsg{sk})
	}

	summary := SyncCompleteMsg{}
	registrySlug := tools.SlugifyRegistry(teamRepo)

	for _, sk := range statuses {
		switch sk.Status {
		case StatusCurrent, StatusExtra:
			s.emit(SkillSkippedMsg{Name: sk.Name})
			summary.Skipped++

		case StatusMissing, StatusOutdated:
			if sk.IsPackage {
				s.emit(PackageSkippedMsg{Name: sk.Name, Reason: "package install not yet implemented"})
				summary.Skipped++
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

			// Write files to canonical store once, then symlink per target.
			canonicalDir, err := tools.WriteToStore(registrySlug, sk.Name, tFiles)
			if err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("write store: %w", err)})
				summary.Failed++
				continue
			}

			qualifiedName := registrySlug + "/" + sk.Name
			var paths []string
			var toolNames []string
			toolFailed := false
			for _, t := range s.Tools {
				links, err := t.Install(qualifiedName, canonicalDir)
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

			// Parse source for version display.
			src, err := manifest.ParseSource(sk.Entry.Source)
			version := "unknown"
			if err == nil {
				version = src.Ref
			}

			st.RecordInstall(qualifiedName, state.InstalledSkill{
				Version:   version,
				CommitSHA: sk.LatestSHA,
				Source:    sk.Entry.Source,
				Tools:     toolNames,
				Paths:     paths,
			})
			// Save after each successful install — partial sync is safe.
			if err := st.Save(); err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("save state after %s: %w", sk.Name, err)})
			}

			s.emit(SkillInstalledMsg{
				Name:    sk.Name,
				Version: version,
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
