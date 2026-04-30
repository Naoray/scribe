package registry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

// GitHubPusher is the GitHub API surface needed to push a skill.
type GitHubPusher interface {
	GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error)
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
	PushFilesAtomic(ctx context.Context, owner, repo, branch string, files map[string][]byte, message, expectedHead string) (CommitResult, error)
}

// TreeEntry is a Git tree entry used for conflict checks.
type TreeEntry struct {
	Path string
	Type string
	SHA  string
}

// CommitResult describes the commit created by a push.
type CommitResult struct {
	SHA string
	URL string
}

type githubPusher struct {
	client *gh.Client
}

// NewGitHubPusher adapts the shared GitHub client to the push interface.
func NewGitHubPusher(client *gh.Client) GitHubPusher {
	return githubPusher{client: client}
}

func (p githubPusher) GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error) {
	entries, err := p.client.GetTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, err
	}
	out := make([]TreeEntry, len(entries))
	for i, entry := range entries {
		out[i] = TreeEntry{Path: entry.Path, Type: entry.Type, SHA: entry.SHA}
	}
	return out, nil
}

func (p githubPusher) LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	return p.client.LatestCommitSHA(ctx, owner, repo, branch)
}

func (p githubPusher) PushFilesAtomic(ctx context.Context, owner, repo, branch string, files map[string][]byte, message, expectedHead string) (CommitResult, error) {
	commit, err := p.client.PushFilesAtomic(ctx, owner, repo, branch, files, message, expectedHead)
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{SHA: commit.SHA, URL: commit.URL}, nil
}

// PushResult is returned after a successful skill push.
type PushResult struct {
	Skill     string
	Registry  string
	CommitSHA string
	CommitURL string
}

// PushSkill publishes all files in skillDir back to source in one commit.
func PushSkill(ctx context.Context, client GitHubPusher, skillName, skillDir string, source state.SkillSource) (PushResult, error) {
	registry := source.PushRegistry()
	owner, repo, err := manifest.ParseOwnerRepo(registry)
	if err != nil {
		return PushResult{}, clierrors.Wrap(err, "PUSH_INVALID_REGISTRY", clierrors.ExitConflict,
			clierrors.WithRemediation("reinstall the skill from a valid registry source"),
		)
	}
	branch := source.Ref
	if branch == "" {
		branch = "main"
	}
	remoteDir := strings.Trim(strings.TrimSpace(source.Path), "/")
	if remoteDir == "" {
		remoteDir = skillName
	}

	tree, err := client.GetTree(ctx, owner, repo, branch)
	if err != nil {
		return PushResult{}, err
	}
	headSHA, err := client.LatestCommitSHA(ctx, owner, repo, branch)
	if err != nil {
		return PushResult{}, err
	}
	if source.LastSHA == "" {
		return PushResult{}, clierrors.Wrap(errors.New("skill has no recorded remote baseline"), "PUSH_CONFLICT", clierrors.ExitConflict,
			clierrors.WithRemediation("run `scribe sync` before pushing this skill"),
			clierrors.WithResource(skillName),
		)
	}
	if remoteSHA, found := skillBlobSHA(tree, remoteDir); !found || remoteSHA != source.LastSHA {
		return PushResult{}, clierrors.Wrap(errors.New("remote skill has changed since it was installed"), "PUSH_CONFLICT", clierrors.ExitConflict,
			clierrors.WithRemediation("run `scribe sync` and reapply your local edits before pushing"),
			clierrors.WithResource(skillName),
		)
	}

	files, err := collectSkillFiles(skillDir, remoteDir)
	if err != nil {
		return PushResult{}, err
	}
	commit, err := client.PushFilesAtomic(ctx, owner, repo, branch, files, fmt.Sprintf("Update %s skill", skillName), headSHA)
	if err != nil {
		return PushResult{}, err
	}
	return PushResult{Skill: skillName, Registry: registry, CommitSHA: commit.SHA, CommitURL: commit.URL}, nil
}

func collectSkillFiles(skillDir, remoteDir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(skillDir, func(localPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "versions":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(skillDir, localPath)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return err
		}
		files[path.Join(remoteDir, filepath.ToSlash(rel))] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect skill files: %w", err)
	}
	if len(files) == 0 {
		return nil, clierrors.Wrap(errors.New("skill directory has no files"), "PUSH_EMPTY_SKILL", clierrors.ExitConflict)
	}
	return files, nil
}

func skillBlobSHA(tree []TreeEntry, remoteDir string) (string, bool) {
	target := path.Join(remoteDir, "SKILL.md")
	if remoteDir == "." || remoteDir == "" {
		target = "SKILL.md"
	}
	for _, entry := range tree {
		if entry.Type == "blob" && entry.Path == target {
			return entry.SHA, true
		}
	}
	return "", false
}
