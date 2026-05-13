package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/app"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	clischema "github.com/Naoray/scribe/internal/cli/schema"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/state"
)

func TestProjectInitCreatesProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newProjectInitCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if len(pf.Kits) != 0 {
		t.Fatalf("kits = %#v, want empty", pf.Kits)
	}
	if !strings.Contains(stdout.String(), "Initialized .scribe.yaml") {
		t.Fatalf("stdout = %q, want initialized message", stdout.String())
	}
}

func TestProjectInitForceOverwritesExistingProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := projectfile.Save(filepath.Join(dir, projectfile.Filename), &projectfile.ProjectFile{Kits: []string{"old"}}); err != nil {
		t.Fatalf("write existing project file: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--force", "--kits", "new"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --force: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "new" {
		t.Fatalf("kits = %q, want new", got)
	}
}

func TestProjectInitConflictsWhenProjectFileExistsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := projectfile.Save(filepath.Join(dir, projectfile.Filename), &projectfile.ProjectFile{Kits: []string{"old"}}); err != nil {
		t.Fatalf("write existing project file: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error for existing project file")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitConflict {
		t.Fatalf("exit code = %d, want %d; err=%v", got, clierrors.ExitConflict, err)
	}
}

func TestProjectInitDoesNotGitignoreProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	cmd := newProjectInitCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore exists after project init: %v", err)
	}
	if strings.Contains(stdout.String(), ".gitignore") {
		t.Fatalf("stdout = %q, want no gitignore message", stdout.String())
	}
}

func TestProjectInitLeavesExistingGitignoreAlone(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.scribe.yaml\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(data); got != "node_modules\n.scribe.yaml\n" {
		t.Fatalf(".gitignore = %q, want unchanged", got)
	}
}

func TestProjectInitJSONEnvelope(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"project", "init", "--json"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --json: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var data struct {
		Kits        []string `json:"kits"`
		ProjectFile string   `json:"project_file"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.ProjectFile != ".scribe.yaml" {
		t.Fatalf("project_file = %q, want .scribe.yaml", data.ProjectFile)
	}
	if data.Kits == nil {
		t.Fatal("kits = nil, want empty array")
	}
}

func TestProjectInitKitsFlagUsesProvidedValues(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".scribe", "kits"), 0o755); err != nil {
		t.Fatalf("mkdir kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".scribe", "kits", "go.yaml"), []byte("name: go\nskills: []\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "go,unknown"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --kits: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "go,unknown" {
		t.Fatalf("kits = %q, want go,unknown", got)
	}
	if !strings.Contains(stderr.String(), "warning: unknown kit unknown") {
		t.Fatalf("stderr = %q, want unknown kit warning", stderr.String())
	}
}

func TestProjectInitInstallsRemoteKitFromKitsFlag(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	st := stateFixture(t, home)
	oldFactory := commandFactory
	oldList := listRemoteKitsFn
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		listRemoteKitsFn = oldList
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			State: func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) {
				return gh.NewClient(context.Background(), ""), nil
			},
		}
	}
	listRemoteKitsFn = func(_ context.Context, _ registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		return []registry.ManifestKit{{Registry: repo, Name: "daily-workflow", Path: "kits/daily-workflow.yaml"}}, nil
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name, Path: "kits/" + name + ".yaml"}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"plan-my-day"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc123", nil }
	depsCalled := false
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, _ map[string][]kitInstallDep, _ bool, _ bool) error {
		depsCalled = true
		return nil
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "acme/skills:daily-workflow"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --kits acme/skills:daily-workflow: %v", err)
	}

	if !depsCalled {
		t.Fatal("expected remote kit install to call dep workflow")
	}
	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "daily-workflow" {
		t.Fatalf("kits = %q, want daily-workflow", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")); err != nil {
		t.Fatalf("installed kit missing: %v", err)
	}
}

func TestProjectInitWarnsOnUnknownRemoteKit(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	oldFactory := commandFactory
	t.Cleanup(func() { commandFactory = oldFactory })
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) { return &config.Config{}, nil },
		}
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "ghost/skills:nope"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: unknown remote kit ghost/skills:nope") {
		t.Fatalf("stderr = %q, want unknown remote kit warning", stderr.String())
	}
	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if len(pf.Kits) != 0 {
		t.Fatalf("kits = %#v, want empty (unknown remote dropped)", pf.Kits)
	}
}

func TestProjectInitRejectsLocalKitFromDifferentRegistry(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Local kit sourced from other/skills — should NOT satisfy a request for
	// acme/skills:daily-workflow even though the local name matches.
	if err := os.MkdirAll(filepath.Join(home, ".scribe", "kits"), 0o755); err != nil {
		t.Fatalf("mkdir kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml"), []byte("name: daily-workflow\nsource:\n  registry: other/skills\nskills: []\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	withProjectInitWorkingDir(t, dir)

	oldFactory := commandFactory
	t.Cleanup(func() { commandFactory = oldFactory })
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) { return &config.Config{}, nil },
		}
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "acme/skills:daily-workflow"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: unknown remote kit acme/skills:daily-workflow") {
		t.Fatalf("stderr = %q, want unknown remote kit warning (registry mismatch must not silently resolve)", stderr.String())
	}
	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if len(pf.Kits) != 0 {
		t.Fatalf("kits = %#v, want empty (registry-mismatch local kit must not resolve)", pf.Kits)
	}
}

func TestProjectInitResolvesLocalKitFromMatchingRegistry(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".scribe", "kits"), 0o755); err != nil {
		t.Fatalf("mkdir kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml"), []byte("name: daily-workflow\nsource:\n  registry: acme/skills\nskills: []\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	withProjectInitWorkingDir(t, dir)

	oldFactory := commandFactory
	t.Cleanup(func() { commandFactory = oldFactory })
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) { return &config.Config{}, nil },
		}
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "acme/skills:daily-workflow"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}
	if strings.Contains(stderr.String(), "warning: unknown remote kit") {
		t.Fatalf("stderr = %q, want no warning (matching-registry local kit should resolve cleanly)", stderr.String())
	}
	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "daily-workflow" {
		t.Fatalf("kits = %q, want daily-workflow", got)
	}
}

func TestProjectInitWritesScribeYamlBeforeRemoteInstall(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	st := stateFixture(t, home)
	oldFactory := commandFactory
	oldList := listRemoteKitsFn
	oldFind := findRemoteKitFn
	oldFetch := fetchRemoteKitFn
	oldRev := remoteKitRevFn
	oldDeps := runKitInstallDepsFn
	t.Cleanup(func() {
		commandFactory = oldFactory
		listRemoteKitsFn = oldList
		findRemoteKitFn = oldFind
		fetchRemoteKitFn = oldFetch
		remoteKitRevFn = oldRev
		runKitInstallDepsFn = oldDeps
	})
	commandFactory = func() *app.Factory {
		return &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}, nil
			},
			State:  func() (*state.State, error) { return st, nil },
			Client: func() (*gh.Client, error) { return gh.NewClient(context.Background(), ""), nil },
		}
	}
	listRemoteKitsFn = func(_ context.Context, _ registry.FileFetcher, repo string) ([]registry.ManifestKit, error) {
		return []registry.ManifestKit{{Registry: repo, Name: "remote-kit", Path: "kits/remote-kit.yaml"}}, nil
	}
	findRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, _, name string) (manifest.KitEntry, error) {
		return manifest.KitEntry{Name: name, Path: "kits/" + name + ".yaml"}, nil
	}
	fetchRemoteKitFn = func(_ context.Context, _ registry.FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
		return &kit.Kit{Name: entry.Name, Skills: []string{"x"}, Source: &kit.Source{Registry: registryRepo}}, nil
	}
	remoteKitRevFn = func(context.Context, *gh.Client, string) (string, error) { return "abc", nil }
	// Simulate install failure mid-flow.
	runKitInstallDepsFn = func(_ *cobra.Command, _ *app.Factory, _ map[string][]kitInstallDep, _ bool, _ bool) error {
		return fmt.Errorf("simulated install failure")
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "acme/skills:remote-kit"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute project init: expected error from install failure, got nil")
	}
	// Critical: .scribe.yaml must already exist with the selected kit, so the
	// user can recover by rerunning `scribe kit install` or `project init --force`.
	pf, perr := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if perr != nil {
		t.Fatalf("load project file after install failure: %v — .scribe.yaml must be written before remote install runs", perr)
	}
	if got := strings.Join(pf.Kits, ","); got != "remote-kit" {
		t.Fatalf("kits = %q, want remote-kit (project file must persist the selection through install failures)", got)
	}
}

func TestProjectInitOutputSchemaRegistered(t *testing.T) {
	if _, ok := clischema.Get("scribe project init"); !ok {
		t.Fatal("missing output schema for scribe project init")
	}
}

func withProjectInitWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
