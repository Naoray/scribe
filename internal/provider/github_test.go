package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/source"
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
	result, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if !result.IsTeam {
		t.Error("expected IsTeam=true for scribe.yaml discovery")
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Name != "deploy" {
		t.Errorf("name: got %q", result.Entries[0].Name)
	}
	if result.Entries[0].Author != "alice" {
		t.Errorf("author: got %q", result.Entries[0].Author)
	}
}

func TestDiscoverScribeYAMLFetchesKits(t *testing.T) {
	yamlContent := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test-team
catalog:
  - name: deploy
    source: "github:acme/skills@main"
kits:
  - name: daily-workflow
    path: kits/daily-workflow.yaml
  - name: release-pipeline
    path: kits/release-pipeline.yaml
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.yaml": []byte(yamlContent),
			"acme/team-skills/kits/daily-workflow.yaml": []byte(`apiVersion: scribe/v1
kind: Kit
name: daily-workflow
skills: [deploy]
`),
			"acme/team-skills/kits/release-pipeline.yaml": []byte(`apiVersion: scribe/v1
kind: Kit
name: release-pipeline
skills: [deploy]
`),
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(result.KitErrors) != 0 {
		t.Fatalf("KitErrors = %v, want none", result.KitErrors)
	}
	if len(result.Kits) != 2 {
		t.Fatalf("kits: got %d, want 2", len(result.Kits))
	}
	if result.Kits[0].Name != "daily-workflow" || result.Kits[0].Path != "kits/daily-workflow.yaml" {
		t.Errorf("kit[0] = %+v", result.Kits[0])
	}
	if result.Kits[1].Name != "release-pipeline" || result.Kits[1].Ref != "HEAD" {
		t.Errorf("kit[1] = %+v", result.Kits[1])
	}
}

func TestDiscoverScribeYAMLKitBodySizeCap(t *testing.T) {
	yamlContent := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test-team
catalog: []
kits:
  - name: huge
    path: kits/huge.yaml
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.yaml":    []byte(yamlContent),
			"acme/team-skills/kits/huge.yaml": []byte(strings.Repeat("x", 64*1024+1)),
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.Kits) != 0 {
		t.Fatalf("kits: got %d, want 0", len(result.Kits))
	}
	if len(result.KitErrors) != 1 {
		t.Fatalf("KitErrors: got %d, want 1", len(result.KitErrors))
	}
	if !strings.Contains(result.KitErrors[0].Error(), "exceeds") {
		t.Errorf("KitErrors[0] = %v", result.KitErrors[0])
	}
}

func TestDiscoverScribeYAMLKitCountCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`
apiVersion: scribe/v1
kind: Registry
team:
  name: test-team
catalog: []
kits:
`)
	for i := 0; i < 51; i++ {
		b.WriteString("  - name: kit-")
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString("-")
		b.WriteString(string(rune('a' + i/26)))
		b.WriteString("\n    path: kits/kit.yaml\n")
	}
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.yaml": []byte(b.String()),
		},
	}

	p := provider.NewGitHubProvider(client)
	_, err := p.Discover(context.Background(), "acme/team-skills")
	if err == nil {
		t.Fatal("expected count cap error")
	}
	if !strings.Contains(err.Error(), "maximum is 50") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverScribeYAMLKitPartialErrors(t *testing.T) {
	yamlContent := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test-team
catalog: []
kits:
  - name: good
    path: kits/good.yaml
  - name: missing
    path: kits/missing.yaml
  - name: invalid
    path: kits/invalid.yaml
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/team-skills/scribe.yaml": []byte(yamlContent),
			"acme/team-skills/kits/good.yaml": []byte(`apiVersion: scribe/v1
kind: Kit
name: good
skills: []
`),
			"acme/team-skills/kits/invalid.yaml": []byte(`apiVersion: scribe/v1
kind: Kit
name: ../bad
skills: []
`),
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.Kits) != 1 || result.Kits[0].Name != "good" {
		t.Fatalf("kits = %+v, want good only", result.Kits)
	}
	if len(result.KitErrors) != 2 {
		t.Fatalf("KitErrors: got %d, want 2: %v", len(result.KitErrors), result.KitErrors)
	}
	var typed provider.KitFetchErrors = result.KitErrors
	if typed.Error() == "" {
		t.Fatal("typed error list should render non-empty error")
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

	result, err := p.Discover(context.Background(), "acme/team-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if !result.IsTeam {
		t.Error("expected IsTeam=true for scribe.toml discovery")
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(result.Entries))
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

func TestDiscoverChainOrder(t *testing.T) {
	// When scribe.yaml exists, it takes priority even if marketplace.json also exists.
	yamlContent := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: from-yaml
    source: "github:acme/repo@main"
`
	client := &stubClient{
		files: map[string][]byte{
			"acme/repo/scribe.yaml":                     []byte(yamlContent),
			"acme/repo/.claude-plugin/marketplace.json": []byte(`{"name":"mp","plugins":[]}`),
		},
		treeFiles: []provider.TreeEntry{
			{Path: "skills/from-tree/SKILL.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/repo")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if !result.IsTeam {
		t.Error("expected IsTeam=true for scribe.yaml discovery")
	}
	if len(result.Entries) != 1 || result.Entries[0].Name != "from-yaml" {
		t.Errorf("expected scribe.yaml to win, got %v", result.Entries)
	}
}

func TestDiscoverMarketplaceBeforeTreeScan(t *testing.T) {
	// When no manifest exists but marketplace.json does, use it before tree scan.
	mpJSON := `{
		"name": "test",
		"plugins": [{
			"name": "plug1",
			"source": "./plugins/plug1",
			"skills": ["skills/deploy"]
		}]
	}`
	client := &stubClient{
		files: map[string][]byte{
			"acme/repo/.claude-plugin/marketplace.json": []byte(mpJSON),
		},
		treeFiles: []provider.TreeEntry{
			{Path: "other/SKILL.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/repo")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if result.IsTeam {
		t.Error("expected IsTeam=false for marketplace discovery")
	}
	if len(result.Entries) != 1 || result.Entries[0].Name != "deploy" {
		t.Errorf("expected marketplace entry, got %v", result.Entries)
	}
	if result.Entries[0].Group != "plug1" {
		t.Errorf("expected Group=plug1, got %q", result.Entries[0].Group)
	}
}

func TestDiscoverTreeScanAsLastResort(t *testing.T) {
	// When nothing else works, tree scan kicks in.
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/deploy/SKILL.md", Type: "blob"},
			{Path: "skills/lint/SKILL.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "acme/community-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if result.IsTeam {
		t.Error("expected IsTeam=false for tree scan discovery")
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(result.Entries))
	}

	names := map[string]bool{}
	for _, e := range result.Entries {
		names[e.Name] = true
	}
	if !names["deploy"] || !names["lint"] {
		t.Errorf("expected deploy and lint, got %v", names)
	}
}

func TestDiscoverSourceTreeScopeFiltersAndUsesScopedFetch(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/nextjs/SKILL.md", Type: "blob"},
			{Path: "skills/react/SKILL.md", Type: "blob"},
			{Path: "templates/ignored/SKILL.md", Type: "blob"},
		},
		files: map[string][]byte{
			"vercel-labs/agent-skills/skills/nextjs/SKILL.md": []byte("---\ndescription: Next.js work\n---\n# Next\n"),
			"vercel-labs/agent-skills/skills/react/SKILL.md":  []byte("# React\n"),
		},
	}
	p := provider.NewGitHubProvider(client)

	result, err := p.DiscoverSource(context.Background(), source.SourceSpec{
		Type: source.SourceGitHub,
		Repo: "vercel-labs/agent-skills",
		Ref:  "main",
		Path: "skills",
	})
	if err != nil {
		t.Fatalf("DiscoverSource: %v", err)
	}
	if result.IsTeam {
		t.Fatal("scoped tree scan should not be team registry")
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(result.Entries))
	}
	names := map[string]bool{}
	for _, entry := range result.Entries {
		names[entry.Name] = true
		if entry.Source != "github:vercel-labs/agent-skills@HEAD" {
			t.Fatalf("Source = %q", entry.Source)
		}
		if entry.Path == "templates/ignored" {
			t.Fatal("unscoped tree entry leaked into results")
		}
	}
	if !names["nextjs"] || !names["react"] {
		t.Fatalf("names = %v", names)
	}
}

func TestFetchSourceDoesNotDoublePrefixScopedPath(t *testing.T) {
	client := &stubClient{
		dirs: map[string][]tools.SkillFile{
			"vercel-labs/agent-skills/skills/nextjs": {
				{Path: "SKILL.md", Content: []byte("# Next")},
			},
		},
	}
	p := provider.NewGitHubProvider(client)

	files, err := p.FetchSource(context.Background(),
		source.SourceSpec{Type: source.SourceGitHub, Repo: "vercel-labs/agent-skills", Ref: "main", Path: "skills"},
		manifest.Entry{Name: "nextjs", Source: "github:vercel-labs/agent-skills@HEAD", Path: "skills/nextjs"},
	)
	if err != nil {
		t.Fatalf("FetchSource: %v", err)
	}
	if len(files) != 1 || files[0].Path != "SKILL.md" {
		t.Fatalf("files = %#v", files)
	}
}

func TestFetchSourceResolvesPathRelativeToScope(t *testing.T) {
	client := &stubClient{
		dirs: map[string][]tools.SkillFile{
			"vercel-labs/agent-skills/skills/nextjs": {
				{Path: "SKILL.md", Content: []byte("# Next")},
			},
		},
	}
	p := provider.NewGitHubProvider(client)

	files, err := p.FetchSource(context.Background(),
		source.SourceSpec{Type: source.SourceGitHub, Repo: "vercel-labs/agent-skills", Ref: "main", Path: "skills"},
		manifest.Entry{Name: "nextjs", Source: "github:vercel-labs/agent-skills@HEAD", Path: "nextjs"},
	)
	if err != nil {
		t.Fatalf("FetchSource: %v", err)
	}
	if len(files) != 1 || files[0].Path != "SKILL.md" {
		t.Fatalf("files = %#v", files)
	}
}

func TestDiscoverTreeScanEnrichesSkillFrontmatter(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/nextjs/SKILL.md", Type: "blob"},
		},
		files: map[string][]byte{
			"vercel-labs/agent-skills/skills/nextjs/SKILL.md": []byte(`---
name: next-js
description: Build and debug Next.js applications.
source:
  author: vercel
---
# Next.js
`),
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "https://github.com/vercel-labs/agent-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if result.IsTeam {
		t.Fatal("tree-scan repo should not be a team registry")
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(result.Entries))
	}
	entry := result.Entries[0]
	if entry.Name != "next-js" {
		t.Fatalf("Name = %q, want frontmatter name", entry.Name)
	}
	if entry.Description != "Build and debug Next.js applications." {
		t.Fatalf("Description = %q", entry.Description)
	}
	if entry.Author != "vercel" {
		t.Fatalf("Author = %q", entry.Author)
	}
	if entry.Source != "github:vercel-labs/agent-skills@HEAD" {
		t.Fatalf("Source = %q", entry.Source)
	}
	if entry.Path != "skills/nextjs" {
		t.Fatalf("Path = %q", entry.Path)
	}
}

func TestDiscoverTreeScanWarnsAndKeepsDirectoryNameForBadFrontmatter(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/nextjs/SKILL.md", Type: "blob"},
		},
		files: map[string][]byte{
			"vercel-labs/agent-skills/skills/nextjs/SKILL.md": []byte(`---
name: ../nextjs
description: bad
---
`),
		},
	}

	var warnings []string
	p := provider.NewGitHubProvider(client)
	p.OnWarning = func(msg string) { warnings = append(warnings, msg) }

	result, err := p.Discover(context.Background(), "vercel-labs/agent-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected frontmatter warning")
	}
	if result.Entries[0].Name != "nextjs" {
		t.Fatalf("Name = %q, want directory fallback", result.Entries[0].Name)
	}
}

func TestDiscoverTreeScanAnthropicsFixture(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/algorithmic-art/SKILL.md", Type: "blob"},
			{Path: "skills/brand-guidelines/SKILL.md", Type: "blob"},
			{Path: "skills/webapp-testing/SKILL.md", Type: "blob"},
			{Path: "template/SKILL.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	result, err := p.Discover(context.Background(), "anthropics/skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if result.IsTeam {
		t.Error("expected IsTeam=false for tree-scan discovery")
	}
	if len(result.Entries) != 4 {
		t.Fatalf("entries: got %d, want 4", len(result.Entries))
	}

	names := map[string]bool{}
	for _, e := range result.Entries {
		names[e.Name] = true
	}
	for _, want := range []string{"algorithmic-art", "brand-guidelines", "webapp-testing", "template"} {
		if !names[want] {
			t.Errorf("missing entry %q in %v", want, names)
		}
	}
}
