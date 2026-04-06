package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

// ErrRepoExists is returned when CreateRepo encounters a 422 "name already exists" error.
var ErrRepoExists = errors.New("repository already exists")

// Client wraps the go-github client with Scribe-specific helpers.
type Client struct {
	gh            *github.Client
	authenticated bool
}

// IsAuthenticated returns true if the client has a GitHub token.
func (c *Client) IsAuthenticated() bool { return c.authenticated }

// AuthenticatedUser returns the login name of the authenticated GitHub user.
func (c *Client) AuthenticatedUser(ctx context.Context) (string, error) {
	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return "", wrapErr(err, "get authenticated user")
	}
	return user.GetLogin(), nil
}

// NewClient creates a GitHub client using the auth chain:
//  1. gh auth token  (piggyback on gh CLI if installed)
//  2. GITHUB_TOKEN   environment variable
//  3. token argument (pass "" to skip — loaded from config by caller)
//  4. unauthenticated (public repos only)
func NewClient(ctx context.Context, configToken string) *Client {
	token := resolveToken(configToken)

	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(ctx, ts)
	}

	return &Client{gh: github.NewClient(httpClient), authenticated: token != ""}
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

// CreateRepo creates a new GitHub repository under the given owner.
// If owner matches the authenticated user, it creates a personal repo.
// Returns ErrRepoExists if the repo name is already taken.
func (c *Client) CreateRepo(ctx context.Context, owner, name, description string, private bool) (*github.Repository, error) {
	// Check if owner is the authenticated user — personal repos use "" as org.
	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return nil, wrapErr(err, "get authenticated user")
	}

	org := owner
	if strings.EqualFold(owner, user.GetLogin()) {
		org = "" // personal repo
	}

	repo := &github.Repository{
		Name:        github.Ptr(name),
		Description: github.Ptr(description),
		Private:     github.Ptr(private),
	}

	created, _, err := c.gh.Repositories.Create(ctx, org, repo)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusUnprocessableEntity {
			for _, e := range ghErr.Errors {
				if e.Field == "name" && (e.Code == "already_exists" || e.Code == "custom") {
					return nil, ErrRepoExists
				}
			}
		}
		return nil, wrapErr(err, fmt.Sprintf("create repo %s/%s", owner, name))
	}

	return created, nil
}

// PushFiles pushes files to a GitHub repo via the Git Trees API.
// Handles both empty repos (initial commit) and repos with existing content.
func (c *Client) PushFiles(ctx context.Context, owner, repo string, files map[string]string, message string) error {
	// Check for existing HEAD to determine if this is an initial commit.
	var parentSHA, baseTreeSHA string
	ref, _, err := c.gh.Git.GetRef(ctx, owner, repo, "refs/heads/main")
	if err == nil {
		parentSHA = ref.GetObject().GetSHA()
		// Fetch the tree SHA from the parent commit so new files merge with existing ones.
		parentCommit, _, err := c.gh.Git.GetCommit(ctx, owner, repo, parentSHA)
		if err != nil {
			return wrapErr(err, fmt.Sprintf("get commit %s/%s@%s", owner, repo, parentSHA))
		}
		baseTreeSHA = parentCommit.GetTree().GetSHA()
	}

	// Create blobs for each file (sorted for deterministic ordering).
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var entries []*github.TreeEntry
	for _, path := range paths {
		content := files[path]
		blob, _, err := c.gh.Git.CreateBlob(ctx, owner, repo, &github.Blob{
			Content:  github.Ptr(content),
			Encoding: github.Ptr("utf-8"),
		})
		if err != nil {
			return wrapErr(err, fmt.Sprintf("create blob %s", path))
		}
		entries = append(entries, &github.TreeEntry{
			Path: github.Ptr(path),
			Mode: github.Ptr("100644"),
			Type: github.Ptr("blob"),
			SHA:  blob.SHA,
		})
	}

	tree, _, err := c.gh.Git.CreateTree(ctx, owner, repo, baseTreeSHA, entries)
	if err != nil {
		return wrapErr(err, fmt.Sprintf("create tree %s/%s", owner, repo))
	}

	newCommit := &github.Commit{
		Message: github.Ptr(message),
		Tree:    tree,
	}
	if parentSHA != "" {
		newCommit.Parents = []*github.Commit{{SHA: github.Ptr(parentSHA)}}
	}

	commit, _, err := c.gh.Git.CreateCommit(ctx, owner, repo, newCommit, nil)
	if err != nil {
		return wrapErr(err, fmt.Sprintf("create commit %s/%s", owner, repo))
	}

	if parentSHA == "" {
		// Empty repo — create the ref.
		_, _, err = c.gh.Git.CreateRef(ctx, owner, repo, &github.Reference{
			Ref:    github.Ptr("refs/heads/main"),
			Object: &github.GitObject{SHA: commit.SHA},
		})
	} else {
		// Existing repo — update the ref.
		_, _, err = c.gh.Git.UpdateRef(ctx, owner, repo, &github.Reference{
			Ref:    github.Ptr("refs/heads/main"),
			Object: &github.GitObject{SHA: commit.SHA},
		}, false)
	}
	if err != nil {
		return wrapErr(err, fmt.Sprintf("set ref %s/%s", owner, repo))
	}

	return nil
}

// FileExists checks whether a file exists in a GitHub repo at the given ref.
func (c *Client) FileExists(ctx context.Context, owner, repo, path, ref string) (bool, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}
	_, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, wrapErr(err, fmt.Sprintf("%s/%s/%s@%s", owner, repo, path, ref))
	}
	return true, nil
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
			return fmt.Errorf("%s: not found (check the repo/path exists and you have access): %w", context, err)
		case http.StatusUnauthorized, http.StatusForbidden:
			if ghErr.Response.Header.Get("X-RateLimit-Remaining") == "0" {
				reset := ghErr.Response.Header.Get("X-RateLimit-Reset")
				return fmt.Errorf("%s: rate limit exceeded, resets at %s — set GITHUB_TOKEN for higher limits: %w", context, formatReset(reset), err)
			}
			return fmt.Errorf("%s: authentication required — run `gh auth login` or set GITHUB_TOKEN: %w", context, err)
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
