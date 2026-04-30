package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateEmbeddedSkillRenameRenamesStateStoreAndProjections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := filepath.Join(home, ".scribe", "skills")
	oldCanonical := filepath.Join(store, OldEmbeddedSkillName)
	newCanonical := filepath.Join(store, EmbeddedSkillName)
	if err := os.MkdirAll(oldCanonical, 0o755); err != nil {
		t.Fatalf("mkdir old canonical: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldCanonical, "SKILL.md"), []byte("---\nname: scribe\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldCanonical, ".cursor.mdc"), []byte("cursor"), 0o644); err != nil {
		t.Fatalf("write cursor mdc: %v", err)
	}

	claudeOld := filepath.Join(home, ".claude", "skills", OldEmbeddedSkillName)
	codexOld := filepath.Join(home, ".codex", "skills", OldEmbeddedSkillName)
	cursorOld := filepath.Join(home, ".cursor", "rules", OldEmbeddedSkillName+".mdc")
	for _, link := range []string{claudeOld, codexOld, cursorOld} {
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatalf("mkdir projection parent: %v", err)
		}
	}
	if err := os.Symlink(oldCanonical, claudeOld); err != nil {
		t.Fatalf("symlink claude: %v", err)
	}
	if err := os.Symlink(oldCanonical, codexOld); err != nil {
		t.Fatalf("symlink codex: %v", err)
	}
	if err := os.Symlink(filepath.Join(oldCanonical, ".cursor.mdc"), cursorOld); err != nil {
		t.Fatalf("symlink cursor: %v", err)
	}

	st := &State{Installed: map[string]InstalledSkill{
		OldEmbeddedSkillName: {
			Revision:    4,
			InstalledAt: time.Now(),
			Paths:       []string{claudeOld, codexOld, cursorOld},
			ManagedPaths: []string{
				claudeOld,
				codexOld,
				cursorOld,
			},
			Conflicts: []ProjectionConflict{{Tool: "claude", Path: claudeOld}},
			Sources:   []SkillSource{{Registry: "Naoray/scribe", Path: "skills/" + OldEmbeddedSkillName + "/SKILL.md"}},
			Origin:    OriginBootstrap,
		},
	}}

	result, err := MigrateEmbeddedSkillRename(st)
	if err != nil {
		t.Fatalf("MigrateEmbeddedSkillRename() error = %v", err)
	}
	if !result.Changed || result.Conflict {
		t.Fatalf("result = %#v, want changed without conflict", result)
	}
	if _, ok := st.Installed[OldEmbeddedSkillName]; ok {
		t.Fatal("old state entry still exists")
	}
	migrated, ok := st.Installed[EmbeddedSkillName]
	if !ok {
		t.Fatal("new state entry missing")
	}
	if migrated.Revision != 4 || migrated.Origin != OriginBootstrap {
		t.Fatalf("migrated state lost metadata: %#v", migrated)
	}
	for _, path := range append(append([]string{}, migrated.Paths...), migrated.ManagedPaths...) {
		if filepath.Base(path) == OldEmbeddedSkillName || filepath.Base(path) == OldEmbeddedSkillName+".mdc" {
			t.Fatalf("old path retained in state: %q", path)
		}
	}
	if migrated.Conflicts[0].Path != filepath.Join(home, ".claude", "skills", EmbeddedSkillName) {
		t.Fatalf("conflict path = %q", migrated.Conflicts[0].Path)
	}
	if migrated.Sources[0].Path != "skills/"+EmbeddedSkillName+"/SKILL.md" {
		t.Fatalf("source path = %q", migrated.Sources[0].Path)
	}

	if _, err := os.Stat(filepath.Join(newCanonical, "SKILL.md")); err != nil {
		t.Fatalf("new canonical missing: %v", err)
	}
	if _, err := os.Lstat(oldCanonical); !os.IsNotExist(err) {
		t.Fatalf("old canonical should be gone, err=%v", err)
	}
	assertSymlinkTarget(t, filepath.Join(home, ".claude", "skills", EmbeddedSkillName), newCanonical)
	assertSymlinkTarget(t, filepath.Join(home, ".codex", "skills", EmbeddedSkillName), newCanonical)
	assertSymlinkTarget(t, filepath.Join(home, ".cursor", "rules", EmbeddedSkillName+".mdc"), filepath.Join(newCanonical, ".cursor.mdc"))
	for _, oldPath := range []string{claudeOld, codexOld, cursorOld} {
		if _, err := os.Lstat(oldPath); !os.IsNotExist(err) {
			t.Fatalf("old projection %s should be gone, err=%v", oldPath, err)
		}
	}

	second, err := MigrateEmbeddedSkillRename(st)
	if err != nil {
		t.Fatalf("second MigrateEmbeddedSkillRename() error = %v", err)
	}
	if second.Changed || second.Conflict {
		t.Fatalf("second result = %#v, want no-op", second)
	}
}

func TestMigrateEmbeddedSkillRenameLeavesConflictUnmerged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st := &State{Installed: map[string]InstalledSkill{
		OldEmbeddedSkillName: {Revision: 1},
		EmbeddedSkillName:    {Revision: 2},
	}}

	result, err := MigrateEmbeddedSkillRename(st)
	if err != nil {
		t.Fatalf("MigrateEmbeddedSkillRename() error = %v", err)
	}
	if !result.Conflict || result.Changed {
		t.Fatalf("result = %#v, want conflict without change", result)
	}
	if st.Installed[OldEmbeddedSkillName].Revision != 1 || st.Installed[EmbeddedSkillName].Revision != 2 {
		t.Fatalf("state should be left as-is: %#v", st.Installed)
	}
}

func assertSymlinkTarget(t *testing.T, link, want string) {
	t.Helper()
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink %s: %v", link, err)
	}
	if got != want {
		t.Fatalf("readlink %s = %q, want %q", link, got, want)
	}
}
