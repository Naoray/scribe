package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/source"
	"gopkg.in/yaml.v3"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeYAMLConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissing(t *testing.T) {
	setupHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(cfg.TeamRepos()) != 0 {
		t.Errorf("expected empty TeamRepos, got %v", cfg.TeamRepos())
	}
}

func TestLoadTeamRepos(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, home, `
team_repos = ["ArtistfyHQ/team-skills", "vercel/skills"]
token = "ghp_test"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos()) != 2 {
		t.Fatalf("expected 2 team repos, got %d", len(cfg.TeamRepos()))
	}
	if cfg.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("first repo: got %q", cfg.TeamRepos()[0])
	}
	if cfg.TeamRepos()[1] != "vercel/skills" {
		t.Errorf("second repo: got %q", cfg.TeamRepos()[1])
	}
	if cfg.Token != "ghp_test" {
		t.Errorf("token: got %q", cfg.Token)
	}
	for _, registry := range cfg.Registries {
		if registry.Visibility != config.RegistryVisibilityUnknown {
			t.Errorf("legacy TOML visibility for %s: got %q, want %q", registry.Repo, registry.Visibility, config.RegistryVisibilityUnknown)
		}
	}
}

func TestLoadLegacyTeamRepo(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, home, `
team_repo = "ArtistfyHQ/team-skills"
token = "ghp_legacy"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos()) != 1 {
		t.Fatalf("expected 1 team repo from legacy migration, got %d", len(cfg.TeamRepos()))
	}
	if cfg.Registries[0].Visibility != config.RegistryVisibilityUnknown {
		t.Errorf("legacy team_repo visibility: got %q, want %q", cfg.Registries[0].Visibility, config.RegistryVisibilityUnknown)
	}
	if cfg.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("migrated repo: got %q", cfg.TeamRepos()[0])
	}
}

func TestLoadLegacyIgnoredWhenNewPresent(t *testing.T) {
	home := setupHome(t)
	// Both keys present — team_repos takes precedence, legacy is ignored.
	writeConfig(t, home, `
team_repo = "old/repo"
team_repos = ["new/repo"]
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TeamRepos()) != 1 || cfg.TeamRepos()[0] != "new/repo" {
		t.Errorf("expected [new/repo], got %v", cfg.TeamRepos())
	}
}

func TestSave(t *testing.T) {
	home := setupHome(t)

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true, Type: config.RegistryTypeGitHub},
		},
		Token: "ghp_test",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.TeamRepos()) != 1 || loaded.TeamRepos()[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("TeamRepos: got %v", loaded.TeamRepos())
	}
	if loaded.Token != "ghp_test" {
		t.Errorf("Token: got %q", loaded.Token)
	}

	// Verify file exists on disk.
	data, err := os.ReadFile(filepath.Join(home, ".scribe", "config.yaml"))
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if len(data) == 0 {
		t.Error("config file is empty")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	setupHome(t)

	original := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "a/b", Enabled: true, Type: config.RegistryTypeGitHub},
			{Repo: "c/d", Enabled: true, Type: config.RegistryTypeGitHub},
		},
		Token: "tok",
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.TeamRepos()) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.TeamRepos()))
	}
	if loaded.TeamRepos()[0] != "a/b" || loaded.TeamRepos()[1] != "c/d" {
		t.Errorf("repos: got %v", loaded.TeamRepos())
	}
}

func TestConfigRoundTripScribeAgentEnabled(t *testing.T) {
	setupHome(t)

	original := &config.Config{
		ScribeAgent: config.ScribeAgentConfig{Enabled: false},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ScribeAgent.Enabled {
		t.Fatal("ScribeAgent.Enabled = true, want false")
	}
}

// --- Migration tests (Task 1) ---

func TestMigrateTOMLToYAML(t *testing.T) {
	home := setupHome(t)

	// Write a legacy TOML config.
	writeConfig(t, home, `
team_repos = ["ArtistfyHQ/team-skills", "vercel/skills"]
token = "ghp_migrate_test"
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify fields migrated correctly.
	if len(cfg.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(cfg.Registries))
	}
	if cfg.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("first registry: got %q", cfg.Registries[0].Repo)
	}
	if cfg.Registries[1].Repo != "vercel/skills" {
		t.Errorf("second registry: got %q", cfg.Registries[1].Repo)
	}
	if cfg.Token != "ghp_migrate_test" {
		t.Errorf("token: got %q", cfg.Token)
	}

	// Verify YAML file was written.
	yamlPath := filepath.Join(home, ".scribe", "config.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Error("expected config.yaml to be created during migration")
	}

	// Verify TOML backup preserved.
	tomlPath := filepath.Join(home, ".scribe", "config.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		t.Error("expected config.toml to be preserved as backup")
	}
}

func TestMigrateIdempotent(t *testing.T) {
	home := setupHome(t)

	// Write YAML config directly (already migrated).
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	yamlContent := `registries:
  - repo: ArtistfyHQ/team-skills
    enabled: true
token: ghp_yaml_test
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(cfg.Registries))
	}
	if cfg.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("registry repo: got %q", cfg.Registries[0].Repo)
	}
	if cfg.Token != "ghp_yaml_test" {
		t.Errorf("token: got %q", cfg.Token)
	}
}

func TestMigrateYAMLWinsOverTOML(t *testing.T) {
	home := setupHome(t)
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	// Both files exist -- YAML should win.
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
team_repos = ["old/toml-repo"]
token = "toml_token"
`), 0o644)

	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`registries:
  - repo: new/yaml-repo
    enabled: true
token: yaml_token
`), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 1 || cfg.Registries[0].Repo != "new/yaml-repo" {
		t.Errorf("expected YAML to win, got registries %+v", cfg.Registries)
	}
	if cfg.Token != "yaml_token" {
		t.Errorf("expected YAML token, got %q", cfg.Token)
	}
}

func TestConfig_BuiltinsVersion_RoundTripsThroughYAML(t *testing.T) {
	cfg := &config.Config{BuiltinsVersion: 2}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "builtins_version: 2") {
		t.Errorf("yaml output missing builtins_version: %s", data)
	}

	var decoded config.Config
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.BuiltinsVersion != 2 {
		t.Errorf("want 2, got %d", decoded.BuiltinsVersion)
	}
}

func TestConfig_BuiltinsVersion_OmittedWhenZero(t *testing.T) {
	cfg := &config.Config{}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "builtins_version") {
		t.Errorf("zero builtins_version should be omitted; got: %s", data)
	}
}

func TestLoadYAML(t *testing.T) {
	home := setupHome(t)
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	yamlContent := `registries:
  - repo: ArtistfyHQ/team-skills
    enabled: true
    builtin: false
    type: github
    writable: true
  - repo: vercel/skills
    enabled: true
    builtin: false
    type: github
    writable: false
token: ghp_yaml_full
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0o644)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(cfg.Registries))
	}
	if !cfg.Registries[0].Writable {
		t.Error("first registry should be writable")
	}
	if cfg.Registries[1].Writable {
		t.Error("second registry should not be writable")
	}
}

func TestSaveYAML(t *testing.T) {
	home := setupHome(t)

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true, Type: config.RegistryTypeGitHub, Visibility: config.RegistryVisibilityPrivate, Writable: true},
		},
		Token: "ghp_save_test",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify YAML file written.
	yamlPath := filepath.Join(home, ".scribe", "config.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if len(data) == 0 {
		t.Error("config.yaml is empty")
	}

	// Verify .tmp file cleaned up.
	if _, err := os.Stat(yamlPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after save")
	}

	// Reload and verify round-trip.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.Registries) != 1 || loaded.Registries[0].Repo != "ArtistfyHQ/team-skills" {
		t.Errorf("Registries round-trip: got %+v", loaded.Registries)
	}
	if loaded.Token != "ghp_save_test" {
		t.Errorf("Token round-trip: got %q", loaded.Token)
	}
	if loaded.Registries[0].Visibility != config.RegistryVisibilityPrivate {
		t.Errorf("Visibility round-trip: got %q, want %q", loaded.Registries[0].Visibility, config.RegistryVisibilityPrivate)
	}
}

// --- Registry helper tests (Task 8) ---

func TestRegistryConfigDefaults(t *testing.T) {
	rc := config.RegistryConfig{
		Repo:    "acme/team-skills",
		Enabled: true,
	}
	if !rc.Enabled {
		t.Error("expected enabled")
	}
	if rc.Builtin {
		t.Error("expected not builtin by default")
	}
}

func TestRegistryType(t *testing.T) {
	cases := []struct {
		name     string
		regType  string
		wantTeam bool
	}{
		{"team registry", config.RegistryTypeTeam, true},
		{"community registry", config.RegistryTypeCommunity, false},
		{"marketplace", "marketplace", false},
		{"package", "package", false},
		{"github default", config.RegistryTypeGitHub, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := config.RegistryConfig{
				Repo: "owner/repo",
				Type: c.regType,
			}
			if rc.IsTeam() != c.wantTeam {
				t.Errorf("IsTeam: got %v, want %v", rc.IsTeam(), c.wantTeam)
			}
		})
	}
}

func TestRegistryVisibilityHelpers(t *testing.T) {
	cases := []struct {
		name       string
		visibility string
		wantPublic bool
	}{
		{"public", config.RegistryVisibilityPublic, true},
		{"private", config.RegistryVisibilityPrivate, false},
		{"unknown", config.RegistryVisibilityUnknown, false},
		{"empty", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := config.RegistryConfig{Repo: "owner/repo", Visibility: c.visibility}
			if rc.IsPublic() != c.wantPublic {
				t.Errorf("IsPublic: got %v, want %v", rc.IsPublic(), c.wantPublic)
			}
		})
	}
}

func TestRegistryVisibilityMigration(t *testing.T) {
	cases := []struct {
		name    string
		regType string
		want    string
	}{
		{"community becomes public", config.RegistryTypeCommunity, config.RegistryVisibilityPublic},
		{"team becomes private", config.RegistryTypeTeam, config.RegistryVisibilityPrivate},
		{"github becomes unknown", config.RegistryTypeGitHub, config.RegistryVisibilityUnknown},
		{"empty becomes unknown", "", config.RegistryVisibilityUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := config.RegistryConfig{Repo: "owner/repo", Type: c.regType}
			rc.Normalize()
			if rc.Visibility != c.want {
				t.Errorf("Visibility = %q, want %q", rc.Visibility, c.want)
			}
		})
	}
}

func TestLoadYAMLMigratesLegacyRegistryVisibility(t *testing.T) {
	home := setupHome(t)
	writeYAMLConfig(t, home, `
registries:
  - repo: acme/community
    enabled: true
    type: community
  - repo: acme/team
    enabled: true
    type: team
  - repo: acme/github
    enabled: true
    type: github
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := map[string]string{}
	for _, registry := range cfg.Registries {
		got[registry.Repo] = registry.Visibility
	}
	want := map[string]string{
		"acme/community": config.RegistryVisibilityPublic,
		"acme/team":      config.RegistryVisibilityPrivate,
		"acme/github":    config.RegistryVisibilityUnknown,
	}
	for repo, visibility := range want {
		if got[repo] != visibility {
			t.Errorf("%s visibility = %q, want %q", repo, got[repo], visibility)
		}
	}
}

func TestFindRegistry(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: true},
			{Repo: "acme/community", Enabled: false},
		},
	}

	found := cfg.FindRegistry("acme/team-skills")
	if found == nil {
		t.Fatal("expected to find registry")
	}
	if found.Repo != "acme/team-skills" {
		t.Errorf("repo: got %q", found.Repo)
	}

	// Case-insensitive.
	found = cfg.FindRegistry("ACME/Team-Skills")
	if found == nil {
		t.Fatal("expected case-insensitive match")
	}

	// Not found.
	if cfg.FindRegistry("nonexistent/repo") != nil {
		t.Error("expected nil for nonexistent repo")
	}
}

func TestRegistryConfigSourceSpecLegacyRepo(t *testing.T) {
	rc := config.RegistryConfig{
		Repo:    "Owner/Repo",
		Enabled: true,
		Type:    config.RegistryTypeCommunity,
	}

	spec := rc.SourceSpec()
	if spec.Type != source.SourceGitHub || spec.Repo != "Owner/Repo" {
		t.Fatalf("SourceSpec = %#v", spec)
	}
	parsed, err := config.ParseConfigSource(rc)
	if err != nil {
		t.Fatalf("ParseConfigSource: %v", err)
	}
	if parsed.URL != "https://github.com/Owner/Repo" {
		t.Fatalf("parsed URL = %q", parsed.URL)
	}
	ident := rc.Identity()
	if ident.Key != "github:owner/repo" {
		t.Fatalf("identity key = %q", ident.Key)
	}
}

func TestConfigLoadSupportsNestedSourceWithoutLegacyChurn(t *testing.T) {
	home := setupHome(t)
	content := `registries:
  - repo: acme/legacy-skills
    enabled: true
    type: team
  - id: scoped
    enabled: true
    type: community
    source:
      type: github
      repo: vercel-labs/agent-skills
      ref: main
      path: skills
`
	writeYAMLConfig(t, home, content)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.TeamRepos(); len(got) != 1 || got[0] != "acme/legacy-skills" {
		t.Fatalf("TeamRepos = %#v", got)
	}
	legacy := cfg.FindRegistry("acme/legacy-skills")
	if legacy == nil || legacy.Source != nil {
		t.Fatalf("legacy registry changed: %#v", legacy)
	}
	if got := cfg.FindRegistryByKeyOrRepo("scoped"); got == nil || got.ID != "scoped" {
		t.Fatalf("FindRegistryByKeyOrRepo(id) = %#v", got)
	}
	if got := cfg.FindRegistryByKeyOrRepo("github:vercel-labs/agent-skills:skills"); got == nil || got.ID != "scoped" {
		t.Fatalf("FindRegistryByKeyOrRepo(key) = %#v", got)
	}
	if got := cfg.FindRegistryByKeyOrRepo("https://github.com/vercel-labs/agent-skills"); got == nil || got.ID != "scoped" {
		t.Fatalf("FindRegistryByKeyOrRepo(url) = %#v", got)
	}

	data, err := os.ReadFile(filepath.Join(home, ".scribe", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != content {
		t.Fatalf("config file was churned\n got:\n%s\nwant:\n%s", data, content)
	}
}

func TestEnabledSourcesGeneratesUniqueIDs(t *testing.T) {
	cfg := &config.Config{Registries: []config.RegistryConfig{
		{Repo: "org/skills", Enabled: true},
		{Enabled: true, Source: &source.SourceSpec{Type: source.SourceGitHub, Repo: "other/skills"}, ID: "org-skills"},
	}}

	sources := cfg.EnabledSources()
	if len(sources) != 2 {
		t.Fatalf("EnabledSources len = %d", len(sources))
	}
	if sources[0].ID != "org-skills" || sources[1].ID != "org-skills-2" {
		t.Fatalf("IDs = %q, %q", sources[0].ID, sources[1].ID)
	}
}

func TestLoadRejectsDuplicateIDsAndSourceKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "duplicate ids",
			content: `registries:
  - id: team
    enabled: true
    source:
      type: github
      repo: org/one
  - id: TEAM
    enabled: true
    source:
      type: github
      repo: org/two
`,
		},
		{
			name: "duplicate source keys",
			content: `registries:
  - id: one
    enabled: true
    source:
      type: github
      repo: org/skills
  - id: two
    enabled: true
    source:
      type: github
      repo: ORG/Skills
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := setupHome(t)
			writeYAMLConfig(t, home, tt.content)
			if _, err := config.Load(); err == nil {
				t.Fatal("expected load error")
			}
		})
	}
}

func TestAddRegistryUpdatesExisting(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/skills", Enabled: true, Type: config.RegistryTypeGitHub},
		},
	}

	cfg.AddRegistry(config.RegistryConfig{
		Repo:       "acme/skills",
		Enabled:    true,
		Type:       config.RegistryTypeTeam,
		Visibility: config.RegistryVisibilityPrivate,
		Writable:   true,
	})

	if len(cfg.Registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(cfg.Registries))
	}
	if cfg.Registries[0].Type != config.RegistryTypeTeam {
		t.Errorf("type: got %q, want %q", cfg.Registries[0].Type, config.RegistryTypeTeam)
	}
	if !cfg.Registries[0].Writable {
		t.Error("expected writable to be updated")
	}
	if cfg.Registries[0].Visibility != config.RegistryVisibilityPrivate {
		t.Errorf("visibility: got %q, want %q", cfg.Registries[0].Visibility, config.RegistryVisibilityPrivate)
	}
}

func TestEnabledRegistries(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/enabled", Enabled: true},
			{Repo: "acme/disabled", Enabled: false},
			{Repo: "acme/also-enabled", Enabled: true},
		},
	}

	enabled := cfg.EnabledRegistries()
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled, got %d", len(enabled))
	}
}

// TeamRepos returns the list of registry repo strings for backward compatibility.
func TestTeamReposCompat(t *testing.T) {
	gitWritable := true
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true},
			{Repo: "disabled/repo", Enabled: false},
			{Repo: "vercel/skills", Enabled: true},
			{
				ID:      "github-tree",
				Enabled: true,
				Source: &source.SourceSpec{
					Type: source.SourceGitHub,
					Repo: "nested/github-skills",
					Path: "packs/team",
				},
			},
			{
				ID:      "gitlab",
				Enabled: true,
				Source: &source.SourceSpec{
					Type: source.SourceGitLab,
					Repo: "group/project",
				},
			},
			{
				ID:      "git",
				Enabled: true,
				Source: &source.SourceSpec{
					Type:     source.SourceGit,
					URL:      "https://example.com/team/skills.git",
					Writable: &gitWritable,
				},
			},
			{
				ID:      "local",
				Enabled: true,
				Source: &source.SourceSpec{
					Type: source.SourceLocal,
					Path: "/opt/scribe/skills",
				},
			},
		},
	}
	repos := cfg.TeamRepos()
	if len(repos) != 2 {
		t.Fatalf("expected 2 enabled repos, got %d: %v", len(repos), repos)
	}
	if repos[0] != "ArtistfyHQ/team-skills" || repos[1] != "vercel/skills" {
		t.Errorf("repos: got %v", repos)
	}
}
