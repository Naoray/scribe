package provider

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/tools"
)

const (
	maxKitsPerManifest = 50
	maxKitBodyBytes    = 64 * 1024
)

// TreeEntry mirrors github.TreeEntry so the provider package doesn't import github directly.
type TreeEntry struct {
	Path string
	Type string // "blob" or "tree"
	SHA  string
}

// GitHubClient abstracts the GitHub API operations needed by the provider.
type GitHubClient interface {
	FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]tools.SkillFile, error)
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
	GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error)
	HasPushAccess(ctx context.Context, owner, repo string) (bool, error)
}

// GitHubProvider discovers and fetches skills from GitHub repositories.
type GitHubProvider struct {
	client    GitHubClient
	OnWarning func(msg string) // optional callback for non-fatal warnings
}

// NewGitHubProvider creates a GitHubProvider wrapping the given client.
func NewGitHubProvider(client GitHubClient) *GitHubProvider {
	return &GitHubProvider{client: client}
}

func (p *GitHubProvider) warn(msg string) {
	if p.OnWarning != nil {
		p.OnWarning(msg)
	}
}

// Discover probes the repo using a fallback chain and returns all discovered entries.
func (p *GitHubProvider) Discover(ctx context.Context, repo string) (*DiscoverResult, error) {
	spec, err := source.ParseSourceArg(repo)
	if err != nil {
		return nil, err
	}
	return p.DiscoverSource(ctx, spec)
}

// DiscoverSource probes a GitHub source using the same fallback chain as
// Discover, scoped to SourceSpec.Path when configured.
func (p *GitHubProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*DiscoverResult, error) {
	spec, err := canonicalGitHubSource(spec)
	if err != nil {
		return nil, err
	}
	owner, repoName, err := manifest.ParseOwnerRepo(spec.Repo)
	if err != nil {
		return nil, err
	}
	ref := sourceRef(spec)

	// Step 1: Try scribe.yaml.
	raw, err := p.client.FetchFile(ctx, owner, repoName, ScopedPath(spec.Path, manifest.ManifestFilename), ref)
	if err == nil {
		return p.parseScribeYAML(ctx, owner, repoName, spec.Path, ref, raw)
	}

	// Step 2: Try scribe.toml (legacy).
	m, err := p.discoverScribeTOML(ctx, owner, repoName, spec.Path, ref)
	if err == nil {
		p.warn(fmt.Sprintf("%s uses legacy scribe.toml format — consider migrating to scribe.yaml", spec.Repo))
		return &DiscoverResult{Entries: m.Catalog, IsTeam: true, Manifest: m}, nil
	}

	// Step 3: Try .claude-plugin/marketplace.json.
	entries, err := p.discoverMarketplace(ctx, owner, repoName, spec.Path, ref)
	if err == nil {
		return &DiscoverResult{Entries: entries, IsTeam: false}, nil
	}

	// Step 4: Tree scan for SKILL.md files.
	entries, err = p.discoverTreeScan(ctx, owner, repoName, spec.Path, ref)
	if err == nil && len(entries) > 0 {
		return &DiscoverResult{Entries: entries, IsTeam: false}, nil
	}

	return nil, fmt.Errorf("%s: no skills found (looked for scribe.yaml, scribe.toml, marketplace.json, and SKILL.md files)", spec.Repo)
}

func (p *GitHubProvider) parseScribeYAML(ctx context.Context, owner, repo, scope, ref string, raw []byte) (*DiscoverResult, error) {
	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, err
	}
	kits, kitErrors, err := p.fetchKits(ctx, owner, repo, scope, ref, m.Kits)
	if err != nil {
		return nil, err
	}
	return &DiscoverResult{
		Entries:   m.Catalog,
		Kits:      kits,
		KitErrors: kitErrors,
		IsTeam:    true,
		Manifest:  m,
	}, nil
}

func (p *GitHubProvider) discoverScribeTOML(ctx context.Context, owner, repo, scope, ref string) (*manifest.Manifest, error) {
	raw, err := p.client.FetchFile(ctx, owner, repo, ScopedPath(scope, manifest.LegacyManifestFilename), ref)
	if err != nil {
		return nil, err
	}
	m, err := migrate.Convert(raw)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (p *GitHubProvider) discoverMarketplace(ctx context.Context, owner, repo, scope, ref string) ([]manifest.Entry, error) {
	raw, err := p.client.FetchFile(ctx, owner, repo, ScopedPath(scope, marketplacePath), ref)
	if err != nil {
		return nil, err
	}
	entries, err := ParseMarketplace(raw, owner, repo)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("marketplace.json has no skills")
	}
	if scope != "" {
		for i := range entries {
			entries[i].Path = ScopedPath(scope, entries[i].Path)
		}
	}
	return entries, nil
}

func (p *GitHubProvider) discoverTreeScan(ctx context.Context, owner, repo, scope, ref string) ([]manifest.Entry, error) {
	tree, err := p.client.GetTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, err
	}
	tree = filterTreeToScope(tree, scope)
	entries := ScanTreeForSkills(tree, owner, repo)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no SKILL.md files found in %s/%s", owner, repo)
	}

	for i := range entries {
		skillPath := entries[i].Path
		if path.Base(skillPath) != skillFileName {
			skillPath = path.Join(skillPath, skillFileName)
		}
		data, err := p.client.FetchFile(ctx, owner, repo, skillPath, ref)
		if err != nil {
			p.warn(fmt.Sprintf("%s/%s: could not read %s frontmatter: %v", owner, repo, skillPath, err))
			continue
		}
		enriched, err := EnrichTreeSkillEntry(entries[i], data)
		if err != nil {
			p.warn(fmt.Sprintf("%s/%s: invalid %s frontmatter: %v", owner, repo, skillPath, err))
			continue
		}
		entries[i] = enriched
	}
	return entries, nil
}

func (p *GitHubProvider) fetchKits(ctx context.Context, owner, repo, scope, ref string, refs []manifest.KitEntry) ([]KitFile, KitFetchErrors, error) {
	if len(refs) == 0 {
		return nil, nil, nil
	}
	if len(refs) > maxKitsPerManifest {
		return nil, nil, fmt.Errorf("manifest declares %d kits; maximum is %d", len(refs), maxKitsPerManifest)
	}

	type result struct {
		index int
		file  KitFile
		err   KitFetchError
		ok    bool
	}

	sem := make(chan struct{}, 4)
	results := make(chan result, len(refs))
	var wg sync.WaitGroup
	for i, kitRef := range refs {
		i, kitRef := i, kitRef
		wg.Add(1)
		go func() {
			defer wg.Done()
			kitPath := kitRef.PathOrDefault()
			fetchPath := ScopedPath(scope, kitPath)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: i, err: KitFetchError{Name: kitRef.Name, Path: fetchPath, Err: ctx.Err()}}
				return
			}

			body, err := p.client.FetchFile(ctx, owner, repo, fetchPath, ref)
			if err != nil {
				results <- result{index: i, err: KitFetchError{Name: kitRef.Name, Path: fetchPath, Err: err}}
				return
			}
			if len(body) > maxKitBodyBytes {
				results <- result{index: i, err: KitFetchError{
					Name: kitRef.Name,
					Path: fetchPath,
					Err:  fmt.Errorf("kit body exceeds %d bytes", maxKitBodyBytes),
				}}
				return
			}
			if _, err := kit.ParseYAML(body); err != nil {
				results <- result{index: i, err: KitFetchError{Name: kitRef.Name, Path: fetchPath, Err: err}}
				return
			}
			results <- result{
				index: i,
				file: KitFile{
					Name: kitRef.Name,
					Path: fetchPath,
					Body: body,
					Ref:  ref,
				},
				ok: true,
			}
		}()
	}
	wg.Wait()
	close(results)

	byIndex := make(map[int]KitFile, len(refs))
	errs := make(KitFetchErrors, 0)
	for r := range results {
		if r.ok {
			byIndex[r.index] = r.file
			continue
		}
		errs = append(errs, r.err)
	}
	kits := make([]KitFile, 0, len(byIndex))
	for i := range refs {
		if file, ok := byIndex[i]; ok {
			kits = append(kits, file)
		}
	}
	return kits, errs, nil
}

// clientAdapter adapts *gh.Client to GitHubClient interface.
type clientAdapter struct {
	client *gh.Client
}

// WrapGitHubClient returns a GitHubClient backed by a real github.Client.
func WrapGitHubClient(c *gh.Client) GitHubClient {
	return &clientAdapter{client: c}
}

func (a *clientAdapter) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return a.client.FetchFile(ctx, owner, repo, path, ref)
}

func (a *clientAdapter) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]tools.SkillFile, error) {
	ghFiles, err := a.client.FetchDirectory(ctx, owner, repo, dirPath, ref)
	if err != nil {
		return nil, err
	}
	files := make([]tools.SkillFile, len(ghFiles))
	for i, f := range ghFiles {
		files[i] = tools.SkillFile{Path: f.Path, Content: f.Content}
	}
	return files, nil
}

func (a *clientAdapter) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	return a.client.LatestCommitSHA(ctx, owner, repo, branch)
}

func (a *clientAdapter) GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error) {
	ghEntries, err := a.client.GetTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, err
	}
	entries := make([]TreeEntry, len(ghEntries))
	for i, e := range ghEntries {
		entries[i] = TreeEntry{Path: e.Path, Type: e.Type, SHA: e.SHA}
	}
	return entries, nil
}

func (a *clientAdapter) HasPushAccess(ctx context.Context, owner, repo string) (bool, error) {
	return a.client.HasPushAccess(ctx, owner, repo)
}

// Fetch downloads all files for a catalog entry from the source repo.
func (p *GitHubProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]File, error) {
	src, err := manifest.ParseSource(entry.Source)
	if err != nil {
		return nil, fmt.Errorf("parse source for %s: %w", entry.Name, err)
	}
	spec := source.SourceSpec{Type: source.SourceGitHub, Repo: src.Owner + "/" + src.Repo, Ref: src.Ref}
	return p.FetchSource(ctx, spec, entry)
}

// FetchSource downloads an entry from a GitHub source, resolving paths relative
// to the source scope unless the entry already includes that scope.
func (p *GitHubProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]File, error) {
	spec, err := canonicalGitHubSource(spec)
	if err != nil {
		return nil, err
	}
	owner, repoName, err := manifest.ParseOwnerRepo(spec.Repo)
	if err != nil {
		return nil, err
	}
	ref := sourceRef(spec)

	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	skillPath = ScopedPath(spec.Path, skillPath)

	if path.Base(skillPath) == skillFileName {
		data, err := p.client.FetchFile(ctx, owner, repoName, skillPath, ref)
		if err != nil {
			return nil, err
		}
		return []File{{Path: skillFileName, Content: data}}, nil
	}

	ghFiles, err := p.client.FetchDirectory(ctx, owner, repoName, skillPath, ref)
	if err != nil {
		return nil, err
	}

	files := make([]File, len(ghFiles))
	for i, f := range ghFiles {
		files[i] = File{Path: f.Path, Content: f.Content}
	}
	return files, nil
}

func canonicalGitHubSource(spec source.SourceSpec) (source.SourceSpec, error) {
	spec, err := source.CanonicalSpec(spec)
	if err != nil {
		return source.SourceSpec{}, err
	}
	if spec.Type != source.SourceGitHub {
		return source.SourceSpec{}, fmt.Errorf("unsupported source type %q for GitHub provider", spec.Type)
	}
	return spec, nil
}

func sourceRef(spec source.SourceSpec) string {
	if spec.Ref != "" {
		return spec.Ref
	}
	return "HEAD"
}

// ScopedPath resolves rel under scope without double-prefixing paths that
// already include the scope.
func ScopedPath(scope, rel string) string {
	scope = strings.Trim(path.Clean(strings.TrimSpace(scope)), "/")
	rel = strings.Trim(path.Clean(strings.TrimSpace(rel)), "/")
	if scope == "." {
		scope = ""
	}
	if rel == "." {
		rel = ""
	}
	if scope == "" {
		return rel
	}
	if rel == "" || rel == scope || strings.HasPrefix(rel, scope+"/") {
		return rel
	}
	return path.Join(scope, rel)
}

func filterTreeToScope(tree []TreeEntry, scope string) []TreeEntry {
	scope = strings.Trim(path.Clean(strings.TrimSpace(scope)), "/")
	if scope == "" || scope == "." {
		return tree
	}
	filtered := make([]TreeEntry, 0, len(tree))
	for _, entry := range tree {
		if entry.Path == scope || strings.HasPrefix(entry.Path, scope+"/") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
