package sync

import (
	"context"
	"fmt"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
)

// Re-export so external callers can build their own fetchers without
// importing the provider package.
type TreeEntry = provider.TreeEntry

// WrapGitHubClient returns a GitHubFetcher backed by a real gh.Client.
// Delegates to provider.WrapGitHubClient — provider.GitHubClient is a superset of GitHubFetcher.
func WrapGitHubClient(c *gh.Client) GitHubFetcher {
	return provider.WrapGitHubClient(c)
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

func (n *NoopFetcher) GetTree(_ context.Context, _, _, _ string) ([]provider.TreeEntry, error) {
	return nil, fmt.Errorf("NoopFetcher: GetTree not available (use Provider)")
}
