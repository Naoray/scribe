package cmd

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

func TestClearRemovedBeforeInstallClearsAllRegistriesForName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.RemovedByUser = []state.RemovedSkill{
		{Name: "recap", Registry: "acme/skills"},
		{Name: "recap", Registry: "other/skills"},
		{Name: "deploy", Registry: "acme/skills"},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := clearRemovedBeforeInstall(newCommandFactory(), []string{"recap"}, ""); err != nil {
		t.Fatalf("clearRemovedBeforeInstall: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded.IsRemovedByUser("acme/skills", "recap") || loaded.IsRemovedByUser("other/skills", "recap") {
		t.Fatal("recap deny-list entries should be cleared across registries")
	}
	if !loaded.IsRemovedByUser("acme/skills", "deploy") {
		t.Fatal("deploy deny-list entry should remain")
	}
}

func TestClearRemovedBeforeInstallScopesRegistryFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/skills", Enabled: true},
			{Repo: "other/skills", Enabled: true},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.RemovedByUser = []state.RemovedSkill{
		{Name: "recap", Registry: "acme/skills"},
		{Name: "recap", Registry: "other/skills"},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := clearRemovedBeforeInstall(newCommandFactory(), []string{"recap"}, "acme/skills"); err != nil {
		t.Fatalf("clearRemovedBeforeInstall: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded.IsRemovedByUser("acme/skills", "recap") {
		t.Fatal("acme/skills recap entry should be cleared")
	}
	if !loaded.IsRemovedByUser("other/skills", "recap") {
		t.Fatal("other/skills recap entry should remain")
	}
}
