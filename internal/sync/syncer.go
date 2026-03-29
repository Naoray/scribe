package sync

import (
	"context"
	"fmt"
	"sort"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/targets"
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

// Syncer wires manifest, github, targets, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client  GitHubFetcher
	Targets []targets.Target
	Emit    func(any) // receives events defined in events.go
}

// Diff fetches the team loadout and computes status for every skill
// without making any changes. Used by `scribe list`.
// Returns the parsed manifest alongside statuses so callers can reuse it.
func (s *Syncer) Diff(ctx context.Context, teamRepo string, st *state.State) ([]SkillStatus, *manifest.Manifest, error) {
	owner, repo, err := manifest.ParseOwnerRepo(teamRepo)
	if err != nil {
		return nil, nil, err
	}

	raw, err := s.Client.FetchFile(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		return nil, nil, fmt.Errorf("fetch loadout: %w", err)
	}

	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parse loadout: %w", err)
	}
	if !m.IsLoadout() {
		return nil, nil, fmt.Errorf("%s/scribe.toml has no [team] section", teamRepo)
	}

	var statuses []SkillStatus

	// Sort skill names for deterministic output.
	names := make([]string, 0, len(m.Skills))
	for name := range m.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	// Skills in the loadout.
	for _, name := range names {
		skill := m.Skills[name]
		installed := st.Installed[name]
		installedPtr := &installed
		if _, ok := st.Installed[name]; !ok {
			installedPtr = nil
		}

		latestSHA := ""
		src, err := manifest.ParseSource(skill.Source)
		if err == nil && src.IsBranch() {
			sha, err := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
			if err == nil {
				latestSHA = sha
			}
		}

		status := compareSkill(skill, installedPtr, latestSHA)
		statuses = append(statuses, SkillStatus{
			Name:       name,
			Status:     status,
			Installed:  installedPtr,
			LoadoutRef: loadoutRef(skill),
			Maintainer: skill.Maintainer(),
		})
	}

	// Skills installed locally but not in the loadout.
	for name, installed := range st.Installed {
		if _, inLoadout := m.Skills[name]; !inLoadout {
			cp := installed
			statuses = append(statuses, SkillStatus{
				Name:      name,
				Status:    StatusExtra,
				Installed: &cp,
			})
		}
	}

	return statuses, m, nil
}

// Run executes the full sync: diff, then install/update as needed.
// Emits events throughout. Updates state incrementally — a failed skill
// does not prevent successful skills from being recorded.
func (s *Syncer) Run(ctx context.Context, teamRepo string, st *state.State) error {
	statuses, m, err := s.Diff(ctx, teamRepo, st)
	if err != nil {
		return err
	}

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
			s.emit(SkillDownloadingMsg{Name: sk.Name})

			skill := m.Skills[sk.Name]
			src, err := manifest.ParseSource(skill.Source)
			if err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
				summary.Failed++
				continue
			}

			skillPath := skill.Path
			if skillPath == "" {
				skillPath = sk.Name
			}

			files, err := s.Client.FetchDirectory(ctx, src.Owner, src.Repo, skillPath, src.Ref)
			if err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
				summary.Failed++
				continue
			}

			// Convert sync.SkillFile → targets.SkillFile for the store writer.
			tFiles := make([]targets.SkillFile, len(files))
			for i, f := range files {
				tFiles[i] = targets.SkillFile{Path: f.Path, Content: f.Content}
			}

			// Write files to canonical store once, then symlink per target.
			canonicalDir, err := targets.WriteToStore(sk.Name, tFiles)
			if err != nil {
				s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("write store: %w", err)})
				summary.Failed++
				continue
			}

			var paths []string
			var targetNames []string
			for _, t := range s.Targets {
				links, err := t.Install(sk.Name, canonicalDir)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("link to %s: %w", t.Name(), err)})
					summary.Failed++
					break
				}
				paths = append(paths, links...)
				targetNames = append(targetNames, t.Name())
			}

			latestSHA := ""
			if src.IsBranch() {
				sha, err := s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("latest SHA for %s: %w", sk.Name, err)})
					// Non-fatal: continue with empty SHA.
				} else {
					latestSHA = sha
				}
			}

			st.RecordInstall(sk.Name, state.InstalledSkill{
				Version:   src.Ref,
				CommitSHA: latestSHA,
				Source:    skill.Source,
				Targets:   targetNames,
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

// loadoutRef extracts the human-readable version ref from a skill entry.
func loadoutRef(skill manifest.Skill) string {
	src, err := manifest.ParseSource(skill.Source)
	if err != nil {
		return "?"
	}
	return src.Ref
}
