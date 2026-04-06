# Provider & Community Registries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Provider abstraction that decouples "how to discover & fetch skills from a repo" from the sync engine. Support community registries (repos without `[team]`), marketplace.json plugin format, and automatic SKILL.md tree scanning. Add built-in registries, first-run setup, and registry enable/disable.

**Architecture:** A new `internal/provider/` package defines the `Provider` interface with `Discover()` and `Fetch()`. `GitHubProvider` implements a four-step discovery chain: scribe.yaml, scribe.toml (legacy), marketplace.json, SKILL.md tree scan. The connect command drops its `[team]` requirement and uses Provider.Discover(). The sync engine delegates fetching to Provider instead of calling GitHubFetcher directly. Config gains registry metadata (type, writable, enabled, builtin).

**Tech Stack:** Go 1.26, existing go-github v69, existing Cobra + Charm stack unchanged.

**Depends on:** Plan 1 (tools rename, config YAML, state namespacing) being complete.

---

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Provider | `internal/provider/provider.go` | Create | Provider interface, File type alias |
| Provider | `internal/provider/provider_test.go` | Create | Interface satisfaction tests |
| Provider | `internal/provider/github.go` | Create | GitHubProvider struct, Discover (scribe.yaml + scribe.toml), Fetch |
| Provider | `internal/provider/github_test.go` | Create | Discovery chain tests with mock client |
| Provider | `internal/provider/marketplace.go` | Create | marketplace.json parsing, flattenPlugins |
| Provider | `internal/provider/marketplace_test.go` | Create | Parse + flatten tests |
| Provider | `internal/provider/treescan.go` | Create | SKILL.md tree scan discovery |
| Provider | `internal/provider/treescan_test.go` | Create | Tree scan tests |
| GitHub | `internal/github/client.go` | Modify | Add GetTree method, HasPushAccess method |
| Manifest | `internal/manifest/manifest.go` | Modify | Add Group field to Entry |
| Config | `internal/config/config.go` | Modify | Add RegistryConfig with type/writable/enabled/builtin fields |
| Workflow | `internal/workflow/connect.go` | Modify | Use Provider.Discover, drop [team] requirement |
| Workflow | `internal/workflow/bag.go` | Modify | Add Provider field |
| Cmd | `cmd/connect.go` | Modify | Move to `scribe registry connect` subcommand |
| Cmd | `cmd/registry.go` | Modify | Add connect, enable, disable subcommands |
| Cmd | `cmd/registry_enable.go` | Create | Enable/disable subcommands |
| Cmd | `cmd/root.go` | Modify | Add PersistentPreRun for first-run, remove top-level connectCmd |
| Sync | `internal/sync/syncer.go` | Modify | Accept Provider, delegate Fetch |
| Sync | `internal/sync/adapter.go` | Modify | Implement Provider interface on ghAdapter |

---

### Task 1: Provider interface

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/provider_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package provider_test

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/targets"
)

// TestProviderInterfaceSatisfaction verifies that the interface contract
// is correct by checking a mock satisfies it.
func TestProviderInterfaceSatisfaction(t *testing.T) {
	var _ provider.Provider = &mockProvider{}
}

type mockProvider struct{}

func (m *mockProvider) Discover(ctx context.Context, repo string) ([]manifest.Entry, error) {
	return []manifest.Entry{{Name: "test"}}, nil
}

func (m *mockProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]targets.SkillFile, error) {
	return []targets.SkillFile{{Path: "SKILL.md", Content: []byte("# test")}}, nil
}

func TestFileTypeAlias(t *testing.T) {
	// Verify File is the same type as targets.SkillFile.
	var f provider.File
	f.Path = "test"
	f.Content = []byte("hello")

	sf := targets.SkillFile(f)
	if sf.Path != "test" {
		t.Errorf("File alias broken: got %q", sf.Path)
	}
}
```

- [ ] **Step 2: Implement the interface**

```go
package provider

import (
	"context"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/targets"
)

// File is a single file within a downloaded skill directory.
// Re-exported from targets for caller clarity.
type File = targets.SkillFile

// Provider abstracts how skills are discovered and fetched from a repository.
// Different repo formats (scribe.yaml, marketplace.json, bare SKILL.md trees)
// are handled transparently behind this interface.
type Provider interface {
	// Discover probes a repository and returns all discoverable catalog entries.
	// The repo argument is "owner/repo" format.
	Discover(ctx context.Context, repo string) ([]manifest.Entry, error)

	// Fetch downloads all files for a single catalog entry.
	// Returns skill files ready to be written to the canonical store.
	Fetch(ctx context.Context, entry manifest.Entry) ([]File, error)
}
```

- [ ] **Step 3: Verify tests pass**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/provider/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/provider/provider.go internal/provider/provider_test.go
git commit -m "[agent] Add Provider interface for skill discovery and fetching

Step 1 of plan2: provider-registry"
```

---

### Task 2: Add GetTree and HasPushAccess to GitHub client

**Files:**
- Modify: `internal/github/client.go`
- Create: `internal/github/client_test.go` (if not exists, add GetTree unit tests)

- [ ] **Step 1: Write the failing tests**

Create or extend `internal/github/client_test.go`:

```go
package github_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/github"
)

func TestTreeEntryStruct(t *testing.T) {
	// Verify the TreeEntry struct has the fields we need.
	entry := github.TreeEntry{
		Path: "skills/deploy/SKILL.md",
		Type: "blob",
		SHA:  "abc123",
	}
	if entry.Path != "skills/deploy/SKILL.md" {
		t.Errorf("Path: got %q", entry.Path)
	}
	if entry.Type != "blob" {
		t.Errorf("Type: got %q", entry.Type)
	}
}
```

- [ ] **Step 2: Add TreeEntry struct and GetTree method**

Add to `internal/github/client.go`:

```go
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
```

- [ ] **Step 3: Verify it compiles**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/github/client.go internal/github/client_test.go
git commit -m "[agent] Add GetTree and HasPushAccess methods to GitHub client

Step 2 of plan2: provider-registry"
```

---

### Task 3: Add Group field to manifest.Entry

**Files:**
- Modify: `internal/manifest/manifest.go`
- Modify: `internal/manifest/manifest_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/manifest/manifest_test.go`:

```go
func TestEntryGroupField(t *testing.T) {
	// Group is display-only and not serialized to YAML.
	const input = `
apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: my-skill
    source: "github:owner/repo@main"
`
	m, err := manifest.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Set Group programmatically (not from YAML).
	m.Catalog[0].Group = "my-plugin"
	if m.Catalog[0].Group != "my-plugin" {
		t.Errorf("Group: got %q", m.Catalog[0].Group)
	}

	// Verify Group is not serialized.
	out, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(out), "group") {
		t.Errorf("Group should not be serialized, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Add Group field**

Add to the `Entry` struct in `internal/manifest/manifest.go`:

```go
// Entry represents one item in the catalog list.
type Entry struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source,omitempty"`
	Path        string `yaml:"path,omitempty"`
	Type        string `yaml:"type,omitempty"`
	Install     string `yaml:"install,omitempty"`
	Update      string `yaml:"update,omitempty"`
	Author      string `yaml:"author,omitempty"`
	Description string `yaml:"description,omitempty"`
	Timeout     int    `yaml:"timeout,omitempty"`
	Group       string `yaml:"-"` // display-only, set by marketplace discovery
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/manifest/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/manifest/manifest.go internal/manifest/manifest_test.go
git commit -m "[agent] Add Group field to manifest.Entry for marketplace plugin grouping

Step 3 of plan2: provider-registry"
```

---

### Task 4: GitHubProvider — scribe.yaml/toml discovery + Fetch

**Files:**
- Create: `internal/provider/github.go`
- Create: `internal/provider/github_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/targets"
)

// stubClient implements the provider.GitHubClient interface for testing.
type stubClient struct {
	files     map[string][]byte          // key: "owner/repo/path"
	dirs      map[string][]targets.SkillFile // key: "owner/repo/dirPath"
	treeFiles []provider.TreeEntry
	pushAccess bool
}

func (s *stubClient) FetchFile(_ context.Context, owner, repo, path, ref string) ([]byte, error) {
	key := owner + "/" + repo + "/" + path
	if data, ok := s.files[key]; ok {
		return data, nil
	}
	return nil, errors.New("not found: " + key)
}

func (s *stubClient) FetchDirectory(_ context.Context, owner, repo, dirPath, ref string) ([]targets.SkillFile, error) {
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

func TestDiscoverNoManifestFallsToTreeScan(t *testing.T) {
	client := &stubClient{
		treeFiles: []provider.TreeEntry{
			{Path: "skills/deploy/SKILL.md", Type: "blob"},
			{Path: "skills/lint/SKILL.md", Type: "blob"},
			{Path: "README.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	entries, err := p.Discover(context.Background(), "acme/community-skills")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["deploy"] || !names["lint"] {
		t.Errorf("expected deploy and lint, got %v", names)
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

func TestFetchDelegatesToFetchDirectory(t *testing.T) {
	client := &stubClient{
		dirs: map[string][]targets.SkillFile{
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
```

- [ ] **Step 2: Define the GitHubClient interface consumed by the provider**

Add to `internal/provider/github.go`:

```go
package provider

import (
	"context"
	"fmt"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/targets"
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
	FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]targets.SkillFile, error)
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
	GetTree(ctx context.Context, owner, repo, ref string) ([]TreeEntry, error)
	HasPushAccess(ctx context.Context, owner, repo string) (bool, error)
}

// GitHubProvider discovers and fetches skills from GitHub repositories.
// It implements the Provider interface with a four-step discovery chain:
// 1. scribe.yaml (canonical manifest)
// 2. scribe.toml (legacy, auto-converted)
// 3. .claude-plugin/marketplace.json (plugin format)
// 4. SKILL.md tree scan (bare repos)
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
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/provider/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "[agent] Implement GitHubProvider with scribe.yaml/toml discovery and Fetch

Step 4 of plan2: provider-registry"
```

---

### Task 5: marketplace.json discovery

**Files:**
- Create: `internal/provider/marketplace.go`
- Create: `internal/provider/marketplace_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/provider"
)

func TestParseMarketplace(t *testing.T) {
	raw := []byte(`{
		"name": "acme-plugins",
		"plugins": [
			{
				"name": "deploy-tools",
				"source": "./plugins/deploy-tools",
				"skills": ["skills/deploy", "skills/rollback"]
			},
			{
				"name": "testing",
				"source": "./plugins/testing",
				"skills": ["skills/unit-test"]
			}
		]
	}`)

	entries, err := provider.ParseMarketplace(raw, "acme", "plugins-repo")
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	// First plugin's skills.
	if entries[0].Name != "deploy" {
		t.Errorf("entry[0].Name: got %q, want deploy", entries[0].Name)
	}
	if entries[0].Group != "deploy-tools" {
		t.Errorf("entry[0].Group: got %q, want deploy-tools", entries[0].Group)
	}
	if entries[0].Source != "github:acme/plugins-repo@HEAD" {
		t.Errorf("entry[0].Source: got %q", entries[0].Source)
	}
	if entries[0].Path != "plugins/deploy-tools/skills/deploy" {
		t.Errorf("entry[0].Path: got %q", entries[0].Path)
	}

	if entries[1].Name != "rollback" {
		t.Errorf("entry[1].Name: got %q", entries[1].Name)
	}

	if entries[2].Name != "unit-test" {
		t.Errorf("entry[2].Name: got %q", entries[2].Name)
	}
	if entries[2].Group != "testing" {
		t.Errorf("entry[2].Group: got %q", entries[2].Group)
	}
}

func TestParseMarketplaceEmptyPlugins(t *testing.T) {
	raw := []byte(`{"name": "empty", "plugins": []}`)
	entries, err := provider.ParseMarketplace(raw, "acme", "repo")
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseMarketplaceInvalidJSON(t *testing.T) {
	_, err := provider.ParseMarketplace([]byte("{invalid"), "acme", "repo")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
```

- [ ] **Step 2: Implement marketplace parsing**

```go
package provider

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
)

const marketplacePath = ".claude-plugin/marketplace.json"

// marketplaceFile is the JSON structure of .claude-plugin/marketplace.json.
type marketplaceFile struct {
	Name    string             `json:"name"`
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplacePlugin struct {
	Name   string   `json:"name"`
	Source string   `json:"source"`
	Skills []string `json:"skills"`
}

// ParseMarketplace parses a marketplace.json byte slice and returns catalog entries.
// Each plugin's skills are flattened into individual entries with Group set to the plugin name.
func ParseMarketplace(data []byte, owner, repo string) ([]manifest.Entry, error) {
	var mf marketplaceFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parse marketplace.json: %w", err)
	}

	var entries []manifest.Entry
	source := fmt.Sprintf("github:%s/%s@HEAD", owner, repo)

	for _, plugin := range mf.Plugins {
		// Resolve the plugin source path (strip leading "./").
		pluginDir := strings.TrimPrefix(plugin.Source, "./")

		for _, skillPath := range plugin.Skills {
			// Skill name is the last path component.
			skillName := path.Base(skillPath)

			entries = append(entries, manifest.Entry{
				Name:   skillName,
				Source: source,
				Path:   path.Join(pluginDir, skillPath),
				Author: owner,
				Group:  plugin.Name,
			})
		}
	}

	return entries, nil
}
```

- [ ] **Step 3: Wire marketplace into GitHubProvider.Discover**

Add the `discoverMarketplace` method to `internal/provider/github.go`:

```go
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
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/provider/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/provider/marketplace.go internal/provider/marketplace_test.go internal/provider/github.go
git commit -m "[agent] Add marketplace.json discovery for Claude plugin format

Step 5 of plan2: provider-registry"
```

---

### Task 6: SKILL.md tree scan discovery

**Files:**
- Create: `internal/provider/treescan.go`
- Create: `internal/provider/treescan_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/provider"
)

func TestScanTreeForSkills(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "skills/deploy/SKILL.md", Type: "blob"},
		{Path: "skills/deploy/scripts/run.sh", Type: "blob"},
		{Path: "skills/lint/SKILL.md", Type: "blob"},
		{Path: "docs/README.md", Type: "blob"},
		{Path: "SKILL.md", Type: "blob"}, // root-level SKILL.md
		{Path: "nested/deep/tool/SKILL.md", Type: "blob"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "community-skills")

	if len(entries) != 4 {
		t.Fatalf("entries: got %d, want 4", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
		if e.Source != "github:acme/community-skills@HEAD" {
			t.Errorf("source for %s: got %q", e.Name, e.Source)
		}
		if e.Author != "acme" {
			t.Errorf("author for %s: got %q", e.Name, e.Author)
		}
	}

	if !names["deploy"] {
		t.Error("expected deploy skill")
	}
	if !names["lint"] {
		t.Error("expected lint skill")
	}
	if !names["community-skills"] {
		t.Error("expected root-level skill named after repo")
	}
	if !names["tool"] {
		t.Error("expected nested/deep/tool skill")
	}
}

func TestScanTreeForSkillsEmpty(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "README.md", Type: "blob"},
		{Path: "src/main.go", Type: "blob"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "no-skills")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestScanTreeSkipsTreeEntryType(t *testing.T) {
	// SKILL.md entry with type "tree" should be ignored.
	tree := []provider.TreeEntry{
		{Path: "skills/deploy/SKILL.md", Type: "tree"},
	}

	entries := provider.ScanTreeForSkills(tree, "acme", "repo")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for tree type, got %d", len(entries))
	}
}
```

- [ ] **Step 2: Implement tree scan**

```go
package provider

import (
	"fmt"
	"path"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
)

const skillFileName = "SKILL.md"

// ScanTreeForSkills finds all SKILL.md files in a tree listing and returns
// catalog entries for each. The skill name is derived from the parent directory.
// A root-level SKILL.md uses the repo name.
func ScanTreeForSkills(tree []TreeEntry, owner, repo string) []manifest.Entry {
	source := fmt.Sprintf("github:%s/%s@HEAD", owner, repo)

	var entries []manifest.Entry
	for _, entry := range tree {
		if entry.Type != "blob" {
			continue
		}
		if path.Base(entry.Path) != skillFileName {
			continue
		}

		// Derive skill name from parent directory.
		dir := path.Dir(entry.Path)
		var name, skillPath string
		if dir == "." {
			// Root-level SKILL.md — use repo name.
			name = repo
			skillPath = "."
		} else {
			// Name is the immediate parent dir.
			name = path.Base(dir)
			skillPath = dir
		}

		// Avoid duplicate names by appending parent path for deeply nested skills.
		// For now, use the immediate parent — collisions are unlikely in practice.
		entries = append(entries, manifest.Entry{
			Name:   name,
			Source: source,
			Path:   skillPath,
			Author: owner,
		})
	}

	return entries
}
```

- [ ] **Step 3: Wire tree scan into GitHubProvider.Discover**

Add the `discoverTreeScan` method to `internal/provider/github.go`:

```go
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
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/provider/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/provider/treescan.go internal/provider/treescan_test.go internal/provider/github.go
git commit -m "[agent] Add SKILL.md tree scan as final discovery fallback

Step 6 of plan2: provider-registry"
```

---

### Task 7: Discovery chain integration test

**Files:**
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Write integration-style tests for the full chain**

Add to `internal/provider/github_test.go`:

```go
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
			"acme/repo/scribe.yaml":                   []byte(yamlContent),
			"acme/repo/.claude-plugin/marketplace.json": []byte(`{"name":"mp","plugins":[]}`),
		},
		treeFiles: []provider.TreeEntry{
			{Path: "skills/from-tree/SKILL.md", Type: "blob"},
		},
	}

	p := provider.NewGitHubProvider(client)
	entries, err := p.Discover(context.Background(), "acme/repo")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(entries) != 1 || entries[0].Name != "from-yaml" {
		t.Errorf("expected scribe.yaml to win, got %v", entries)
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
	entries, err := p.Discover(context.Background(), "acme/repo")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(entries) != 1 || entries[0].Name != "deploy" {
		t.Errorf("expected marketplace entry, got %v", entries)
	}
	if entries[0].Group != "plug1" {
		t.Errorf("expected Group=plug1, got %q", entries[0].Group)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/provider/... -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/provider/github_test.go
git commit -m "[agent] Add discovery chain integration tests

Step 7 of plan2: provider-registry"
```

---

### Task 8: Registry type inference + config changes

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go` (or extend)

After Plan 1 converts config to YAML, the Config struct will use `yaml` tags. This task adds RegistryConfig with metadata fields. Until Plan 1 lands, use TOML tags (the field names are the same).

- [ ] **Step 1: Write the failing tests**

```go
package config_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestRegistryConfigDefaults(t *testing.T) {
	rc := config.RegistryConfig{
		Repo: "acme/team-skills",
	}
	if !rc.IsEnabled() {
		t.Error("expected new registry to be enabled by default")
	}
	if rc.IsBuiltin() {
		t.Error("expected new registry to not be builtin by default")
	}
}

func TestRegistryType(t *testing.T) {
	cases := []struct {
		name     string
		regType  string
		writable bool
		wantTeam bool
	}{
		{"team registry", "team", true, true},
		{"community registry", "community", false, false},
		{"marketplace", "marketplace", false, false},
		{"package", "package", false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := config.RegistryConfig{
				Repo:     "owner/repo",
				Type:     c.regType,
				Writable: c.writable,
			}
			if rc.IsTeam() != c.wantTeam {
				t.Errorf("IsTeam: got %v, want %v", rc.IsTeam(), c.wantTeam)
			}
		})
	}
}
```

- [ ] **Step 2: Add RegistryConfig to config**

Add to `internal/config/config.go`:

```go
// RegistryConfig holds metadata about a connected registry.
type RegistryConfig struct {
	Repo     string `toml:"repo"`
	Type     string `toml:"type,omitempty"`     // "team", "community", "marketplace", "package"
	Writable bool   `toml:"writable,omitempty"` // user has push access
	Enabled  *bool  `toml:"enabled,omitempty"`  // nil = true (default enabled)
	Builtin  bool   `toml:"builtin,omitempty"`  // auto-added by first-run
}

// IsEnabled returns whether this registry is active. Defaults to true if not set.
func (rc RegistryConfig) IsEnabled() bool {
	if rc.Enabled == nil {
		return true
	}
	return *rc.Enabled
}

// IsBuiltin returns whether this was auto-added during first-run setup.
func (rc RegistryConfig) IsBuiltin() bool {
	return rc.Builtin
}

// IsTeam returns whether this is a team registry (has write access and team manifest).
func (rc RegistryConfig) IsTeam() bool {
	return rc.Type == "team"
}
```

Add `Registries` field to the `Config` struct:

```go
type Config struct {
	TeamRepos  []string         `toml:"team_repos"`
	Registries []RegistryConfig `toml:"registries,omitempty"`
	Token      string           `toml:"token"`
}
```

Add helper methods:

```go
// FindRegistry returns the RegistryConfig for a given repo, or nil if not found.
func (c *Config) FindRegistry(repo string) *RegistryConfig {
	for i := range c.Registries {
		if strings.EqualFold(c.Registries[i].Repo, repo) {
			return &c.Registries[i]
		}
	}
	return nil
}

// AddRegistry adds or updates a registry in the config.
func (c *Config) AddRegistry(rc RegistryConfig) {
	for i := range c.Registries {
		if strings.EqualFold(c.Registries[i].Repo, rc.Repo) {
			c.Registries[i] = rc
			return
		}
	}
	c.Registries = append(c.Registries, rc)
}

// EnabledRegistries returns all registries that are enabled.
func (c *Config) EnabledRegistries() []RegistryConfig {
	var enabled []RegistryConfig
	for _, rc := range c.Registries {
		if rc.IsEnabled() {
			enabled = append(enabled, rc)
		}
	}
	return enabled
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./internal/config/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "[agent] Add RegistryConfig with type, writable, enabled, builtin fields

Step 8 of plan2: provider-registry"
```

---

### Task 9: Rework connect command to use Provider

**Files:**
- Modify: `internal/workflow/connect.go`
- Modify: `internal/workflow/bag.go`
- Modify: `cmd/connect.go`
- Modify: `cmd/registry.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Add Provider field to Bag**

In `internal/workflow/bag.go`, add the Provider field:

```go
import (
	// ... existing imports ...
	"github.com/Naoray/scribe/internal/provider"
)

type Bag struct {
	// ... existing fields ...

	// Provider is the skill discovery/fetch backend. Set by StepLoadConfig.
	Provider provider.Provider
}
```

- [ ] **Step 2: Rewrite StepFetchManifest to use Provider.Discover**

In `internal/workflow/connect.go`, replace `StepFetchManifest`:

```go
func StepFetchManifest(ctx context.Context, b *Bag) error {
	if b.Provider == nil {
		return fmt.Errorf("internal: Provider not set in workflow bag")
	}

	entries, err := b.Provider.Discover(ctx, b.RepoArg)
	if err != nil {
		return fmt.Errorf("could not discover skills in %s: %w", b.RepoArg, err)
	}

	// Build a minimal manifest from discovered entries.
	b.manifest = &manifest.Manifest{
		APIVersion: "scribe/v1",
		Kind:       "Registry",
		Team:       &manifest.Team{Name: b.RepoArg},
		Catalog:    entries,
	}
	return nil
}
```

- [ ] **Step 3: Drop [team] requirement in StepValidateManifest**

Replace `StepValidateManifest` to just check that entries were found:

```go
func StepValidateManifest(_ context.Context, b *Bag) error {
	if b.manifest == nil || len(b.manifest.Catalog) == 0 {
		return fmt.Errorf("%s: no skills discovered — is this a valid skill registry?", b.RepoArg)
	}
	return nil
}
```

- [ ] **Step 4: Add StepInferRegistryType**

Add a new step after StepValidateManifest in `internal/workflow/connect.go`:

```go
func StepInferRegistryType(ctx context.Context, b *Bag) error {
	owner, repo, err := manifest.ParseOwnerRepo(b.RepoArg)
	if err != nil {
		return err
	}

	regType := "community"
	if b.manifest.IsRegistry() {
		regType = "team"
	}

	writable := false
	if b.Client != nil {
		writable, _ = b.Client.HasPushAccess(ctx, owner, repo)
	}

	b.Config.AddRegistry(config.RegistryConfig{
		Repo:     b.RepoArg,
		Type:     regType,
		Writable: writable,
	})

	return nil
}
```

Update `ConnectSteps()` to include the new step:

```go
func ConnectSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"DedupCheck", StepDedupCheck},
		{"FetchManifest", StepFetchManifest},
		{"ValidateManifest", StepValidateManifest},
		{"InferRegistryType", StepInferRegistryType},
		{"SaveConfig", StepSaveConfig},
		{"LoadState", StepLoadState},
		{"SetSingleRepo", StepSetSingleRepo},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTargets", StepResolveTargets},
		{"SyncSkills", StepConnectSyncError},
	}
}
```

- [ ] **Step 5: Wire Provider in StepLoadConfig**

In `internal/workflow/sync.go`, update `StepLoadConfig` to create the Provider:

```go
import (
	// ... existing imports ...
	"github.com/Naoray/scribe/internal/provider"
)

func StepLoadConfig(ctx context.Context, b *Bag) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	b.Config = cfg
	b.Client = gh.NewClient(ctx, cfg.Token)

	// Wrap the GitHub client into a Provider for discovery/fetch.
	ghProvider := provider.NewGitHubProvider(provider.WrapGitHubClient(b.Client))
	b.Provider = ghProvider

	return nil
}
```

- [ ] **Step 6: Move connect to `scribe registry connect`**

Move the connect command registration. In `cmd/registry.go`:

```go
func init() {
	registryCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(connectCmd) // move connect under registry
}
```

In `cmd/root.go`, remove the top-level connectCmd and add a backward-compat alias:

```go
func init() {
	// rootCmd.AddCommand(connectCmd)  // removed — now under registry
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(guideCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(migrateCmd)

	// Backward compat: top-level "connect" as alias
	aliasConnect := *connectCmd
	aliasConnect.Hidden = true
	aliasConnect.Deprecated = "use 'scribe registry connect' instead"
	rootCmd.AddCommand(&aliasConnect)
}
```

- [ ] **Step 7: Create WrapGitHubClient adapter in provider package**

Add to `internal/provider/github.go` a convenience adapter so the provider package can consume the real `*github.Client`:

```go
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

func (a *clientAdapter) FetchDirectory(ctx context.Context, owner, repo, dirPath, ref string) ([]targets.SkillFile, error) {
	ghFiles, err := a.client.FetchDirectory(ctx, owner, repo, dirPath, ref)
	if err != nil {
		return nil, err
	}
	files := make([]targets.SkillFile, len(ghFiles))
	for i, f := range ghFiles {
		files[i] = targets.SkillFile{Path: f.Path, Content: f.Content}
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
```

- [ ] **Step 8: Run tests and build**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./...
```

- [ ] **Step 9: Commit**

```bash
git add internal/workflow/connect.go internal/workflow/bag.go internal/workflow/sync.go cmd/connect.go cmd/registry.go cmd/root.go internal/provider/github.go
git commit -m "[agent] Rework connect to use Provider.Discover, move to registry subcommand

Step 9 of plan2: provider-registry"
```

---

### Task 10: Registry enable/disable commands

**Files:**
- Create: `cmd/registry_enable.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/registry_enable_test.go`:

```go
package cmd

import (
	"testing"
)

func TestRegistryEnableDisableArgs(t *testing.T) {
	// enableCmd and disableCmd should require exactly one arg.
	if registryEnableCmd.Args == nil {
		t.Error("enableCmd should have arg validation")
	}
	if registryDisableCmd.Args == nil {
		t.Error("disableCmd should have arg validation")
	}
}
```

- [ ] **Step 2: Implement enable/disable commands**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
)

var registryEnableCmd = &cobra.Command{
	Use:   "enable <owner/repo>",
	Short: "Enable a connected registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegistryEnable,
}

var registryDisableCmd = &cobra.Command{
	Use:   "disable <owner/repo>",
	Short: "Disable a connected registry (keeps config, skips during sync)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegistryDisable,
}

func init() {
	registryCmd.AddCommand(registryEnableCmd)
	registryCmd.AddCommand(registryDisableCmd)
}

func runRegistryEnable(cmd *cobra.Command, args []string) error {
	return setRegistryEnabled(args[0], true)
}

func runRegistryDisable(cmd *cobra.Command, args []string) error {
	return setRegistryEnabled(args[0], false)
}

func setRegistryEnabled(repo string, enabled bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Try to resolve against known registries.
	allRepos := make([]string, 0, len(cfg.Registries)+len(cfg.TeamRepos))
	for _, r := range cfg.Registries {
		allRepos = append(allRepos, r.Repo)
	}
	// Also check legacy TeamRepos.
	for _, r := range cfg.TeamRepos {
		found := false
		for _, ar := range allRepos {
			if ar == r {
				found = true
				break
			}
		}
		if !found {
			allRepos = append(allRepos, r)
		}
	}

	resolved, err := resolveRegistry(repo, allRepos)
	if err != nil {
		return err
	}

	rc := cfg.FindRegistry(resolved)
	if rc == nil {
		// Create a minimal registry config for legacy TeamRepos entries.
		cfg.AddRegistry(config.RegistryConfig{
			Repo:    resolved,
			Enabled: &enabled,
		})
	} else {
		rc.Enabled = &enabled
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Registry %s %s\n", resolved, action)
	return nil
}
```

- [ ] **Step 3: Run tests and build**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./cmd/...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/registry_enable.go cmd/registry_enable_test.go
git commit -m "[agent] Add registry enable/disable subcommands

Step 10 of plan2: provider-registry"
```

---

### Task 11: Built-in registries + first-run setup

**Files:**
- Modify: `cmd/root.go`
- Create: `internal/firstrun/firstrun.go`
- Create: `internal/firstrun/firstrun_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package firstrun_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
)

func TestBuiltinRegistries(t *testing.T) {
	registries := firstrun.BuiltinRegistries()
	if len(registries) == 0 {
		t.Fatal("expected at least one built-in registry")
	}

	for _, r := range registries {
		if r.Repo == "" {
			t.Error("builtin registry has empty repo")
		}
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
	}
}

func TestIsFirstRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// No config file exists yet = first run.
	if !firstrun.IsFirstRun() {
		t.Error("expected first run when no config exists")
	}
}

func TestApplyBuiltins(t *testing.T) {
	cfg := &config.Config{}
	firstrun.ApplyBuiltins(cfg)

	if len(cfg.Registries) == 0 {
		t.Fatal("expected registries to be populated")
	}

	for _, r := range cfg.Registries {
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
		if !r.IsEnabled() {
			t.Errorf("%s: expected enabled by default", r.Repo)
		}
	}
}
```

- [ ] **Step 2: Implement firstrun package**

```go
package firstrun

import (
	"os"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/paths"
)

// BuiltinRegistries returns the list of well-known community registries
// that Scribe auto-adds during first-run setup.
var builtinRepos = []string{
	"anthropic/skills",
	"openai/codex-skills",
	"expo/skills",
}

// BuiltinRegistries returns RegistryConfig entries for built-in registries.
func BuiltinRegistries() []config.RegistryConfig {
	registries := make([]config.RegistryConfig, len(builtinRepos))
	for i, repo := range builtinRepos {
		registries[i] = config.RegistryConfig{
			Repo:    repo,
			Type:    "community",
			Builtin: true,
		}
	}
	return registries
}

// IsFirstRun returns true if no config file exists yet.
func IsFirstRun() bool {
	path, err := paths.ConfigPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err)
}

// ApplyBuiltins adds built-in registries to the config if not already present.
func ApplyBuiltins(cfg *config.Config) {
	for _, builtin := range BuiltinRegistries() {
		if cfg.FindRegistry(builtin.Repo) == nil {
			cfg.AddRegistry(builtin)
		}
	}
}
```

- [ ] **Step 3: Add PersistentPreRun hook to root command**

In `cmd/root.go`, add a first-run check:

```go
import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
)

var rootCmd = &cobra.Command{
	Use:     "scribe",
	Short:   "Team skill sync for AI coding agents",
	Long:    "Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.",
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip first-run for meta commands.
		if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "migrate" {
			return nil
		}

		if !firstrun.IsFirstRun() {
			return nil
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		firstrun.ApplyBuiltins(cfg)

		if isatty.IsTerminal(os.Stdout.Fd()) {
			fmt.Println("Welcome to Scribe! Adding built-in community registries...")
			for _, r := range cfg.EnabledRegistries() {
				if r.Builtin {
					fmt.Printf("  + %s\n", r.Repo)
				}
			}
			fmt.Println()
		}

		return cfg.Save()
	},
}
```

- [ ] **Step 4: Run tests and build**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./internal/firstrun/... ./cmd/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/firstrun/firstrun.go internal/firstrun/firstrun_test.go cmd/root.go
git commit -m "[agent] Add built-in community registries and first-run setup

Step 11 of plan2: provider-registry"
```

---

### Task 12: Integrate Provider into sync engine

**Files:**
- Modify: `internal/sync/syncer.go`
- Modify: `internal/sync/adapter.go`
- Modify: `internal/workflow/sync.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/sync/syncer_test.go` (create if needed):

```go
package sync_test

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

type fakeProvider struct {
	entries []manifest.Entry
	files   map[string][]targets.SkillFile
}

func (f *fakeProvider) Discover(_ context.Context, repo string) ([]manifest.Entry, error) {
	return f.entries, nil
}

func (f *fakeProvider) Fetch(_ context.Context, entry manifest.Entry) ([]targets.SkillFile, error) {
	if files, ok := f.files[entry.Name]; ok {
		return files, nil
	}
	return []targets.SkillFile{{Path: "SKILL.md", Content: []byte("# " + entry.Name)}}, nil
}

func TestSyncerWithProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	fp := &fakeProvider{
		entries: []manifest.Entry{
			{Name: "test-skill", Source: "github:acme/repo@v1.0.0", Path: "skills/test"},
		},
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}

	syncer := &sync.Syncer{
		Client:   &sync.NoopFetcher{},
		Provider: fp,
		Targets:  []targets.Target{},
	}

	statuses, _, err := syncer.Diff(context.Background(), "acme/repo", st)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("statuses: got %d, want 1", len(statuses))
	}
	if statuses[0].Status != sync.StatusMissing {
		t.Errorf("status: got %v, want StatusMissing", statuses[0].Status)
	}
}
```

- [ ] **Step 2: Add Provider field to Syncer**

In `internal/sync/syncer.go`, add the Provider field and update FetchManifest:

```go
import (
	// ... existing imports ...
	"github.com/Naoray/scribe/internal/provider"
)

// Syncer wires manifest, github, targets, and state together.
type Syncer struct {
	Client   GitHubFetcher
	Provider provider.Provider // optional — if set, used for discovery and fetch
	Targets  []targets.Target
	Emit     func(any)
}

// FetchManifest tries Provider.Discover first (if set), then falls back to
// direct file fetch for backward compatibility.
func (s *Syncer) FetchManifest(ctx context.Context, owner, repo string) (*manifest.Manifest, error) {
	if s.Provider != nil {
		entries, err := s.Provider.Discover(ctx, owner+"/"+repo)
		if err != nil {
			return nil, err
		}
		return &manifest.Manifest{
			APIVersion: "scribe/v1",
			Kind:       "Registry",
			Team:       &manifest.Team{Name: owner + "/" + repo},
			Catalog:    entries,
		}, nil
	}

	// Legacy path: direct file fetch.
	raw, err := s.Client.FetchFile(ctx, owner, repo, manifest.ManifestFilename, "HEAD")
	if err == nil {
		return manifest.Parse(raw)
	}

	raw, legacyErr := s.Client.FetchFile(ctx, owner, repo, manifest.LegacyManifestFilename, "HEAD")
	if legacyErr != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	s.emit(LegacyFormatMsg{Repo: owner + "/" + repo})
	return migrate.Convert(raw)
}
```

- [ ] **Step 3: Add NoopFetcher for tests**

Add to `internal/sync/adapter.go`:

```go
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
```

- [ ] **Step 4: Update apply to use Provider.Fetch when available**

In `internal/sync/syncer.go`, modify the `apply` method to use Provider.Fetch:

```go
func (s *Syncer) apply(ctx context.Context, statuses []SkillStatus, st *state.State) error {
	// ... (emit resolved messages — unchanged) ...

	for _, sk := range statuses {
		switch sk.Status {
		case StatusCurrent, StatusExtra:
			// ... unchanged ...

		case StatusMissing, StatusOutdated:
			if sk.IsPackage {
				// ... unchanged ...
				continue
			}

			s.emit(SkillDownloadingMsg{Name: sk.Name})

			var tFiles []targets.SkillFile

			if s.Provider != nil {
				// Use Provider.Fetch.
				files, err := s.Provider.Fetch(ctx, *sk.Entry)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}
				tFiles = files
			} else {
				// Legacy path: direct FetchDirectory.
				src, err := manifest.ParseSource(sk.Entry.Source)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}

				skillPath := sk.Entry.Path
				if skillPath == "" {
					skillPath = sk.Name
				}

				files, err := s.Client.FetchDirectory(ctx, src.Owner, src.Repo, skillPath, src.Ref)
				if err != nil {
					s.emit(SkillErrorMsg{Name: sk.Name, Err: err})
					summary.Failed++
					continue
				}

				tFiles = make([]targets.SkillFile, len(files))
				for i, f := range files {
					tFiles[i] = targets.SkillFile{Path: f.Path, Content: f.Content}
				}
			}

			// Write files to canonical store — unchanged from here on.
			// ... (rest of install logic unchanged) ...
		}
	}
	// ... (rest unchanged) ...
}
```

- [ ] **Step 5: Update workflow to pass Provider to Syncer**

In `internal/workflow/sync.go`, update `StepSyncSkills`:

```go
func StepSyncSkills(ctx context.Context, b *Bag) error {
	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider, // pass through from bag
		Targets:  b.Targets,
		Emit: func(msg any) {
			// ... unchanged ...
		},
	}

	// ... rest unchanged ...
}
```

- [ ] **Step 6: Run tests and build**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/sync/syncer.go internal/sync/adapter.go internal/sync/syncer_test.go internal/workflow/sync.go
git commit -m "[agent] Integrate Provider into sync engine with backward-compat fallback

Step 12 of plan2: provider-registry"
```

---

### Task 13: Filter disabled registries in sync/list workflows

**Files:**
- Modify: `internal/workflow/sync.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/workflow/sync_test.go` (create if needed):

```go
package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestEnabledRegistriesFilter(t *testing.T) {
	enabled := true
	disabled := false

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: &enabled},
			{Repo: "acme/disabled-repo", Enabled: &disabled},
			{Repo: "acme/default-repo"}, // nil Enabled = true
		},
	}

	result := cfg.EnabledRegistries()
	if len(result) != 2 {
		t.Fatalf("expected 2 enabled registries, got %d", len(result))
	}
}
```

- [ ] **Step 2: Update StepFilterRegistries to respect enabled flag**

In `internal/workflow/sync.go`, update `StepFilterRegistries`:

```go
func StepFilterRegistries(_ context.Context, b *Bag) error {
	// Start from all repos (legacy TeamRepos + Registries).
	allRepos := b.Config.TeamRepos

	// If Registries are configured, use them instead (they supersede TeamRepos).
	if len(b.Config.Registries) > 0 {
		enabled := b.Config.EnabledRegistries()
		allRepos = make([]string, len(enabled))
		for i, r := range enabled {
			allRepos[i] = r.Repo
		}
	}

	if b.FilterRegistries != nil {
		repos, err := b.FilterRegistries(b.RepoFlag, allRepos)
		if err != nil {
			return err
		}
		b.Repos = repos
	} else {
		b.Repos = allRepos
	}
	return nil
}
```

- [ ] **Step 3: Run tests and build**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build ./... && go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/workflow/sync.go internal/workflow/sync_test.go
git commit -m "[agent] Filter disabled registries from sync and list workflows

Step 13 of plan2: provider-registry"
```

---

### Task 14: End-to-end verification

- [ ] **Step 1: Run full test suite**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go test ./... -count=1
```

- [ ] **Step 2: Build binary and smoke test**

```bash
cd /Users/krishankonig/Workspace/bets/scribe && go build -o /tmp/scribe-test ./cmd/scribe && /tmp/scribe-test --help
```

- [ ] **Step 3: Verify registry subcommands exist**

```bash
/tmp/scribe-test registry --help
/tmp/scribe-test registry connect --help
/tmp/scribe-test registry enable --help
/tmp/scribe-test registry disable --help
```

- [ ] **Step 4: Verify backward-compat connect alias**

```bash
/tmp/scribe-test connect --help 2>&1 | head -5
# Should show deprecation notice
```

- [ ] **Step 5: Cleanup**

```bash
rm /tmp/scribe-test
```
