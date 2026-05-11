package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
			Skills:      []string{"tdd", "other/skills:debugging", "missing/skills:qa"},
			Source:      &kit.Source{Registry: registryRepo},
		}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) {
		return "abc123", nil
	}
	var gotRegistry string
	var gotDeps []string
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, registryRepo string, skillNames []string) error {
		gotRegistry = registryRepo
		gotDeps = append([]string(nil), skillNames...)
		return nil
	}

	cmd := newKitInstallCommand()
	cmd.SetArgs([]string{"acme/skills:baseline", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kit install: %v", err)
	}
	if gotRegistry != "acme/skills" || !reflect.DeepEqual(gotDeps, []string{"tdd"}) {
		t.Fatalf("deps registry=%q deps=%v", gotRegistry, gotDeps)
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
	if len(data.MissingRefs) != 2 {
		t.Fatalf("missing refs = %#v", data.MissingRefs)
	}
	if data.MissingRefs[0].Reason != "cross_registry_deferred" || data.MissingRefs[1].Reason != "registry_not_connected" {
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
