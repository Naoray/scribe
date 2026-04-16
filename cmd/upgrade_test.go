package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v69/github"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/upgrade"
)

type fakeUpgradeClient struct {
	tag string
	err error
}

func (f fakeUpgradeClient) LatestRelease(context.Context, string, string) (*gogithub.RepositoryRelease, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gogithub.RepositoryRelease{TagName: gogithub.Ptr(f.tag)}, nil
}

func TestRunUpgradeWithDepsRecordsTimestampOnNoOpSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	st := &state.State{
		Installed:          map[string]state.InstalledSkill{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}

	called := 0
	err := runUpgradeWithDeps(context.Background(), st, fakeUpgradeClient{
		tag: "v1.2.3",
	}, upgrade.MethodGoInstall, func(context.Context, upgrade.Method, *gogithub.RepositoryRelease, bool) error {
		called++
		return fmt.Errorf("runner should not be called for no-op success")
	}, false)
	if err != nil {
		t.Fatalf("runUpgradeWithDeps() error = %v", err)
	}
	if called != 0 {
		t.Fatalf("runner called %d times, want 0", called)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after no-op upgrade: %v", err)
	}
	if !loaded.ScribeBinaryUpdateCooldownFresh(time.Now().UTC()) {
		t.Fatal("expected successful no-op upgrade to refresh the scribe cooldown")
	}
}

func TestRunUpgradeWithDepsRecordsTimestampOnSuccessfulUpgrade(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	st := &state.State{
		Installed:          map[string]state.InstalledSkill{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}

	called := 0
	err := runUpgradeWithDeps(context.Background(), st, fakeUpgradeClient{
		tag: "v1.2.4",
	}, upgrade.MethodGoInstall, func(context.Context, upgrade.Method, *gogithub.RepositoryRelease, bool) error {
		called++
		return nil
	}, false)
	if err != nil {
		t.Fatalf("runUpgradeWithDeps() error = %v", err)
	}
	if called != 1 {
		t.Fatalf("runner called %d times, want 1", called)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after successful upgrade: %v", err)
	}
	if !loaded.ScribeBinaryUpdateCooldownFresh(time.Now().UTC()) {
		t.Fatal("expected successful upgrade to refresh the scribe cooldown")
	}
}

func TestRunUpgradeWithDepsDoesNotRecordTimestampOnFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	st := &state.State{
		Installed:          map[string]state.InstalledSkill{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	st.RecordScribeBinaryUpdateSuccessAt(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := st.Save(); err != nil {
		t.Fatalf("Save initial state: %v", err)
	}

	wantErr := errors.New("boom")
	err := runUpgradeWithDeps(context.Background(), st, fakeUpgradeClient{
		tag: "v1.2.4",
	}, upgrade.MethodGoInstall, func(context.Context, upgrade.Method, *gogithub.RepositoryRelease, bool) error {
		return wantErr
	}, false)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runUpgradeWithDeps() error = %v, want %v", err, wantErr)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after failed upgrade: %v", err)
	}
	if loaded.ScribeBinaryUpdateCooldownFresh(time.Now().UTC()) {
		t.Fatal("failed upgrade should not refresh the scribe cooldown")
	}
}

func TestRunUpgradeWithDepsAllowsUnauthenticatedReleaseChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	err := runUpgradeWithDeps(context.Background(), st, fakeUpgradeClient{
		tag: "v1.2.3",
	}, upgrade.MethodGoInstall, func(context.Context, upgrade.Method, *gogithub.RepositoryRelease, bool) error {
		return fmt.Errorf("runner should not be called for no-op success")
	}, false)
	if err != nil {
		t.Fatalf("runUpgradeWithDeps() error = %v, want nil for public release checks", err)
	}
}
