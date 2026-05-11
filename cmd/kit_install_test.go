package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/app"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/state"
)

func TestKitInstallWritesKitStateAndInstallsSameRegistryDeps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})

	st := stateFixture(t, home)
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{
					{Repo: "acme/skills", Enabled: true},
					{Repo: "other/skills", Enabled: true},
				}}, nil
			},
			State: func() (*state.State, error) {
				return st, nil
			},
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name, Path: "kits/" + name + ".yaml"}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{
			Name:        entry.Name,
			Description: "Baseline",
			Skills:      []string{"tdd", "other/skills:debugging"},
			Source:      &kit.Source{Registry: registryRepo},
		}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) {
		return "abc123", nil
	}
	var gotDeps map[string][]kitInstallDep
	var gotForceBudget bool
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, depsByRegistry map[string][]kitInstallDep, forceBudget bool) error {
		gotDeps = depsByRegistry
		gotForceBudget = forceBudget
		return nil
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline", "--json", "--force"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit install: %v", err)
	}
	wantDeps := map[string][]kitInstallDep{
		"acme/skills":  {{Name: "tdd"}},
		"other/skills": {{Name: "debugging"}},
	}
	if !reflect.DeepEqual(gotDeps, wantDeps) {
		t.Fatalf("deps = %#v, want %#v", gotDeps, wantDeps)
	}
	if !gotForceBudget {
		t.Fatal("expected kit install --force to pass through to dependency budget checks")
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "kits", "baseline.yaml")); err != nil {
		t.Fatalf("installed kit missing: %v", err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	installed := loaded.Kits["baseline"]
	if installed.SourceRegistry != "acme/skills" || installed.Rev != "abc123" || installed.ContentHash == "" {
		t.Fatalf("installed kit state = %+v", installed)
	}

	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	var data struct {
		MissingRefs []struct {
			Raw    string `json:"raw"`
			Reason string `json:"reason"`
		} `json:"missing_refs"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.MissingRefs) != 0 {
		t.Fatalf("missing refs = %#v", data.MissingRefs)
	}
}

func TestKitInstallMissingRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	t.Cleanup(func() { commandFactory = oldFactory })
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) { return &config.Config{}, nil },
		}
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitNotFound {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitNotFound, err)
	}
}

func TestKitInstallMissingKit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
	})
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	findRemoteKitFn = func(context.Context, registry.FileFetcher, string, string) (manifest.KitEntry, error) {
		return manifest.KitEntry{}, os.ErrNotExist
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitNotFound {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitNotFound, err)
	}
}

func TestKitInstallNoInteractionReturnsMissingRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
	})
	st := stateFixture(t, home)
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"other/skills:debugging"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc123", nil }

	cmd := newKitInstallCommand()
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"acme/skills:baseline", "--json", "--no-interaction"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected partial error")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitPartial {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitPartial, err)
	}
	var env testEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	var data struct {
		MissingRegistries []string `json:"missing_registries"`
		MissingRefs       []struct {
			Raw    string `json:"raw"`
			Reason string `json:"reason"`
		} `json:"missing_refs"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if !reflect.DeepEqual(data.MissingRegistries, []string{"other/skills"}) {
		t.Fatalf("missing registries = %#v", data.MissingRegistries)
	}
	if len(data.MissingRefs) != 1 || data.MissingRefs[0].Reason != "registry_not_connected" {
		t.Fatalf("missing refs = %#v", data.MissingRefs)
	}
}

func TestKitInstallConnectPromptInstallsCrossRegistryRefs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	oldConfirm := confirmKitMissingRegistriesFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
		confirmKitMissingRegistriesFn = oldConfirm
	})
	st := stateFixture(t, home)
	cfg := &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) { return cfg, nil },
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"other/skills:debugging"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc123", nil }
	confirmKitMissingRegistriesFn = func(_ *cobra.Command, cfg *config.Config, registries []string) error {
		if !reflect.DeepEqual(registries, []string{"other/skills"}) {
			t.Fatalf("registries = %#v", registries)
		}
		cfg.AddRegistry(config.RegistryConfig{Repo: "other/skills", Enabled: true})
		return nil
	}
	var gotDeps map[string][]kitInstallDep
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, depsByRegistry map[string][]kitInstallDep, _ bool) error {
		gotDeps = depsByRegistry
		return nil
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit install: %v", err)
	}
	want := map[string][]kitInstallDep{"other/skills": {{Name: "debugging"}}}
	if !reflect.DeepEqual(gotDeps, want) {
		t.Fatalf("deps = %#v, want %#v", gotDeps, want)
	}
}

func TestKitInstallAliasMappingPassesSkillAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	st := stateFixture(t, home)
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{
					{Repo: "acme/skills", Enabled: true},
					{Repo: "other/skills", Enabled: true},
				}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{
			Name:         entry.Name,
			Skills:       []string{"other/skills:tdd"},
			SkillAliases: map[string]string{"other/skills:tdd": "other-tdd"},
			Source:       &kit.Source{Registry: registryRepo},
		}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc123", nil }
	var gotDeps map[string][]kitInstallDep
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, depsByRegistry map[string][]kitInstallDep, _ bool) error {
		gotDeps = depsByRegistry
		return nil
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit install: %v", err)
	}
	want := map[string][]kitInstallDep{"other/skills": {{Name: "tdd", Alias: "other-tdd"}}}
	if !reflect.DeepEqual(gotDeps, want) {
		t.Fatalf("deps = %#v, want %#v", gotDeps, want)
	}
}

func TestKitInstallPinnedGitHubRefPassesPinnedSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	st := stateFixture(t, home)
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{
					{Repo: "acme/skills", Enabled: true},
					{Repo: "other/skills", Enabled: true},
				}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"github:other/skills@v1.2.3:tdd"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc123", nil }
	var gotDeps map[string][]kitInstallDep
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, depsByRegistry map[string][]kitInstallDep, _ bool) error {
		gotDeps = depsByRegistry
		return nil
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit install: %v", err)
	}
	want := map[string][]kitInstallDep{"other/skills": {{Name: "tdd", Source: "github:other/skills@v1.2.3"}}}
	if !reflect.DeepEqual(gotDeps, want) {
		t.Fatalf("deps = %#v, want %#v", gotDeps, want)
	}
}

func TestKitSyncRefreshesInstalledRegistryKit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	st := stateFixture(t, home)
	st.Kits["baseline"] = state.InstalledKit{Name: "baseline", SourceRegistry: "acme/skills", Rev: "old"}
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"tdd"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "new", nil }
	var gotDeps map[string][]kitInstallDep
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, depsByRegistry map[string][]kitInstallDep, _ bool) error {
		gotDeps = depsByRegistry
		return nil
	}

	cmd := newKitSyncCommand()
	cmd.SetArgs([]string{"--json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit sync: %v", err)
	}
	if st.Kits["baseline"].Rev != "new" || st.Kits["baseline"].ContentHash == "" {
		t.Fatalf("kit state = %+v", st.Kits["baseline"])
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "kits", "baseline.yaml")); err != nil {
		t.Fatalf("synced kit missing: %v", err)
	}
	want := map[string][]kitInstallDep{"acme/skills": {{Name: "tdd"}}}
	if !reflect.DeepEqual(gotDeps, want) {
		t.Fatalf("deps = %#v, want %#v", gotDeps, want)
	}
}

func TestKitSyncRefusesToOverwriteLocallyEditedKit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldFactory := commandFactory
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	st := stateFixture(t, home)
	kitDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitDir, 0o755); err != nil {
		t.Fatalf("mkdir kit dir: %v", err)
	}
	kitPath := filepath.Join(kitDir, "baseline.yaml")
	original := []byte("name: baseline\nskills:\n  - tdd\n")
	if err := os.WriteFile(kitPath, original, 0o644); err != nil {
		t.Fatalf("write original kit: %v", err)
	}
	hash, err := hashKitFile("baseline", kitPath)
	if err != nil {
		t.Fatalf("hash original kit: %v", err)
	}
	st.Kits["baseline"] = state.InstalledKit{Name: "baseline", SourceRegistry: "acme/skills", Rev: "old", ContentHash: hash}
	edited := []byte("name: baseline\nskills:\n  - tdd\n  - local-edit\n")
	if err := os.WriteFile(kitPath, edited, 0o644); err != nil {
		t.Fatalf("write edited kit: %v", err)
	}

	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"tdd", "upstream"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "new", nil }
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, _ map[string][]kitInstallDep, _ bool) error {
		t.Fatal("dependencies should not install after local kit conflict")
		return nil
	}

	cmd := newKitSyncCommand()
	cmd.SetArgs([]string{"--json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected local edit conflict")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitConflict {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitConflict, err)
	}
	if !strings.Contains(err.Error(), "local edits") {
		t.Fatalf("error = %v, want local edits message", err)
	}
	got, err := os.ReadFile(kitPath)
	if err != nil {
		t.Fatalf("read kit path: %v", err)
	}
	if string(got) != string(edited) {
		t.Fatalf("kit file was overwritten:\n%s", got)
	}
	if st.Kits["baseline"].Rev != "old" {
		t.Fatalf("state rev = %q, want old", st.Kits["baseline"].Rev)
	}
}
