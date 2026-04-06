package sync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// SkillFile is a single file within a downloaded skill directory.
// Mirrors github.SkillFile so the sync package does not import github directly.
type SkillFile struct {
	Path    string
	Content []byte
}

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
	Client  GitHubFetcher
	Tools []tools.Tool
	Emit    func(any) // receives events defined in events.go
}

// FetchManifest tries scribe.yaml first, falls back to scribe.toml with warning.
func (s *Syncer) FetchManifest(ctx context.Context, owner, repo string) (*manifest.Manifest, error) {
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

	for i := range m.Catalog {
		entry := &m.Catalog[i]
		qualifiedName := registrySlug + "/" + entry.Name
		installedPtr := lookupInstalled(st, qualifiedName)

		latestSHA := ""
		src, err := manifest.ParseSource(entry.Source)
		if err == nil && (src.IsBranch() || entry.IsPackage()) {
			sha, err := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
			if err == nil {
				latestSHA = sha
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

			// Filter out repo infrastructure files that leak when skill path == repo root.
			var filtered []SkillFile
			for _, f := range files {
				if shouldInclude(f.Path) {
					filtered = append(filtered, f)
				}
			}

			// Convert sync.SkillFile → tools.SkillFile for the store writer.
			tFiles := make([]tools.SkillFile, len(filtered))
			for i, f := range filtered {
				tFiles[i] = tools.SkillFile{Path: f.Path, Content: f.Content}
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

			latestSHA := ""
			if src.IsBranch() {
				sha, err := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
				if err == nil {
					latestSHA = sha
				}
			}

			st.RecordInstall(qualifiedName, state.InstalledSkill{
				Version:   src.Ref,
				CommitSHA: latestSHA,
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
				Version: src.Ref,
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
