package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

// Client wraps the go-github client with Scribe-specific helpers.
type Client struct {
	gh *github.Client
}

// NewClient creates a GitHub client using the auth chain:
//  1. gh auth token  (piggyback on gh CLI if installed)
//  2. GITHUB_TOKEN   environment variable
//  3. token argument (pass "" to skip — loaded from config by caller)
//  4. unauthenticated (public repos only)
func NewClient(configToken string) *Client {
	token := resolveToken(configToken)

	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	}

	return &Client{gh: github.NewClient(httpClient)}
}

func resolveToken(configToken string) string {
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return token
		}
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return configToken
}

// FetchFile fetches a single file's content from a GitHub repo.
func (c *Client) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}
	file, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("%s/%s/%s@%s", owner, repo, path, ref))
	}
	data, err := file.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode %s/%s/%s: %w", owner, repo, path, err)
	}
	return []byte(data), nil
}

// SkillFile is a single file within a downloaded skill directory.
type SkillFile struct {
	Path    string // relative to the skill root (e.g. "scripts/deploy.sh")
	Content []byte
}

// FetchDirectory downloads all files under dirPath at the given ref.
// Uses the Git Trees API (recursive) to enumerate files, then fetches each one.
func (c *Client) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]SkillFile, error) {
	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, ref, true)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("%s/%s tree@%s", owner, repo, ref))
	}

	prefix := strings.TrimSuffix(dirPath, "/") + "/"
	var files []SkillFile

	for _, entry := range tree.Entries {
		path := entry.GetPath()
		if !strings.HasPrefix(path, prefix) || entry.GetType() != "blob" {
			continue
		}
		content, err := c.FetchFile(ctx, owner, repo, path, ref)
		if err != nil {
			return nil, err
		}
		files = append(files, SkillFile{
			Path:    strings.TrimPrefix(path, prefix),
			Content: content,
		})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("%s/%s: no files found under %q at %s", owner, repo, dirPath, ref)
	}
	return files, nil
}

// LatestCommitSHA returns the current HEAD SHA for a branch.
// Used to detect updates for branch-pinned skills (e.g. @main).
func (c *Client) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	ref, _, err := c.gh.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return "", wrapErr(err, fmt.Sprintf("%s/%s branch %s", owner, repo, branch))
	}
	return ref.GetObject().GetSHA(), nil
}

// wrapErr produces user-friendly errors for common GitHub API failures.
func wrapErr(err error, context string) error {
	if err == nil {
		return nil
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		switch ghErr.Response.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%s: not found (check the repo/path exists and you have access)", context)
		case http.StatusUnauthorized, http.StatusForbidden:
			if ghErr.Response.Header.Get("X-RateLimit-Remaining") == "0" {
				reset := ghErr.Response.Header.Get("X-RateLimit-Reset")
				return fmt.Errorf("%s: rate limit exceeded, resets at %s — set GITHUB_TOKEN for higher limits", context, formatReset(reset))
			}
			return fmt.Errorf("%s: authentication required — run `gh auth login` or set GITHUB_TOKEN", context)
		}
	}
	return fmt.Errorf("%s: %w", context, err)
}

func formatReset(unix string) string {
	var sec int64
	if _, err := fmt.Sscan(unix, &sec); err != nil {
		return unix
	}
	return time.Unix(sec, 0).Format("15:04:05")
}
