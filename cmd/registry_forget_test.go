package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

func TestForgetRegistryRemovesConfigAndFailureState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/skills", Enabled: true},
			{Repo: "other/skills", Enabled: true},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save(): %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	st.RecordRegistryFailure("acme/skills", nil, 3)
	if err := st.Save(); err != nil {
		t.Fatalf("st.Save(): %v", err)
	}

	if err := forgetRegistry("acme/skills"); err != nil {
		t.Fatalf("forgetRegistry(): %v", err)
	}

	loadedCfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load(): %v", err)
	}
	if loadedCfg.FindRegistry("acme/skills") != nil {
		t.Fatal("acme/skills should have been removed from config")
	}

	loadedState, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	if got := loadedState.RegistryFailure("acme/skills"); got.Consecutive != 0 {
		t.Fatalf("registry failure not cleared: %+v", got)
	}
}

func TestResyncRegistryClearsMuteState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save(): %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	st.RecordRegistryFailure("acme/skills", nil, 1)
	if err := st.Save(); err != nil {
		t.Fatalf("st.Save(): %v", err)
	}

	if err := resyncRegistry("acme/skills"); err != nil {
		t.Fatalf("resyncRegistry(): %v", err)
	}

	loadedState, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	if got := loadedState.RegistryFailure("acme/skills"); got.Consecutive != 0 {
		t.Fatalf("registry failure not cleared: %+v", got)
	}

	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".scribe", "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}
