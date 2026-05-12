package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type registryResyncProvider struct{}

func (registryResyncProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{
		IsTeam: true,
		Entries: []manifest.Entry{{
			Name:   "plan-my-day",
			Source: "github:Naoray/skills@main",
		}},
		Kits: []provider.KitFile{{
			Name: "daily-workflow",
			Path: "kits/daily-workflow.yaml",
			Body: []byte("apiVersion: scribe/v1\nkind: Kit\nname: daily-workflow\nskills: [plan-my-day]\n"),
			Ref:  "abc123",
		}},
	}, nil
}

func (registryResyncProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

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
	st.RemovedByUser = []state.RemovedSkill{
		{Name: "recap", Registry: "acme/skills"},
		{Name: "recap", Registry: "other/skills"},
	}
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
	if loadedState.IsRemovedByUser("acme/skills", "recap") {
		t.Fatal("acme/skills deny-list entries should be cleared")
	}
	if !loadedState.IsRemovedByUser("other/skills", "recap") {
		t.Fatal("other/skills deny-list entry should remain")
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

	cmd := newRegistryResyncCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := resyncRegistry(cmd, "acme/skills"); err != nil {
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
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".scribe", "kits")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("legacy resync should not create kits dir; stat err=%v", err)
	}
	if !strings.Contains(errOut.String(), "will refresh kits by default") {
		t.Fatalf("expected deprecation banner, got %q", errOut.String())
	}
	if !strings.Contains(out.String(), "will be retried") {
		t.Fatalf("expected legacy stdout, got %q", out.String())
	}
}

func TestResyncRegistryRefreshKitsRunsFullPipeline(t *testing.T) {
	home := setupResyncRegistryHome(t)
	restore := installResyncProviderFactory(t)
	defer restore()

	cmd := newRegistryResyncCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("refresh-kits", "true"); err != nil {
		t.Fatal(err)
	}
	if err := resyncRegistry(cmd, "Naoray/skills"); err != nil {
		t.Fatalf("resyncRegistry(): %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")); err != nil {
		t.Fatalf("kit file missing: %v", err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	if got := loaded.Kits["daily-workflow"]; got.Source != "Naoray/skills" || got.Version != "abc123" {
		t.Fatalf("state kit stamp = %+v", got)
	}
}

func TestResyncRegistryDeprecationBannerSuppressedInJSON(t *testing.T) {
	setupResyncRegistryHome(t)
	cmd := newRegistryResyncCommand()
	cmd.Flags().Bool("json", false, "test json flag")
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	if err := resyncRegistry(cmd, "Naoray/skills"); err != nil {
		t.Fatalf("resyncRegistry(): %v", err)
	}
	if strings.Contains(errOut.String(), "will refresh kits by default") {
		t.Fatalf("banner should be suppressed in JSON mode: %q", errOut.String())
	}
}

func TestResyncRefreshKitsPersistsState(t *testing.T) {
	home := setupResyncRegistryHome(t)
	restore := installResyncProviderFactory(t)
	defer restore()

	cmd := newRegistryResyncCommand()
	cmd.Flags().Bool("json", false, "test json flag")
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("refresh-kits", "true"); err != nil {
		t.Fatal(err)
	}
	if err := resyncRegistry(cmd, "Naoray/skills"); err != nil {
		t.Fatalf("resyncRegistry(): %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".scribe", "state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state.State
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("unmarshal state: %v\n%s", err, string(raw))
	}
	if got := st.Kits["daily-workflow"]; got.Source != "Naoray/skills" || len(got.Skills) != 1 {
		t.Fatalf("persisted state kit stamp = %+v", got)
	}
}

func setupResyncRegistryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := &config.Config{
		Registries: []config.RegistryConfig{{Repo: "Naoray/skills", Enabled: true}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save(): %v", err)
	}
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load(): %v", err)
	}
	st.RecordRegistryFailure("Naoray/skills", nil, 1)
	if err := st.Save(); err != nil {
		t.Fatalf("st.Save(): %v", err)
	}
	return home
}

func installResyncProviderFactory(t *testing.T) func() {
	t.Helper()
	old := commandFactory
	commandFactory = func() *app.Factory {
		f := app.NewFactory()
		f.Client = func() (*gh.Client, error) { return nil, nil }
		f.Provider = func() (provider.Provider, error) { return registryResyncProvider{}, nil }
		return f
	}
	return func() { commandFactory = old }
}
