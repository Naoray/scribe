package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type browseSourceProvider struct {
	entries []manifest.Entry
}

func (p browseSourceProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{Entries: p.entries, IsTeam: true}, nil
}

func (p browseSourceProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*provider.DiscoverResult, error) {
	return p.Discover(ctx, spec.Repo)
}

func (p browseSourceProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]provider.File, error) {
	return nil, nil
}

func (p browseSourceProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]provider.File, error) {
	return nil, nil
}

func TestRunBrowseWithDeps_JSONQueryFiltersResults(t *testing.T) {
	old := discoverSourceEntriesFn
	defer func() { discoverSourceEntriesFn = old }()
	discoverSourceEntriesFn = func(context.Context, []config.RegistrySource, *gh.Client, []tools.Tool, *state.State) ([]browseEntry, []error) {
		return []browseEntry{
			{Registry: "scoped", SourceKey: "github:acme/skills:skills", Source: source.SourceSpec{Type: source.SourceGitHub, Repo: "acme/skills", Path: "skills"}, Status: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing}},
			{Registry: "scoped", SourceKey: "github:acme/skills:skills", Source: source.SourceSpec{Type: source.SourceGitHub, Repo: "acme/skills", Path: "skills"}, Status: sync.SkillStatus{Name: "deploy", Status: sync.StatusMissing}},
		}, nil
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = runBrowseWithDeps(context.Background(), []config.RegistrySource{{
		ID:       "acme/skills",
		Source:   source.SourceSpec{Type: source.SourceGitHub, Repo: "acme/skills"},
		Identity: source.SourceIdentity{Key: "acme/skills"},
	}}, "clean", "", nil, &state.State{Installed: map[string]state.InstalledSkill{}}, nil, nil, true, true, false)
	w.Close()
	if err != nil {
		t.Fatalf("runBrowseWithDeps() error = %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	var out struct {
		Results []struct {
			Name      string            `json:"name"`
			SourceKey string            `json:"source_key"`
			Source    source.SourceSpec `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal browse json: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Name != "cleanup" {
		t.Fatalf("results = %+v, want only cleanup", out.Results)
	}
	if out.Results[0].SourceKey != "github:acme/skills:skills" || out.Results[0].Source.Path != "skills" {
		t.Fatalf("source fields = %+v", out.Results[0])
	}
}

func TestRunBrowseKitsWithDeps_JSONQueryFiltersResults(t *testing.T) {
	old := discoverKitEntriesFn
	defer func() { discoverKitEntriesFn = old }()
	discoverKitEntriesFn = func(context.Context, []string, registry.FileFetcher, *state.State) ([]kitBrowseEntry, []error) {
		return []kitBrowseEntry{
			{Kit: registry.ManifestKit{Registry: "acme/skills", Name: "baseline", Path: "kits/baseline.yaml", Description: "Laravel baseline"}, Installed: true},
			{Kit: registry.ManifestKit{Registry: "acme/skills", Name: "ops", Path: "kits/ops.yaml", Description: "Ops kit"}},
		}, nil
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = runBrowseKitsWithDeps(context.Background(), []string{"acme/skills"}, "base", "", nil, &state.State{Kits: map[string]state.InstalledKit{}}, nil, true, true)
	w.Close()
	if err != nil {
		t.Fatalf("runBrowseKitsWithDeps() error = %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	var out struct {
		Results []struct {
			Name             string `json:"name"`
			Registry         string `json:"registry"`
			InstalledLocally bool   `json:"installed_locally"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal browse kits json: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Name != "baseline" || !out.Results[0].InstalledLocally {
		t.Fatalf("results = %+v, want installed baseline only", out.Results)
	}
}

func TestLegacyRegistrySourceUsesRepoStateKey(t *testing.T) {
	cfg := &config.Config{Registries: []config.RegistryConfig{{
		Repo:    "acme/skills",
		Enabled: true,
	}}}
	sources := cfg.EnabledSources()
	if len(sources) != 1 {
		t.Fatalf("EnabledSources len = %d, want 1", len(sources))
	}
	sourceKey := registryStateKey(sources[0])
	if sourceKey != "acme/skills" {
		t.Fatalf("registryStateKey = %q, want acme/skills", sourceKey)
	}

	syncer := &sync.Syncer{
		Client: &sync.NoopFetcher{},
		Provider: browseSourceProvider{entries: []manifest.Entry{{
			Name:   "cleanup",
			Source: "github:acme/skills@v1.0.0",
		}}},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"cleanup": {
			Sources: []state.SkillSource{{
				Registry: "acme/skills",
				Ref:      "v1.0.0",
			}},
		},
	}}

	statuses, _, err := syncer.DiffSource(context.Background(), sourceKey, sources[0].Source, st)
	if err != nil {
		t.Fatalf("DiffSource: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].Status != sync.StatusCurrent {
		t.Fatalf("status = %s, want %s", statuses[0].Status, sync.StatusCurrent)
	}
}

func TestBrowseReposAcceptsUnconnectedGitHubSource(t *testing.T) {
	repos, err := browseRepos("https://github.com/vercel-labs/agent-skills", nil)
	if err != nil {
		t.Fatalf("browseRepos() error = %v", err)
	}
	if len(repos) != 1 || repos[0] != "vercel-labs/agent-skills" {
		t.Fatalf("repos = %#v", repos)
	}
}

func TestBrowseReposPrefersConnectedRegistryAlias(t *testing.T) {
	repos, err := browseRepos("skills", []string{"acme/skills"})
	if err != nil {
		t.Fatalf("browseRepos() error = %v", err)
	}
	if len(repos) != 1 || repos[0] != "acme/skills" {
		t.Fatalf("repos = %#v", repos)
	}
}

func TestRunBrowseAcceptsTypedLocalSourceFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := writeBrowseLocalSkill(t, "deploy")

	stdout, stderr, err := executeBrowseCommand(t, "browse", "--source", "local", "--path", root, "--json")
	if err != nil {
		t.Fatalf("browse typed local: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	var out struct {
		Results []struct {
			Name      string            `json:"name"`
			SourceKey string            `json:"source_key"`
			Source    source.SourceSpec `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal browse json: %v\nstdout=%s", err, stdout)
	}
	if len(out.Results) != 1 || out.Results[0].Name != "deploy" {
		t.Fatalf("results = %+v, want deploy", out.Results)
	}
	if out.Results[0].Source.Type != source.SourceLocal || out.Results[0].Source.Path != root || out.Results[0].SourceKey == "" {
		t.Fatalf("source fields = %+v", out.Results[0])
	}
}

func TestRunBrowseInstallLocalSourceDoesNotRequireGitHubAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	root := writeBrowseLocalSkill(t, "deploy")

	stdout, stderr, err := executeBrowseCommand(t, "browse", "--source", root, "--install", "deploy", "--json", "--no-interaction")
	if err != nil {
		if strings.Contains(err.Error(), "GH_AUTH_FAILED") || strings.Contains(stderr, "GH_AUTH_FAILED") {
			t.Fatalf("browse install local source required GitHub auth: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		t.Fatalf("browse install local source: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	var out struct {
		Installed []installResult `json:"installed"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal install json: %v\nstdout=%s", err, stdout)
	}
	if len(out.Installed) != 1 || out.Installed[0].Name != "deploy" || out.Installed[0].Status == "error" {
		t.Fatalf("installed = %+v, want successful deploy install", out.Installed)
	}
}

func writeBrowseLocalSkill(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return root
}

func executeBrowseCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	root.SetArgs(args)
	var stderr bytes.Buffer
	root.SetErr(&stderr)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	execErr := root.Execute()
	if closeErr := w.Close(); closeErr != nil && execErr == nil {
		execErr = closeErr
	}

	var stdout bytes.Buffer
	if _, err := stdout.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return stdout.String(), stderr.String(), execErr
}

func TestNewBrowseCommandSupportsJSON(t *testing.T) {
	if !commandSupportsJSON(newBrowseCommand()) {
		t.Fatal("browse should be marked JSON-supported")
	}
}

func TestBrowseInstallRejectsAmbiguousName(t *testing.T) {
	err := browseInstall(context.Background(), "cleanup", []browseEntry{
		{Registry: "acme/skills", Status: sync.SkillStatus{Name: "cleanup"}},
		{Registry: "other/skills", Status: sync.SkillStatus{Name: "cleanup"}},
	}, nil, nil, nil, nil, true, true, false)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("browseInstall() error = %v, want ambiguous error", err)
	}
}

func TestBrowseInstallRejectsMissingName(t *testing.T) {
	err := browseInstall(context.Background(), "cleanup", nil, nil, nil, nil, nil, true, true, false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("browseInstall() error = %v, want not found error", err)
	}
}

func TestDiscoverSourceEntriesReadsLocalProvider(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Deploy\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	spec, ident, err := source.Canonicalize(source.SourceSpec{Type: source.SourceLocal, Path: root})
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}

	entries, errs := discoverSourceEntries(context.Background(), []config.RegistrySource{{
		ID:       "local-fixture",
		Source:   spec,
		Identity: ident,
	}}, nil, nil, &state.State{Installed: map[string]state.InstalledSkill{}})
	if len(errs) != 0 {
		t.Fatalf("errs = %#v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Status.Name != "deploy" || entries[0].Status.Status != sync.StatusMissing {
		t.Fatalf("entry = %#v", entries[0])
	}
	if entries[0].SourceKey != ident.Key || entries[0].Source.Type != source.SourceLocal {
		t.Fatalf("source fields = %#v", entries[0])
	}
}
