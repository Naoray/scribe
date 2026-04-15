package sync

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type diffTestFetcher struct {
	tree []provider.TreeEntry
}

func (f *diffTestFetcher) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return nil, nil
}

func (f *diffTestFetcher) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]tools.SkillFile, error) {
	return nil, nil
}

func (f *diffTestFetcher) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	return "", nil
}

func (f *diffTestFetcher) GetTree(ctx context.Context, owner, repo, ref string) ([]provider.TreeEntry, error) {
	return f.tree, nil
}

type diffTestProvider struct {
	entry manifest.Entry
}

func (p *diffTestProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{
		Entries: []manifest.Entry{p.entry},
		IsTeam:  true,
	}, nil
}

func (p *diffTestProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

func TestDiffMarksBranchSkillOutdatedWhenSkillFileMissingFromTree(t *testing.T) {
	syncer := &Syncer{
		Client: &diffTestFetcher{
			tree: []provider.TreeEntry{
				{Path: "README.md", Type: "blob", SHA: "readme"},
			},
		},
		Provider: &diffTestProvider{
			entry: manifest.Entry{
				Name:   "xray",
				Path:   "skills/xray",
				Source: "github:acme/skills@main",
			},
		},
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"xray": {
				Revision: 1,
				Sources: []state.SkillSource{{
					Registry: "acme/team",
					Ref:      "main",
					LastSHA:  "old-blob",
				}},
			},
		},
	}

	statuses, _, err := syncer.Diff(context.Background(), "acme/team", st)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].Status != StatusOutdated {
		t.Fatalf("status = %s, want %s", statuses[0].Status, StatusOutdated)
	}
	if statuses[0].LatestSHA != missingSkillBlobSHA {
		t.Fatalf("latest SHA = %q, want %q", statuses[0].LatestSHA, missingSkillBlobSHA)
	}
}

// nonTeamProvider mirrors what GitHubProvider returns when a repo has skills
// but no scribe.yaml team section — e.g. a root SKILL.md single-skill repo
// or a marketplace.json registry. IsTeam=false used to make Diff abort with
// "has no team section"; it should now succeed because the catalog is
// non-empty.
type nonTeamProvider struct {
	entries []manifest.Entry
}

func (p *nonTeamProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{Entries: p.entries, IsTeam: false}, nil
}

func (p *nonTeamProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

func TestDiffAcceptsNonTeamRegistryWithSkills(t *testing.T) {
	syncer := &Syncer{
		Client: &diffTestFetcher{},
		Provider: &nonTeamProvider{
			entries: []manifest.Entry{{
				Name:   "scribe-agent",
				Path:   "SKILL.md",
				Source: "github:Naoray/scribe@main",
			}},
		},
	}

	statuses, m, err := syncer.Diff(context.Background(), "Naoray/scribe", &state.State{
		Installed: map[string]state.InstalledSkill{},
	})
	if err != nil {
		t.Fatalf("Diff on non-team registry returned error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if m.IsRegistry() {
		t.Fatal("manifest should report IsRegistry()=false for non-team repo")
	}
}

// emptyCatalogProvider returns zero entries — Diff must surface a clear
// "no skills" error rather than silently succeeding.
type emptyCatalogProvider struct{}

func (p *emptyCatalogProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{Entries: nil, IsTeam: false}, nil
}

func (p *emptyCatalogProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

func TestDiffRejectsEmptyCatalog(t *testing.T) {
	syncer := &Syncer{
		Client:   &diffTestFetcher{},
		Provider: &emptyCatalogProvider{},
	}

	_, _, err := syncer.Diff(context.Background(), "acme/empty", &state.State{})
	if err == nil {
		t.Fatal("expected error for empty catalog, got nil")
	}
}
