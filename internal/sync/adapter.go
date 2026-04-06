package sync

import (
	"context"
	"fmt"

	gh "github.com/Naoray/scribe/internal/github"
)

// ghAdapter wraps *gh.Client to satisfy the GitHubFetcher interface,
// converting []github.SkillFile → []sync.SkillFile.
type ghAdapter struct {
	client *gh.Client
}

// WrapGitHubClient returns a GitHubFetcher backed by a real gh.Client.
func WrapGitHubClient(c *gh.Client) GitHubFetcher {
	return &ghAdapter{client: c}
}

func (a *ghAdapter) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return a.client.FetchFile(ctx, owner, repo, path, ref)
}

func (a *ghAdapter) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]SkillFile, error) {
	ghFiles, err := a.client.FetchDirectory(ctx, owner, repo, dirPath, ref)
	if err != nil {
		return nil, err
	}
	files := make([]SkillFile, len(ghFiles))
	for i, f := range ghFiles {
		files[i] = SkillFile{Path: f.Path, Content: f.Content}
	}
	return files, nil
}

func (a *ghAdapter) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	return a.client.LatestCommitSHA(ctx, owner, repo, branch)
}

// NoopFetcher is a GitHubFetcher that returns errors for all operations.
// Used when Provider handles all fetching.
type NoopFetcher struct{}

func (n *NoopFetcher) FetchFile(_ context.Context, _, _, _, _ string) ([]byte, error) {
	return nil, fmt.Errorf("NoopFetcher: FetchFile not available (use Provider)")
}

func (n *NoopFetcher) FetchDirectory(_ context.Context, _, _, _, _ string) ([]SkillFile, error) {
	return nil, fmt.Errorf("NoopFetcher: FetchDirectory not available (use Provider)")
}

func (n *NoopFetcher) LatestCommitSHA(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("NoopFetcher: LatestCommitSHA not available (use Provider)")
}
