package provider

import (
	"context"
	"fmt"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/tools"
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
func (p *GitHubProvider) Discover(ctx context.Context, repo string) ([]manifest.Entry, error) {
	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return nil, err
	}

	// Step 1: Try scribe.yaml.
	entries, err := p.discoverScribeYAML(ctx, owner, repoName)
	if err == nil {
		return entries, nil
	}

	// Step 2: Try scribe.toml (legacy).
	entries, err = p.discoverScribeTOML(ctx, owner, repoName)
	if err == nil {
		p.warn(fmt.Sprintf("%s uses legacy scribe.toml format — consider migrating to scribe.yaml", repo))
		return entries, nil
	}

	// Step 3: Try .claude-plugin/marketplace.json.
	entries, err = p.discoverMarketplace(ctx, owner, repoName)
	if err == nil {
		return entries, nil
	}

	// Step 4: Tree scan for SKILL.md files.
	entries, err = p.discoverTreeScan(ctx, owner, repoName)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}

	return nil, fmt.Errorf("%s: no skills found (looked for scribe.yaml, scribe.toml, marketplace.json, and SKILL.md files)", repo)
}

func (p *GitHubProvider) discoverScribeYAML(ctx context.Context, owner, repo string) ([]manifest.Entry, error) {
	raw, err := p.client.FetchFile(ctx, owner, repo, manifest.ManifestFilename, "HEAD")
	if err != nil {
		return nil, err
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, err
	}
	return m.Catalog, nil
}

func (p *GitHubProvider) discoverScribeTOML(ctx context.Context, owner, repo string) ([]manifest.Entry, error) {
	raw, err := p.client.FetchFile(ctx, owner, repo, manifest.LegacyManifestFilename, "HEAD")
	if err != nil {
		return nil, err
	}
	m, err := migrate.Convert(raw)
	if err != nil {
		return nil, err
	}
	return m.Catalog, nil
}

func (p *GitHubProvider) discoverMarketplace(ctx context.Context, owner, repo string) ([]manifest.Entry, error) {
	raw, err := p.client.FetchFile(ctx, owner, repo, marketplacePath, "HEAD")
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
	return entries, nil
}

func (p *GitHubProvider) discoverTreeScan(ctx context.Context, owner, repo string) ([]manifest.Entry, error) {
	tree, err := p.client.GetTree(ctx, owner, repo, "HEAD")
	if err != nil {
		return nil, err
	}
	entries := ScanTreeForSkills(tree, owner, repo)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no SKILL.md files found in %s/%s", owner, repo)
	}
	return entries, nil
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

	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}

	ghFiles, err := p.client.FetchDirectory(ctx, src.Owner, src.Repo, skillPath, src.Ref)
	if err != nil {
		return nil, err
	}

	files := make([]File, len(ghFiles))
	for i, f := range ghFiles {
		files[i] = File{Path: f.Path, Content: f.Content}
	}
	return files, nil
}
