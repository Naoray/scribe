package workflow

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type syncRegistryFailureProvider struct {
	calls []string
	errs  map[string]error
}

func (p *syncRegistryFailureProvider) Discover(_ context.Context, repo string) (*provider.DiscoverResult, error) {
	p.calls = append(p.calls, repo)
	if err := p.errs[repo]; err != nil {
		return nil, err
	}
	return &provider.DiscoverResult{
		Entries: []manifest.Entry{{
			Name:   "deploy",
			Source: "github:" + repo + "@main",
			Path:   "skills/deploy",
		}},
	}, nil
}

func (p *syncRegistryFailureProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	return nil, errors.New("unexpected fetch")
}

func TestStepSyncSkillsSkipsEmptyRegistryLoadoutAndContinues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	provider := &syncRegistryFailureProvider{
		errs: map[string]error{
			"acme/empty": errors.New("acme/empty: no skills found (looked for scribe.yaml, scribe.toml, marketplace.json, and SKILL.md files)"),
		},
	}
	var out, errOut bytes.Buffer
	bag := &Bag{
		State:     newSyncRegistryFailureState(),
		Repos:     []string{"acme/empty", "acme/full"},
		Provider:  provider,
		Formatter: NewFormatterWithWriters(false, true, &out, &errOut),
	}

	err := StepSyncSkills(context.Background(), bag)
	if err != nil {
		t.Fatalf("StepSyncSkills error = %v, want nil", err)
	}
	if !slices.Equal(provider.calls, []string{"acme/empty", "acme/full"}) {
		t.Fatalf("Discover calls = %v, want empty registry followed by full registry", provider.calls)
	}
	if failure := bag.State.RegistryFailure("acme/empty"); failure.Consecutive != 1 || !strings.Contains(failure.LastError, "no skills found") {
		t.Fatalf("empty registry failure not recorded: %+v", failure)
	}
	if failure := bag.State.RegistryFailure("acme/full"); failure.Consecutive != 0 {
		t.Fatalf("full registry failure = %+v, want zero", failure)
	}
	if !bag.StateDirty {
		t.Fatal("StateDirty = false, want true after recording registry failure")
	}
	if bag.Partial {
		t.Fatal("Partial = true, want false for skipped empty registry")
	}
}

func TestStepSyncSkillsKeepsNonEmptyRegistryErrorsFatal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	provider := &syncRegistryFailureProvider{
		errs: map[string]error{
			"acme/broken": errors.New("invalid scribe.yaml"),
		},
	}
	bag := &Bag{
		State:     newSyncRegistryFailureState(),
		Repos:     []string{"acme/broken", "acme/full"},
		Provider:  provider,
		Formatter: NewFormatterWithWriters(false, true, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	err := StepSyncSkills(context.Background(), bag)
	if err == nil {
		t.Fatal("StepSyncSkills error = nil, want fatal parse error")
	}
	if !strings.Contains(err.Error(), "invalid scribe.yaml") {
		t.Fatalf("StepSyncSkills error = %v, want invalid manifest error", err)
	}
	if !slices.Equal(provider.calls, []string{"acme/broken"}) {
		t.Fatalf("Discover calls = %v, want fatal error to stop before next registry", provider.calls)
	}
}

func newSyncRegistryFailureState() *state.State {
	return &state.State{
		SchemaVersion: state.CurrentSchemaVersion,
		Installed:     map[string]state.InstalledSkill{},
		Kits:          map[string]state.InstalledKit{},
	}
}
