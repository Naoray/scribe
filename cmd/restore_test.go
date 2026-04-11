package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

func TestRunRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "cleanup")
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Current SKILL.md at rev 3.
	currentContent := []byte("current content rev 3\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), currentContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Old version to restore.
	oldContent := []byte("old content rev 1\n")
	if err := os.WriteFile(filepath.Join(versionsDir, "rev-1.md"), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create state.
	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"cleanup": {
				Revision:      3,
				InstalledHash: sync.ComputeFileHash(currentContent),
				Tools:         []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	// Restore rev 1.
	cmd := newRestoreCommand()
	cmd.SetArgs([]string{"cleanup", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify SKILL.md has restored content.
	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old content rev 1\n" {
		t.Errorf("expected restored content, got %q", string(got))
	}

	// Verify current version was snapshotted.
	snapshotPath := filepath.Join(versionsDir, "rev-3.md")
	snapData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("snapshot not created: %v", err)
	}
	if string(snapData) != "current content rev 3\n" {
		t.Errorf("snapshot content mismatch: got %q", string(snapData))
	}

	// Verify state: revision bumped to 4.
	st2, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	skill := st2.Installed["cleanup"]
	if skill.Revision != 4 {
		t.Errorf("expected revision 4, got %d", skill.Revision)
	}
	if skill.InstalledHash != sync.ComputeFileHash(oldContent) {
		t.Errorf("expected hash of old content, got %q", skill.InstalledHash)
	}
}

func TestRestoreWithRevPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "deploy")
	versionsDir := filepath.Join(skillDir, "versions")
	os.MkdirAll(versionsDir, 0o755)

	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("current\n"), 0o644)
	os.WriteFile(filepath.Join(versionsDir, "rev-2.md"), []byte("v2 content\n"), 0o644)

	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"deploy": {Revision: 5, InstalledHash: "abc", Tools: []string{"claude"}},
		},
	}
	st.Save()

	cmd := newRestoreCommand()
	cmd.SetArgs([]string{"deploy", "rev-2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if string(got) != "v2 content\n" {
		t.Errorf("expected v2 content, got %q", string(got))
	}
}

func TestRestoreInvalidRevision(t *testing.T) {
	cmd := newRestoreCommand()
	cmd.SetArgs([]string{"test-skill", "abc"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for invalid revision")
	}
}

func TestRestoreVersionNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "cleanup")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("content\n"), 0o644)

	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"cleanup": {Revision: 1, InstalledHash: "abc", Tools: []string{"claude"}},
		},
	}
	st.Save()

	cmd := newRestoreCommand()
	cmd.SetArgs([]string{"cleanup", "999"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}
