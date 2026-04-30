package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
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

// NewClient creates a GitHub client using the auth chain:
//  1. gh auth token  (piggyback on gh CLI if installed)
//  2. GITHUB_TOKEN   environment variable
//  3. token argument (pass "" to skip — loaded from config by caller)
//  4. unauthenticated (public repos only)
func NewClient(ctx context.Context, configToken string) *Client {
	token := resolveToken(configToken)

	httpClient := newHTTPClient(ctx, token)

	return &Client{gh: github.NewClient(httpClient), authenticated: token != ""}
}

func resolveToken(configToken string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output(); err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return token
		}
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	if token := strings.TrimSpace(configToken); token != "" {
		return token
	}

	return ""
}

func newHTTPClient(ctx context.Context, token string) *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       25,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if token == "" {
		return &http.Client{Transport: transport}
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &http.Client{
		Transport: &oauth2.Transport{
			Source: ts,
			Base:   transport,
		},
	}
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
		if content == "" {
			// Deletion: tree entry with nil SHA removes the file.
			entries = append(entries, &github.TreeEntry{
				Path: github.Ptr(path),
				Mode: github.Ptr("100644"),
				Type: github.Ptr("blob"),
				// SHA intentionally nil — signals deletion to GitHub API.
			})
			continue
		}
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

// TreeEntry represents a single entry from a recursive Git tree listing.
type TreeEntry struct {
	Path string // full path from repo root (e.g. "skills/deploy/SKILL.md")
	Type string // "blob" or "tree"
	SHA  string
}

// GetTree returns a recursive tree listing for the given ref.
// Uses the GitHub Trees API with recursive=true for a single API call.
func (c *Client) GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error) {
	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, ref, true)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("%s/%s tree@%s", owner, repo, ref))
	}

	entries := make([]TreeEntry, 0, len(tree.Entries))
	for _, e := range tree.Entries {
		entries = append(entries, TreeEntry{
			Path: e.GetPath(),
			Type: e.GetType(),
			SHA:  e.GetSHA(),
		})
	}
	return entries, nil
}

// HasPushAccess checks whether the authenticated user has push (write) access
// to the given repository. Returns false if unauthenticated or access denied.
func (c *Client) HasPushAccess(ctx context.Context, owner, repo string) (bool, error) {
	if !c.authenticated {
		return false, nil
	}
	r, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return false, wrapErr(err, fmt.Sprintf("check access %s/%s", owner, repo))
	}
	perms := r.GetPermissions()
	return perms["push"] || perms["admin"], nil
}

// LatestRelease fetches the latest published release for a repository.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, error) {
	release, _, err := c.gh.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("latest release %s/%s", owner, repo))
	}
	return release, nil
}

// DownloadReleaseAsset downloads a release asset by ID, following redirects.
func (c *Client) DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, error) {
	rc, redirectURL, err := c.gh.Repositories.DownloadReleaseAsset(ctx, owner, repo, id, http.DefaultClient)
	if err != nil {
		return nil, wrapErr(err, fmt.Sprintf("download asset %d from %s/%s", id, owner, repo))
	}
	if redirectURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirectURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build redirect request for asset %d: %w", id, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("follow redirect for asset %d: %w", id, err)
		}
		return resp.Body, nil
	}
	return rc, nil
}

// wrapErr produces user-friendly errors for common GitHub API failures.
func wrapErr(err error, operation string) error {
	if err == nil {
		return nil
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		switch ghErr.Response.StatusCode {
		case http.StatusNotFound:
			return clierrors.Wrap(err, "GH_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithMessage(fmt.Sprintf("%s: not found (check the repo/path exists and you have access)", operation)),
				clierrors.WithResource(operation),
			)
		case http.StatusUnauthorized, http.StatusForbidden:
			if ghErr.Response.Header.Get("X-RateLimit-Remaining") == "0" {
				reset := ghErr.Response.Header.Get("X-RateLimit-Reset")
				return clierrors.Wrap(err, "GH_RATE_LIMITED", clierrors.ExitNetwork,
					clierrors.WithMessage(fmt.Sprintf("%s: rate limit exceeded, resets at %s", operation, formatReset(reset))),
					clierrors.WithRetryable(true),
					clierrors.WithRemediation("set GITHUB_TOKEN for higher limits"),
					clierrors.WithResource(operation),
				)
			}
			return clierrors.Wrap(err, "GH_AUTH_FAILED", clierrors.ExitPerm,
				clierrors.WithMessage(fmt.Sprintf("%s: authentication required", operation)),
				clierrors.WithRemediation("run `gh auth login` or set GITHUB_TOKEN"),
				clierrors.WithResource(operation),
			)
		}
	}
	return clierrors.Wrap(err, "GH_NETWORK_FAILED", clierrors.ExitNetwork,
		clierrors.WithMessage(fmt.Sprintf("%s: %v", operation, err)),
		clierrors.WithRetryable(true),
		clierrors.WithResource(operation),
	)
}

func formatReset(unixTimestamp string) string {
	var sec int64
	if _, err := fmt.Sscan(unixTimestamp, &sec); err != nil {
		return unixTimestamp
	}
	return time.Unix(sec, 0).Format("15:04:05")
}
