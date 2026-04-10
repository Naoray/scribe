package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotVersion(t *testing.T) {
	skillDir := t.TempDir()

	// Write a SKILL.md to snapshot.
	content := []byte("# My Skill\nSome content here.\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SnapshotVersion(skillDir, 3); err != nil {
		t.Fatalf("SnapshotVersion: %v", err)
	}

	snapPath := filepath.Join(skillDir, "versions", "rev-3.md")
	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("snapshot content = %q, want %q", got, content)
	}
}

func TestSnapshotVersionCreatesDir(t *testing.T) {
	skillDir := t.TempDir()

	// Write SKILL.md but don't create versions/ dir.
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SnapshotVersion(skillDir, 1); err != nil {
		t.Fatalf("SnapshotVersion: %v", err)
	}

	// Verify versions/ dir was created and file exists.
	info, err := os.Stat(filepath.Join(skillDir, "versions"))
	if err != nil {
		t.Fatalf("versions dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("versions is not a directory")
	}

	snapPath := filepath.Join(skillDir, "versions", "rev-1.md")
	if _, err := os.Stat(snapPath); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}
}

func TestSnapshotVersionNoSkillMD(t *testing.T) {
	skillDir := t.TempDir()

	// No SKILL.md — should be a no-op.
	if err := SnapshotVersion(skillDir, 1); err != nil {
		t.Fatalf("SnapshotVersion with no SKILL.md: %v", err)
	}

	// versions/ dir should not be created.
	if _, err := os.Stat(filepath.Join(skillDir, "versions")); !os.IsNotExist(err) {
		t.Error("versions dir should not exist when SKILL.md is missing")
	}
}

func TestEnforceRetention(t *testing.T) {
	skillDir := t.TempDir()
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 12 snapshots.
	for i := 1; i <= 12; i++ {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("rev %d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := EnforceRetention(skillDir, 10); err != nil {
		t.Fatalf("EnforceRetention: %v", err)
	}

	// Should have deleted rev-1 and rev-2 (oldest 2).
	for _, rev := range []int{1, 2} {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", rev))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("rev-%d.md should have been deleted", rev)
		}
	}

	// rev-3 through rev-12 should still exist.
	for rev := 3; rev <= 12; rev++ {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", rev))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("rev-%d.md should exist: %v", rev, err)
		}
	}
}

func TestEnforceRetentionUnlimited(t *testing.T) {
	skillDir := t.TempDir()
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 15 snapshots.
	for i := 1; i <= 15; i++ {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", i))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// maxVersions=0 means keep all.
	if err := EnforceRetention(skillDir, 0); err != nil {
		t.Fatalf("EnforceRetention unlimited: %v", err)
	}

	// All 15 should still exist.
	for i := 1; i <= 15; i++ {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", i))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("rev-%d.md should exist: %v", i, err)
		}
	}
}

func TestListVersions(t *testing.T) {
	skillDir := t.TempDir()
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create snapshots out of order.
	for _, rev := range []int{5, 2, 10, 1, 7} {
		path := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", rev))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Also create a non-matching file that should be ignored.
	if err := os.WriteFile(filepath.Join(versionsDir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	versions, err := ListVersions(skillDir)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}

	if len(versions) != 5 {
		t.Fatalf("got %d versions, want 5", len(versions))
	}

	// Should be sorted ascending.
	expected := []int{1, 2, 5, 7, 10}
	for i, v := range versions {
		if v.Revision != expected[i] {
			t.Errorf("versions[%d].Revision = %d, want %d", i, v.Revision, expected[i])
		}
		wantPath := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", expected[i]))
		if v.Path != wantPath {
			t.Errorf("versions[%d].Path = %q, want %q", i, v.Path, wantPath)
		}
	}
}

func TestListVersionsEmptyDir(t *testing.T) {
	skillDir := t.TempDir()

	// No versions/ dir at all.
	versions, err := ListVersions(skillDir)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("got %d versions, want 0", len(versions))
	}
}
