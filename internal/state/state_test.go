package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestAddRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, _ := state.Load()

	s.RecordInstall("deploy", state.InstalledSkill{
		Version: "v1.0.0",
		Source:  "github:org/deploy@v1.0.0",
	})

	s.AddRegistry("deploy", "ArtistfyHQ/team-skills")
	s.AddRegistry("deploy", "vercel/skills")
	s.AddRegistry("deploy", "ArtistfyHQ/team-skills") // duplicate — should be a no-op

	skill := s.Installed["deploy"]
	if len(skill.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d: %v", len(skill.Registries), skill.Registries)
	}
}

func TestRemoveRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, _ := state.Load()

	s.RecordInstall("deploy", state.InstalledSkill{
		Version:    "v1.0.0",
		Source:     "github:org/deploy@v1.0.0",
		Registries: []string{"ArtistfyHQ/team-skills", "vercel/skills"},
	})

	s.RemoveRegistry("deploy", "ArtistfyHQ/team-skills")
	skill := s.Installed["deploy"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "vercel/skills" {
		t.Fatalf("expected [vercel/skills], got %v", skill.Registries)
	}
}

func TestMigrationBackfillsRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a legacy state.json with no Registries field.
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"gstack": {
				"version": "v0.12.9.0",
				"source": "github:garrytan/gstack@v0.12.9.0",
				"installed_at": "2026-01-01T00:00:00Z",
				"targets": ["claude"],
				"paths": ["/Users/test/.claude/skills/gstack/"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skill := s.Installed["gstack"]
	if len(skill.Registries) != 0 {
		t.Errorf("pre-migration skill should have empty registries, got %v", skill.Registries)
	}

	// Migrate applies a default registry.
	s.MigrateRegistries("ArtistfyHQ/team-skills")
	skill = s.Installed["gstack"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("after migration: got %v", skill.Registries)
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if s.Installed == nil {
		t.Error("expected non-nil Installed map")
	}
	if len(s.Installed) != 0 {
		t.Errorf("expected empty Installed, got %d entries", len(s.Installed))
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordSync()
	s.RecordInstall("gstack", state.InstalledSkill{
		Version: "v0.12.9.0",
		Source:  "github:garrytan/gstack@v0.12.9.0",
		Targets: []string{"claude"},
		Paths:   []string{"/Users/krishan/.claude/skills/gstack/"},
	})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	if loaded.Team.LastSync.IsZero() {
		t.Error("expected LastSync to be set")
	}

	skill, ok := loaded.Installed["gstack"]
	if !ok {
		t.Fatal("gstack not found in Installed")
	}
	if skill.Version != "v0.12.9.0" {
		t.Errorf("version: got %q", skill.Version)
	}
	if skill.InstalledAt.IsZero() {
		t.Error("expected InstalledAt to be set")
	}
}

func TestAtomicWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, _ := state.Load()
	s.RecordSync()
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmpPath := filepath.Join(home, ".scribe", "state.json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after save")
	}
}

func TestDisplayVersion(t *testing.T) {
	tag := state.InstalledSkill{Version: "v1.0.0"}
	if tag.DisplayVersion() != "v1.0.0" {
		t.Errorf("tag: got %q", tag.DisplayVersion())
	}

	branch := state.InstalledSkill{
		Version:   "main",
		CommitSHA: "a3f2c1b9e4d5f678",
	}
	if branch.DisplayVersion() != "main@a3f2c1b" {
		t.Errorf("branch: got %q", branch.DisplayVersion())
	}
}

func TestRegistriesRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("deploy", state.InstalledSkill{
		Version:    "v1.0.0",
		Source:     "github:org/deploy@v1.0.0",
		Targets:    []string{"claude"},
		Registries: []string{"ArtistfyHQ/team-skills"},
	})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _ := state.Load()
	skill := loaded.Installed["deploy"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("Registries: got %v", skill.Registries)
	}
}

func TestPackageFieldsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	st.RecordInstall("gstack", state.InstalledSkill{
		Version:    "main",
		CommitSHA:  "abc123",
		Source:     "github:garrytan/gstack@main",
		Targets:    []string{"claude"},
		Paths:      []string{"/home/user/.claude/skills/gstack"},
		Type:       "package",
		InstallCmd: "git clone ...",
		UpdateCmd:  "cd ~/.claude/skills/gstack && git pull",
		CmdHash:    "deadbeef",
		Approval:   "approved",
	})

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	st2, err := state.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	got := st2.Installed["gstack"]
	if got.Type != "package" {
		t.Errorf("Type: got %q, want package", got.Type)
	}
	if got.InstallCmd != "git clone ..." {
		t.Errorf("InstallCmd: got %q", got.InstallCmd)
	}
	if got.UpdateCmd != "cd ~/.claude/skills/gstack && git pull" {
		t.Errorf("UpdateCmd: got %q", got.UpdateCmd)
	}
	if got.CmdHash != "deadbeef" {
		t.Errorf("CmdHash: got %q", got.CmdHash)
	}
	if got.Approval != "approved" {
		t.Errorf("Approval: got %q", got.Approval)
	}
}

func TestRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("gstack", state.InstalledSkill{
		Version:     "v0.12.9.0",
		InstalledAt: time.Now(),
	})
	s.Remove("gstack")

	if _, ok := s.Installed["gstack"]; ok {
		t.Error("expected gstack to be removed")
	}
}

// --- Migration tests (Task 1) ---
// These tests reference fields/methods that don't exist yet (skill.Tools,
// s.LastSync at top level, namespaced keys, etc.). They are expected to fail
// with compilation errors until Tasks 3-4 are implemented.

func TestStateMigrateTargetsToTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write legacy state with "targets" field.
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude", "cursor"],
				"paths": ["/Users/test/.claude/skills/deploy"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skill := s.Installed["deploy"]
	if len(skill.Tools) != 2 {
		t.Fatalf("expected 2 tools after migration, got %d", len(skill.Tools))
	}
	if skill.Tools[0] != "claude" || skill.Tools[1] != "cursor" {
		t.Errorf("tools: got %v", skill.Tools)
	}
}

func TestStatePromoteLastSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.LastSync.IsZero() {
		t.Error("expected LastSync to be promoted from team.last_sync")
	}
	expected := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	if !s.LastSync.Equal(expected) {
		t.Errorf("LastSync: got %v, want %v", s.LastSync, expected)
	}
}

func TestStateNamespaceKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			},
			"recap": {
				"version": "v2.0.0",
				"source": "github:ArtistfyHQ/team-skills@v2.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Bare keys should be namespaced using Registries[0].
	if _, ok := s.Installed["ArtistfyHQ/deploy"]; !ok {
		t.Errorf("expected namespaced key ArtistfyHQ/deploy, got keys: %v", installedKeys(s))
	}
	if _, ok := s.Installed["ArtistfyHQ/recap"]; !ok {
		t.Errorf("expected namespaced key ArtistfyHQ/recap, got keys: %v", installedKeys(s))
	}

	// Bare keys should be gone.
	if _, ok := s.Installed["deploy"]; ok {
		t.Error("bare key 'deploy' should have been removed after namespacing")
	}
}

func TestStateNamespaceKeysNoRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"my-local-skill": {
				"version": "v1.0.0",
				"source": "",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// No registries -- should namespace as "local/<name>".
	if _, ok := s.Installed["local/my-local-skill"]; !ok {
		t.Errorf("expected namespaced key local/my-local-skill, got keys: %v", installedKeys(s))
	}
}

func TestStateMigrateIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	// Already-migrated state: tools field, namespaced keys, top-level last_sync.
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"last_sync": "2026-03-15T10:00:00Z",
		"installed": {
			"ArtistfyHQ/deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Should not double-namespace.
	if _, ok := s.Installed["ArtistfyHQ/deploy"]; !ok {
		t.Errorf("expected key to remain ArtistfyHQ/deploy, got keys: %v", installedKeys(s))
	}
	if len(s.Installed) != 1 {
		t.Errorf("expected 1 installed skill, got %d", len(s.Installed))
	}
}

func TestStateToolsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("ArtistfyHQ/deploy", state.InstalledSkill{
		Version: "v1.0.0",
		Source:  "github:ArtistfyHQ/team-skills@v1.0.0",
		Tools:   []string{"claude", "cursor"},
		Paths:   []string{"/test"},
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := loaded.Installed["ArtistfyHQ/deploy"]
	if len(skill.Tools) != 2 || skill.Tools[0] != "claude" {
		t.Errorf("Tools round-trip: got %v", skill.Tools)
	}
}

// installedKeys is a test helper that returns the keys of the Installed map.
func installedKeys(s *state.State) []string {
	keys := make([]string, 0, len(s.Installed))
	for k := range s.Installed {
		keys = append(keys, k)
	}
	return keys
}
