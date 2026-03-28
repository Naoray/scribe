package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/targets"

	gh "github.com/Naoray/scribe/internal/github"
)

// Syncer wires manifest, github, targets, and state together.
// It emits events via the Emit callback — the caller decides whether
// to forward them to a Bubbletea program or log them to stdout.
type Syncer struct {
	Client  *gh.Client
	Targets []targets.Target
	Emit    func(any) // receives events defined in events.go
}

// Diff fetches the team loadout and computes status for every skill
// without making any changes. Used by `scribe list`.
func (s *Syncer) Diff(ctx context.Context, teamRepo string, st *state.State) ([]SkillStatus, error) {
	owner, repo, err := splitRepo(teamRepo)
	if err != nil {
		return nil, err
	}

	raw, err := s.Client.FetchFile(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("fetch loadout: %w", err)
	}

	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse loadout: %w", err)
	}
	if !m.IsLoadout() {
		return nil, fmt.Errorf("%s/scribe.toml has no [team] section", teamRepo)
	}

	var statuses []SkillStatus

	// Skills in the loadout.
	for name, skill := range m.Skills {
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

	return statuses, nil
}

// Run executes the full sync: diff, then install/update as needed.
// Emits events throughout. Updates state incrementally — a failed skill
// does not prevent successful skills from being recorded.
func (s *Syncer) Run(ctx context.Context, teamRepo string, st *state.State) error {
	statuses, err := s.Diff(ctx, teamRepo, st)
	if err != nil {
		return err
	}

	// Emit resolved status for each skill (populates list view before downloads start).
	for _, sk := range statuses {
		s.emit(SkillResolvedMsg{sk})
	}

	summary := SyncCompleteMsg{}

	owner, repo, _ := splitRepo(teamRepo)
	raw, _ := s.Client.FetchFile(ctx, owner, repo, "scribe.toml", "HEAD")
	m, _ := manifest.Parse(raw)

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

			// Write files to canonical store once, then symlink per target.
			canonicalDir, err := targets.WriteToStore(sk.Name, files)
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
				latestSHA, _ = s.Client.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
			}

			st.RecordInstall(sk.Name, state.InstalledSkill{
				Version:   src.Ref,
				CommitSHA: latestSHA,
				Source:    skill.Source,
				Targets:   targetNames,
				Paths:     paths,
			})
			// Save after each successful install — partial sync is safe.
			_ = st.Save()

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
	_ = st.Save()

	s.emit(summary)
	return nil
}

func (s *Syncer) emit(msg any) {
	if s.Emit != nil {
		s.Emit(msg)
	}
}

func splitRepo(teamRepo string) (owner, repo string, err error) {
	parts := strings.SplitN(teamRepo, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo", teamRepo)
	}
	return parts[0], parts[1], nil
}

// loadoutRef extracts the human-readable version ref from a skill entry.
func loadoutRef(skill manifest.Skill) string {
	src, err := manifest.ParseSource(skill.Source)
	if err != nil {
		return "?"
	}
	return src.Ref
}
