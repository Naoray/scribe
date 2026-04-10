package storemigrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/state"
)

func TestMigrateMovesToFlat(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")

	// Create skills/my-slug/cleanup/SKILL.md
	skillDir := filepath.Join(storeDir, "my-slug", "cleanup")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Cleanup skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}

	warnings, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// Verify moved to flat location.
	flat := filepath.Join(storeDir, "cleanup", "SKILL.md")
	if _, err := os.Stat(flat); err != nil {
		t.Errorf("expected %s to exist: %v", flat, err)
	}

	// Verify .scribe-base.md exists.
	base := filepath.Join(storeDir, "cleanup", ".scribe-base.md")
	if _, err := os.Stat(base); err != nil {
		t.Errorf("expected %s to exist: %v", base, err)
	}

	// Verify old slug dir is gone.
	if _, err := os.Stat(filepath.Join(storeDir, "my-slug")); !os.IsNotExist(err) {
		t.Errorf("expected slug dir my-slug to be removed")
	}
}

func TestMigrateIdenticalQuarantine(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")
	content := []byte("# Same content")

	// Create slug copy.
	slugDir := filepath.Join(storeDir, "slug", "cleanup")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create flat copy with same content.
	flatDir := filepath.Join(storeDir, "cleanup")
	if err := os.MkdirAll(flatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flatDir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}

	warnings, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for identical content, got: %v", warnings)
	}

	// Slug copy should be deleted.
	if _, err := os.Stat(slugDir); !os.IsNotExist(err) {
		t.Errorf("expected slug copy to be deleted")
	}

	// Flat copy should still exist.
	if _, err := os.Stat(filepath.Join(flatDir, "SKILL.md")); err != nil {
		t.Errorf("flat SKILL.md should still exist: %v", err)
	}

	// No quarantine directory should exist.
	conflictsDir := filepath.Join(tmp, "migration-conflicts")
	if _, err := os.Stat(conflictsDir); !os.IsNotExist(err) {
		t.Errorf("expected no migration-conflicts dir for identical content")
	}
}

func TestMigrateDifferentQuarantine(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")

	// Create slug copy with different content.
	slugDir := filepath.Join(storeDir, "slug", "cleanup")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, "SKILL.md"), []byte("# Version A"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create flat copy with different content.
	flatDir := filepath.Join(storeDir, "cleanup")
	if err := os.MkdirAll(flatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flatDir, "SKILL.md"), []byte("# Version B"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}

	warnings, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}

	// Slug copy should be quarantined.
	quarantined := filepath.Join(tmp, "migration-conflicts", "slug-cleanup")
	if _, err := os.Stat(filepath.Join(quarantined, "SKILL.md")); err != nil {
		t.Errorf("expected quarantined SKILL.md at %s: %v", quarantined, err)
	}

	// Verify quarantined content is the slug version.
	data, err := os.ReadFile(filepath.Join(quarantined, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Version A" {
		t.Errorf("quarantined content = %q, want %q", data, "# Version A")
	}
}

func TestMigrateIdempotent(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")

	// Create a slug directory that should NOT be touched because the marker
	// file says migration already ran.
	slugDir := filepath.Join(storeDir, "slug", "cleanup")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, "SKILL.md"), []byte("# Content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-create the marker — simulates "migration already happened".
	if err := os.WriteFile(filepath.Join(storeDir, migratedMarker), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 2, Installed: make(map[string]state.InstalledSkill)}

	warnings, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// Slug dir should still exist — migration was skipped.
	if _, err := os.Stat(filepath.Join(slugDir, "SKILL.md")); err != nil {
		t.Errorf("slug SKILL.md should still exist (idempotent): %v", err)
	}

	// Flat dir should NOT exist.
	if _, err := os.Stat(filepath.Join(storeDir, "cleanup")); !os.IsNotExist(err) {
		t.Errorf("flat dir should not exist after idempotent skip")
	}

	// AlreadyMigrated should report true.
	if !AlreadyMigrated(storeDir) {
		t.Errorf("AlreadyMigrated should return true when marker exists")
	}
}

func TestMigrateWritesMarker(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")

	// Create a slug-style skill to migrate.
	slugDir := filepath.Join(storeDir, "owner-repo", "cleanup")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, "SKILL.md"), []byte("# Content"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}
	if _, err := Migrate(storeDir, st); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Marker must exist after a successful migration so subsequent runs skip.
	if !AlreadyMigrated(storeDir) {
		t.Errorf("marker should exist after Migrate completes")
	}

	// A second Migrate call must be a no-op (and must not error).
	if _, err := Migrate(storeDir, st); err != nil {
		t.Errorf("second Migrate should be idempotent, got: %v", err)
	}
}

func TestMigrateRemovesEmptySlugDirs(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")

	// Create two skills under the same slug.
	for _, name := range []string{"skill-a", "skill-b"} {
		dir := filepath.Join(storeDir, "my-slug", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}

	_, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Slug dir should be removed since all skills were moved out.
	if _, err := os.Stat(filepath.Join(storeDir, "my-slug")); !os.IsNotExist(err) {
		t.Errorf("expected empty slug dir my-slug to be removed")
	}

	// Both skills should exist in flat layout.
	for _, name := range []string{"skill-a", "skill-b"} {
		if _, err := os.Stat(filepath.Join(storeDir, name, "SKILL.md")); err != nil {
			t.Errorf("expected %s/SKILL.md to exist: %v", name, err)
		}
	}
}

func TestMigrateCreatesBase(t *testing.T) {
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "skills")
	content := []byte("# My skill content\n\nSome detailed instructions.")

	// Create a skill in slug layout.
	skillDir := filepath.Join(storeDir, "registry", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{SchemaVersion: 1, Installed: make(map[string]state.InstalledSkill)}

	_, err := Migrate(storeDir, st)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify .scribe-base.md exists with same content as SKILL.md.
	basePath := filepath.Join(storeDir, "my-skill", ".scribe-base.md")
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("expected .scribe-base.md to exist: %v", err)
	}

	skillData, err := os.ReadFile(filepath.Join(storeDir, "my-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	if string(baseData) != string(skillData) {
		t.Errorf(".scribe-base.md content differs from SKILL.md:\nbase: %q\nskill: %q", baseData, skillData)
	}
}
