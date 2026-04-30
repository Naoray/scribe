package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	isync "github.com/Naoray/scribe/internal/sync"
)

type lockPlanOutput struct {
	Registries []registryLockPlan `json:"registries"`
	Updates    []lockfile.Update  `json:"updates"`
}

type registryLockPlan struct {
	Registry string            `json:"registry"`
	Updates  []lockfile.Update `json:"updates"`
}

func buildLockPlan(ctx context.Context, repos []string, fetcher isync.GitHubFetcher, p provider.Provider, st *state.State) (lockPlanOutput, map[string]*lockfile.Lockfile, error) {
	out := lockPlanOutput{Registries: []registryLockPlan{}, Updates: []lockfile.Update{}}
	next := map[string]*lockfile.Lockfile{}
	syncer := &isync.Syncer{Client: fetcher, Provider: p}
	for _, repo := range repos {
		current, err := syncer.FetchLockfile(ctx, repo)
		if err != nil {
			return out, nil, err
		}
		latest, err := buildLatestLockfile(ctx, repo, fetcher, p)
		if err != nil {
			return out, nil, err
		}
		next[repo] = latest
		updates := lockfile.Diff(current, latest)
		out.Registries = append(out.Registries, registryLockPlan{Registry: repo, Updates: updates})
		out.Updates = append(out.Updates, updates...)
		_ = st
	}
	return out, next, nil
}

func buildLatestLockfile(ctx context.Context, repo string, fetcher isync.GitHubFetcher, p provider.Provider) (*lockfile.Lockfile, error) {
	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return nil, err
	}
	syncer := &isync.Syncer{Client: fetcher, Provider: p}
	m, err := syncer.FetchManifest(ctx, owner, repoName)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest for %s: %w", repo, err)
	}
	lf := &lockfile.Lockfile{FormatVersion: lockfile.SchemaVersion, Registry: repo, Entries: []lockfile.Entry{}}
	for _, entry := range m.Catalog {
		pin, err := buildLatestLockEntry(ctx, entry, fetcher, p)
		if err != nil {
			return nil, err
		}
		lf.Entries = append(lf.Entries, pin)
	}
	sort.Slice(lf.Entries, func(i, j int) bool { return lf.Entries[i].Name < lf.Entries[j].Name })
	return lf, nil
}

func buildLatestLockEntry(ctx context.Context, entry manifest.Entry, fetcher isync.GitHubFetcher, p provider.Provider) (lockfile.Entry, error) {
	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return lockfile.Entry{}, err
	}
	commit := src.Ref
	if src.IsBranch() {
		commit, err = fetcher.LatestCommitSHA(ctx, src.Owner, src.Repo, src.Ref)
		if err != nil {
			return lockfile.Entry{}, err
		}
	}
	pinned := entry
	src.Ref = commit
	pinned.Source = src.String()
	files, err := p.Fetch(ctx, pinned)
	if err != nil {
		return lockfile.Entry{}, err
	}
	hash, err := isync.HashInstallableFiles(files)
	if err != nil {
		return lockfile.Entry{}, err
	}
	return lockfile.Entry{
		Name:               entry.Name,
		SourceRegistry:     src.Owner + "/" + src.Repo,
		CommitSHA:          commit,
		ContentHash:        hash,
		InstallCommandHash: installCommandHash(entry),
	}, nil
}

func installCommandHash(entry manifest.Entry) string {
	parts := []string{entry.Install, entry.Update}
	for _, key := range sortedMapKeys(entry.Installs) {
		parts = append(parts, key, entry.Installs[key])
	}
	for _, key := range sortedMapKeys(entry.Updates) {
		parts = append(parts, key, entry.Updates[key])
	}
	return lockfile.CommandHash(parts...)
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
