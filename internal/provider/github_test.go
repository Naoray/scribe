package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/tools"
)

// stubClient implements the provider.GitHubClient interface for testing.
type stubClient struct {
	files      map[string][]byte            // key: "owner/repo/path"
	dirs       map[string][]tools.SkillFile // key: "owner/repo/dirPath"
	treeFiles  []provider.TreeEntry
	pushAccess bool
}

func (s *stubClient) FetchFile(_ context.Context, owner, repo, path, ref string) ([]byte, error) {
	key := owner + "/" + repo + "/" + path
	if data, ok := s.files[key]; ok {
		return data, nil
	}
	return nil, errors.New("not found: " + key)
}

func (s *stubClient) FetchDirectory(_ context.Context, owner, repo, dirPath, ref string) ([]tools.SkillFile, error) {
	key := owner + "/" + repo + "/" + dirPath
	if files, ok := s.dirs[key]; ok {
		return files, nil
	}
	return nil, errors.New("not found: " + key)
}

func (s *stubClient) LatestCommitSHA(_ context.Context, owner, repo, branch string) (string, error) {
	return "abc1234", nil
}

func (s *stubClient) GetTree(_ context.Context, owner, repo, ref string) ([]provider.TreeEntry, error) {
	return s.treeFiles, nil
}

func (s *stubClient) HasPushAccess(_ context.Context, owner, repo string) (bool, error) {
	return s.pushAccess, nil
}

func TestDiscoverScribeYAML(t *testing.T) {
	yamlContent := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test-team
catalog:
  - name: deploy
    source: "github:acme/skills@main"
    path: skills/deploy
    author: alice
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.yaml": []byte(yamlContent),
		},
	}

	p := provider.NewGitHubProvider(client)
	entries, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if entries[0].Name != "deploy" {
		t.Errorf("name: got %q", entries[0].Name)
	}
	if entries[0].Author != "alice" {
		t.Errorf("author: got %q", entries[0].Author)
	}
}

func TestDiscoverFallbackToTOML(t *testing.T) {
	tomlContent := `
[team]
name = "legacy-team"

[skills.deploy]
source = "github:acme/skills@v1.0.0"
path = "skills/deploy"
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.toml": []byte(tomlContent),
		},
	}

	var warnings []string
	p := provider.NewGitHubProvider(client)
	p.OnWarning = func(msg string) { warnings = append(warnings, msg) }

	entries, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(entries))
	}
	if len(warnings) == 0 {
		t.Error("expected legacy warning, got none")
	}
}

func TestFetchDelegatesToFetchDirectory(t *testing.T) {
	client := &stubClient{
		dirs: map[string][]tools.SkillFile{
			"acme/skills/skills/deploy": {
				{Path: "SKILL.md", Content: []byte("# Deploy")},
			},
		},
	}

	p := provider.NewGitHubProvider(client)
	entry := manifest.Entry{
		Name:   "deploy",
		Source: "github:acme/skills@main",
		Path:   "skills/deploy",
	}

	files, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files: got %d, want 1", len(files))
	}
	if files[0].Path != "SKILL.md" {
		t.Errorf("path: got %q", files[0].Path)
	}
}

func TestDiscoverNothingFoundReturnsError(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "README.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	_, err := p.Discover(context.Background(), "acme/empty-repo")
	if err == nil {
		t.Fatal("expected error for repo with no skills")
	}
}

func TestGitHubProviderSatisfiesInterface(t *testing.T) {
	client := &stubClient{}
	p := provider.NewGitHubProvider(client)
	var _ provider.Provider = p
}
