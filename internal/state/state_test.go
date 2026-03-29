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
