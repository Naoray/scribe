package sync

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/source"
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

func (p *diffTestProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*provider.DiscoverResult, error) {
	return p.Discover(ctx, spec.Repo)
}

func (p *diffTestProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

func (p *diffTestProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]provider.File, error) {
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

func TestDiffSourceResolvesScopedManifestEntryBlobSHA(t *testing.T) {
	syncer := &Syncer{
		Client: &diffTestFetcher{
			tree: []provider.TreeEntry{
				{Path: "skills/nextjs/SKILL.md", Type: "blob", SHA: "nextjs-blob"},
			},
		},
		Provider: &diffTestProvider{
			entry: manifest.Entry{
				Name:   "nextjs",
				Path:   "nextjs",
				Source: "github:vercel-labs/agent-skills@HEAD",
			},
		},
	}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"nextjs": {
				Sources: []state.SkillSource{{
					Registry: "github:vercel-labs/agent-skills:skills",
					Ref:      "main",
					LastSHA:  "nextjs-blob",
				}},
			},
		},
	}

	statuses, _, err := syncer.DiffSource(
		context.Background(),
		"github:vercel-labs/agent-skills:skills",
		source.SourceSpec{Type: source.SourceGitHub, Repo: "vercel-labs/agent-skills", Ref: "main", Path: "skills"},
		st,
	)
	if err != nil {
		t.Fatalf("DiffSource: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].LatestSHA != "nextjs-blob" {
		t.Fatalf("LatestSHA = %q, want nextjs-blob", statuses[0].LatestSHA)
	}
	if statuses[0].LatestSHA == missingSkillBlobSHA {
		t.Fatalf("LatestSHA used missing sentinel")
	}
	if statuses[0].Status != StatusCurrent {
		t.Fatalf("Status = %s, want %s", statuses[0].Status, StatusCurrent)
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
				Name:   "scribe",
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
