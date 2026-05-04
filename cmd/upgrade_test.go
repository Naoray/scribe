package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v69/github"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
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
	origInstalledBinaryVersion := installedBinaryVersion
	Version = "1.2.3"
	installedBinaryVersion = func(context.Context) (string, error) {
		return "scribe version 1.2.4\n", nil
	}
	t.Cleanup(func() {
		Version = origVersion
		installedBinaryVersion = origInstalledBinaryVersion
	})

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

func TestRunUpgradeWithDepsFailsWhenInstalledBinaryVersionDoesNotMatchRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	binDir := t.TempDir()
	scribePath := filepath.Join(binDir, "scribe")
	if err := os.WriteFile(scribePath, []byte("#!/bin/sh\necho 'scribe version 1.2.3'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	st := &state.State{
		Installed:          map[string]state.InstalledSkill{},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}

	err := runUpgradeWithDeps(context.Background(), st, fakeUpgradeClient{
		tag: "v1.2.4",
	}, upgrade.MethodGoInstall, func(context.Context, upgrade.Method, *gogithub.RepositoryRelease, bool) error {
		return nil
	}, false)
	if err == nil {
		t.Fatal("runUpgradeWithDeps() error = nil, want version mismatch")
	}

	var ce *clierrors.Error
	if !errors.As(err, &ce) {
		t.Fatalf("runUpgradeWithDeps() error = %T, want *clierrors.Error", err)
	}
	if ce.Code != "UPGRADE_VERSION_MISMATCH" {
		t.Fatalf("code = %q, want UPGRADE_VERSION_MISMATCH", ce.Code)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after version mismatch: %v", err)
	}
	if loaded.ScribeBinaryUpdateCooldownFresh(time.Now().UTC()) {
		t.Fatal("version mismatch should not refresh the scribe cooldown")
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

func TestRunUpgradeCheckResolvesGoInstallVersion(t *testing.T) {
	origVersion := Version
	origCurrent := currentVersion
	Version = "dev"
	currentVersion = func() string { return "1.2.3" }
	t.Cleanup(func() {
		Version = origVersion
		currentVersion = origCurrent
	})

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	err := runUpgradeCheckWithDeps(context.Background(), fakeUpgradeClient{tag: "v1.2.3"})

	w.Close()
	os.Stdout = origStdout
	var buf strings.Builder
	io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("runUpgradeCheckWithDeps() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Already up to date") {
		t.Fatalf("dev binary with module-version BuildInfo should compare cleanly; got %q", out)
	}
	if strings.Contains(out, "New version available") {
		t.Fatalf("dev fallback should not report new version against matching tag; got %q", out)
	}
}

func TestRunUpgradeCheckReportsUpToDate(t *testing.T) {
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	var buf strings.Builder
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runUpgradeCheckWithDeps(context.Background(), fakeUpgradeClient{tag: "v1.2.3"})

	w.Close()
	os.Stdout = origStdout
	io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("runUpgradeCheckWithDeps() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Already up to date") {
		t.Fatalf("expected 'Already up to date', got %q", buf.String())
	}
	if strings.Contains(buf.String(), "New version available") {
		t.Fatal("should not report new version when already current")
	}
}

func TestRunUpgradeCheckReportsNewVersion(t *testing.T) {
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	var buf strings.Builder
	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	err := runUpgradeCheckWithDeps(context.Background(), fakeUpgradeClient{tag: "v1.2.4"})

	w.Close()
	os.Stdout = origStdout
	io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("runUpgradeCheckWithDeps() error = %v", err)
	}
	if !strings.Contains(buf.String(), "New version available") {
		t.Fatalf("expected 'New version available', got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "v1.2.4") {
		t.Fatalf("expected latest tag in output, got %q", buf.String())
	}
}

func TestRunUpgradeCheckDoesNotModifyState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	origVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = origVersion })

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	err := runUpgradeCheckWithDeps(context.Background(), fakeUpgradeClient{tag: "v1.2.4"})

	w.Close()
	os.Stdout = origStdout
	r.Close()

	if err != nil {
		t.Fatalf("runUpgradeCheckWithDeps() error = %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ScribeBinaryUpdateCooldownFresh(time.Now().UTC()) {
		t.Fatal("--check should not update the upgrade cooldown in state")
	}
}
