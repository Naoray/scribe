package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/source"
)

// GitProvider discovers and fetches skills from arbitrary git repositories by
// cloning into a temporary directory per operation.
type GitProvider struct {
	fs *FilesystemProvider
}

func NewGitProvider() *GitProvider {
	return &GitProvider{fs: &FilesystemProvider{sourceName: "git", author: "git"}}
}

func (p *GitProvider) Discover(ctx context.Context, repo string) (*DiscoverResult, error) {
	return p.DiscoverSource(ctx, source.SourceSpec{Type: source.SourceGit, URL: repo})
}

func (p *GitProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]File, error) {
	return nil, fmt.Errorf("git fetch requires SourceSpec URL")
}

func (p *GitProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*DiscoverResult, error) {
	root, sourceID, cleanup, err := p.checkout(ctx, spec)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return p.fs.discoverRoot(ctx, root, sourceID)
}

func (p *GitProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]File, error) {
	root, _, cleanup, err := p.checkout(ctx, spec)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	fullPath, err := safeJoin(root, skillPath)
	if err != nil {
		return nil, err
	}
	return fetchPath(ctx, fullPath)
}

func (p *GitProvider) checkout(ctx context.Context, spec source.SourceSpec) (root string, sourceID string, cleanup func(), err error) {
	spec, err = source.CanonicalSpec(spec)
	if err != nil {
		return "", "", nil, err
	}
	cloneURL, err := gitCloneURL(spec)
	if err != nil {
		return "", "", nil, err
	}
	tmp, err := os.MkdirTemp("", "scribe-source-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("create temp clone dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if err := runGit(ctx, tmp, "clone", "--quiet", cloneURL, "repo"); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("clone %s: %w", cloneURL, err)
	}
	repoDir := tmp + string(os.PathSeparator) + "repo"
	if spec.Ref != "" {
		if err := runGit(ctx, repoDir, "checkout", "--quiet", spec.Ref); err != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("checkout ref %q from %s: %w", spec.Ref, cloneURL, err)
		}
	}
	root = repoDir
	if spec.Path != "" {
		root, err = safeJoin(repoDir, spec.Path)
		if err != nil {
			cleanup()
			return "", "", nil, err
		}
	}
	return root, gitSourceID(spec), cleanup, nil
}

func gitCloneURL(spec source.SourceSpec) (string, error) {
	switch spec.Type {
	case source.SourceGit:
		return spec.URL, nil
	case source.SourceGitLab:
		return spec.URL, nil
	default:
		return "", fmt.Errorf("unsupported source type %q for git provider", spec.Type)
	}
}

func gitSourceID(spec source.SourceSpec) string {
	prefix := "git:"
	locator := spec.URL
	if spec.Type == source.SourceGitLab {
		prefix = "gitlab:"
		locator = spec.Host + "/" + spec.Repo
	}
	id := prefix + locator
	if spec.Ref != "" {
		id += "@" + spec.Ref
	}
	if spec.Path != "" {
		id += ":" + spec.Path
	}
	return id
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}
