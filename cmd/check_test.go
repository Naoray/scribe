package cmd

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type lockPlanFetcher struct {
	lock []byte
}

func (f lockPlanFetcher) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	if path == lockfile.Filename {
		return f.lock, nil
	}
	if path == manifest.ManifestFilename {
		return []byte(`apiVersion: scribe/v1
kind: Registry
team:
  name: acme/registry
catalog:
  - name: deploy
    source: github:acme/source@main
`), nil
	}
	return nil, nil
}

func (f lockPlanFetcher) FetchDirectory(context.Context, string, string, string, string) ([]tools.SkillFile, error) {
	return []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# deploy\n")}}, nil
}

func (f lockPlanFetcher) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "new-sha", nil
}

func (f lockPlanFetcher) GetTree(context.Context, string, string, string) ([]provider.TreeEntry, error) {
	return nil, nil
}

type lockPlanProvider struct {
	fetcher lockPlanFetcher
}

func (p lockPlanProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{IsTeam: true, Entries: []manifest.Entry{{
		Name:   "deploy",
		Source: "github:acme/source@main",
	}}}, nil
}

func (p lockPlanProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]provider.File, error) {
	return []provider.File{{Path: "SKILL.md", Content: []byte("# deploy\n")}}, nil
}

func TestBuildLockPlanReportsAvailableUpdates(t *testing.T) {
	current := []byte(`format_version: 1
registry: acme/registry
entries:
  - name: deploy
    source_registry: acme/source
    commit_sha: old-sha
    content_hash: old-hash
`)
	fetcher := lockPlanFetcher{lock: current}
	out, latest, err := buildLockPlan(context.Background(), []string{"acme/registry"}, fetcher, lockPlanProvider{fetcher: fetcher}, &state.State{})
	if err != nil {
		t.Fatalf("buildLockPlan() error = %v", err)
	}
	if len(out.Updates) != 1 {
		t.Fatalf("len(updates) = %d, want 1: %+v", len(out.Updates), out)
	}
	if out.Updates[0].CurrentSHA != "old-sha" || out.Updates[0].LatestSHA != "new-sha" {
		t.Fatalf("unexpected update: %+v", out.Updates[0])
	}
	if latest["acme/registry"].Entries[0].CommitSHA != "new-sha" {
		t.Fatalf("latest lock not pinned to new-sha: %+v", latest["acme/registry"])
	}
}
