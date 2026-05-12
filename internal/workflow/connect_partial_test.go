package workflow

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type partialKitProvider struct {
	result *provider.DiscoverResult
}

func (p partialKitProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	return p.result, nil
}

func (p partialKitProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	return nil, errors.New("unused")
}

func TestConnectKitPartialFailureWarnsInTTY(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	restoreTTY := forceWorkflowTTY(t, true)
	defer restoreTTY()

	var out, errOut bytes.Buffer
	bag := &Bag{
		RepoArg:   "acme/skills",
		Config:    &config.Config{},
		State:     newPartialKitState(),
		Formatter: NewFormatterWithWriters(false, false, &out, &errOut),
		Provider:  partialKitProvider{result: partialKitDiscoverResult()},
	}

	if err := StepFetchManifest(context.Background(), bag); err != nil {
		t.Fatalf("StepFetchManifest: %v", err)
	}
	if !bag.Partial {
		t.Fatal("Partial = false, want true")
	}
	if got := errOut.String(); !strings.Contains(got, "warning: kit broken-kit skipped: fetch failed") {
		t.Fatalf("kit warning missing from formatter output: %q", got)
	}
	if err := StepInstallKits(context.Background(), bag); err != nil {
		t.Fatalf("StepInstallKits: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")); err != nil {
		t.Fatalf("expected kit written to disk: %v", err)
	}
}

func TestConnectKitPartialFailureAbortsInJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	restoreTTY := forceWorkflowTTY(t, true)
	defer restoreTTY()

	bag := &Bag{
		RepoArg:   "acme/skills",
		JSONFlag:  true,
		Config:    &config.Config{},
		State:     newPartialKitState(),
		Formatter: NewFormatterWithWriters(true, false, &bytes.Buffer{}, &bytes.Buffer{}),
		Provider:  partialKitProvider{result: partialKitDiscoverResult()},
	}

	err := StepFetchManifest(context.Background(), bag)
	if err == nil {
		t.Fatal("StepFetchManifest error = nil, want abort")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitGeneral {
		t.Fatalf("ExitCode = %d, want %d; err=%v", got, clierrors.ExitGeneral, err)
	}
	if bag.Partial {
		t.Fatal("Partial = true, want false on abort")
	}
	if bag.StateDirty {
		t.Fatal("StateDirty = true, want false")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("abort touched kit disk path; stat err = %v", statErr)
	}
	if len(bag.State.Kits) != 0 {
		t.Fatalf("abort stamped kit state: %+v", bag.State.Kits)
	}
}

func forceWorkflowTTY(t *testing.T, tty bool) func() {
	t.Helper()
	orig := conflictModeIsTerminal
	conflictModeIsTerminal = func(uintptr) bool { return tty }
	return func() { conflictModeIsTerminal = orig }
}

func partialKitDiscoverResult() *provider.DiscoverResult {
	return &provider.DiscoverResult{
		Entries: []manifest.Entry{{Name: "deploy", Source: "github:acme/skills@main", Path: "skills/deploy"}},
		Kits: []provider.KitFile{{
			Name: "daily-workflow",
			Path: "kits/daily-workflow.yaml",
			Body: []byte("apiVersion: scribe/v1\nkind: Kit\nname: daily-workflow\nskills: [deploy]\n"),
			Ref:  "abc123",
		}},
		KitErrors: provider.KitFetchErrors{{
			Name: "broken-kit",
			Path: "kits/broken-kit.yaml",
			Err:  errors.New("fetch failed"),
		}},
	}
}

func newPartialKitState() *state.State {
	return &state.State{
		SchemaVersion: state.CurrentSchemaVersion,
		Installed:     map[string]state.InstalledSkill{},
		Kits:          map[string]state.InstalledKit{},
	}
}
