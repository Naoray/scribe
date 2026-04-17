package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

func TestReclassifyLegacyPackages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, _ := paths.StoreDir()
	pkgsDir, _ := paths.PackagesDir()

	// Stage a legacy package-style install under skills/.
	oldDir := filepath.Join(storeDir, "gstack")
	if err := os.MkdirAll(filepath.Join(oldDir, "browse"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "SKILL.md"), []byte("# root\n"), 0o644); err != nil {
		t.Fatalf("write root SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "browse", "SKILL.md"), []byte("# browse\n"), 0o644); err != nil {
		t.Fatalf("write nested SKILL.md: %v", err)
	}

	// Fake tool-side projection that should be cleaned up.
	toolProj := filepath.Join(home, ".claude", "skills", "gstack")
	if err := os.MkdirAll(filepath.Dir(toolProj), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(oldDir, toolProj); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Stage a plain skill install — must not be touched.
	plainSkillDir := filepath.Join(storeDir, "brand-guidelines")
	if err := os.MkdirAll(plainSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll plain: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plainSkillDir, "SKILL.md"), []byte("# brand\n"), 0o644); err != nil {
		t.Fatalf("write plain SKILL.md: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.Installed["gstack"] = state.InstalledSkill{
		Revision:     1,
		Tools:        []string{"claude"},
		Paths:        []string{toolProj},
		ManagedPaths: []string{toolProj},
	}
	st.Installed["brand-guidelines"] = state.InstalledSkill{
		Revision: 1,
		Tools:    []string{"claude"},
	}

	var events []any
	syncer := &sync.Syncer{Emit: func(msg any) { events = append(events, msg) }}
	if err := syncer.ReclassifyLegacyPackages(st); err != nil {
		t.Fatalf("ReclassifyLegacyPackages: %v", err)
	}

	// gstack should now live in packages/.
	newDir := filepath.Join(pkgsDir, "gstack")
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("package dir not moved: %v", err)
	}
	if _, err := os.Stat(oldDir); err == nil {
		t.Error("old skills/ dir should be gone")
	}

	// Tool-side projection cleaned up.
	if _, err := os.Lstat(toolProj); !os.IsNotExist(err) {
		t.Errorf("tool projection not cleaned: %v", err)
	}

	// State reflects the reclassification.
	gstack := st.Installed["gstack"]
	if gstack.Kind != state.KindPackage {
		t.Errorf("gstack kind = %q, want %q", gstack.Kind, state.KindPackage)
	}
	if len(gstack.Tools) != 0 || len(gstack.Paths) != 0 {
		t.Errorf("expected empty Tools/Paths after reclassification; got %+v", gstack)
	}

	// Plain skill untouched.
	brand := st.Installed["brand-guidelines"]
	if brand.Kind != state.KindSkill {
		t.Errorf("plain skill kind changed: %q", brand.Kind)
	}
	if _, err := os.Stat(plainSkillDir); err != nil {
		t.Errorf("plain skill dir removed: %v", err)
	}

	// Reclassification event emitted.
	var sawReclassified bool
	for _, ev := range events {
		if msg, ok := ev.(sync.PackageReclassifiedMsg); ok {
			sawReclassified = true
			if msg.Name != "gstack" {
				t.Errorf("wrong name in reclassify event: %q", msg.Name)
			}
		}
	}
	if !sawReclassified {
		t.Error("expected PackageReclassifiedMsg")
	}

	// Second call must be a no-op (migration marker prevents re-run).
	events = nil
	if err := syncer.ReclassifyLegacyPackages(st); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("second call should be a no-op; got %d events", len(events))
	}
}
