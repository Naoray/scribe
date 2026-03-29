package ui_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/ui"
)

func TestSyncProgress_ResolvedAddsRow(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	updated, _ := m.Update(sync.SkillResolvedMsg{
		SkillStatus: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing},
	})
	model := updated.(ui.SyncProgress)

	if len(model.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(model.Skills))
	}
	if model.Skills[0].Name != "cleanup" {
		t.Errorf("expected name cleanup, got %s", model.Skills[0].Name)
	}
}

func TestSyncProgress_InstalledUpdatesRow(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	updated, _ := m.Update(sync.SkillResolvedMsg{
		SkillStatus: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing},
	})
	m2 := updated.(ui.SyncProgress)

	updated, _ = m2.Update(sync.SkillInstalledMsg{Name: "cleanup", Version: "v2.1.0"})
	model := updated.(ui.SyncProgress)

	if model.Skills[0].State != ui.SkillInstalled {
		t.Errorf("expected state Installed, got %v", model.Skills[0].State)
	}
	if model.Skills[0].Version != "v2.1.0" {
		t.Errorf("expected version v2.1.0, got %s", model.Skills[0].Version)
	}
}

func TestSyncProgress_CompleteQuits(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	_, cmd := m.Update(sync.SyncCompleteMsg{Installed: 3, Skipped: 2})
	if cmd == nil {
		t.Fatal("expected quit command on SyncCompleteMsg")
	}
}
