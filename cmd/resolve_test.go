package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

func TestRunResolve_Ours(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Setup skill directory in the store.
	skillDir := filepath.Join(home, ".scribe", "skills", "cleanup")
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write conflicted SKILL.md.
	conflicted := []byte("<<<<<<< local\nmy stuff\n=======\nupstream stuff\n>>>>>>> upstream\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), conflicted, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write version snapshot (the "ours" side — what we had before merge).
	oursContent := []byte("my stuff\n")
	if err := os.WriteFile(filepath.Join(versionsDir, "rev-1.md"), oursContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write .scribe-base.md (upstream).
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), []byte("upstream stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create state with the skill installed.
	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"cleanup": {
				Revision:      1,
				InstalledHash: sync.ComputeFileHash(conflicted),
				Tools:         []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	// Run resolve --ours.
	cmd := newResolveCommand()
	cmd.SetArgs([]string{"cleanup", "--ours"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify SKILL.md has "ours" content.
	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "my stuff\n" {
		t.Errorf("expected ours content, got %q", string(got))
	}

	// Verify state was updated.
	st2, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	skill := st2.Installed["cleanup"]
	if skill.Revision != 2 {
		t.Errorf("expected revision 2, got %d", skill.Revision)
	}
	if skill.InstalledHash != sync.ComputeFileHash(oursContent) {
		t.Errorf("expected hash of ours content, got %q", skill.InstalledHash)
	}
}

func TestRunResolve_Theirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "cleanup")
	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	conflicted := []byte("<<<<<<< local\nmy stuff\n=======\nupstream stuff\n>>>>>>> upstream\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), conflicted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, "rev-1.md"), []byte("my stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Base stays as the old content (pre-merge) — that's what ThreeWayMerge
	// leaves behind on conflict. Upstream is persisted to the .scribe-theirs.md
	// sidecar.
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), []byte("original stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	theirsContent := []byte("upstream stuff\n")
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-theirs.md"), theirsContent, 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"cleanup": {Revision: 1, InstalledHash: "abc", Tools: []string{"claude"}},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	cmd := newResolveCommand()
	cmd.SetArgs([]string{"cleanup", "--theirs"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "upstream stuff\n" {
		t.Errorf("expected theirs content, got %q", string(got))
	}

	st2, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st2.Installed["cleanup"].Revision != 2 {
		t.Errorf("expected revision 2, got %d", st2.Installed["cleanup"].Revision)
	}

	// Sidecar must be removed after resolution.
	if _, err := os.Stat(filepath.Join(skillDir, ".scribe-theirs.md")); !os.IsNotExist(err) {
		t.Errorf("expected .scribe-theirs.md to be removed, got err=%v", err)
	}

	// Base must have advanced to the resolved (upstream) content so the next
	// 3-way merge starts from a clean baseline.
	gotBase, err := os.ReadFile(filepath.Join(skillDir, ".scribe-base.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBase) != string(theirsContent) {
		t.Errorf("expected base to equal upstream content, got %q", string(gotBase))
	}
}

func TestResolveFlags_MutuallyExclusive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write minimal state so state.Load() doesn't fail.
	stateDir := filepath.Join(home, ".scribe")
	os.MkdirAll(stateDir, 0o755)
	stData, _ := json.Marshal(state.State{SchemaVersion: 2, Installed: map[string]state.InstalledSkill{}})
	os.WriteFile(filepath.Join(stateDir, "state.json"), stData, 0o644)

	// Neither flag.
	cmd := newResolveCommand()
	cmd.SetArgs([]string{"test-skill"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when neither --ours nor --theirs specified")
	}

	// Both flags.
	cmd = newResolveCommand()
	cmd.SetArgs([]string{"test-skill", "--ours", "--theirs"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when both --ours and --theirs specified")
	}
}
