package sync_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// TestApply_TreePackage_DetectsAndInstalls verifies that a fetched repo with
// nested SKILL.md plus a ./setup script gets routed to ~/.scribe/packages/,
// the install script runs, and state records Kind=package with empty tools.
func TestApply_TreePackage_DetectsAndInstalls(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{}
	var events []any

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# root\n")},
				{Path: "browse/SKILL.md", Content: []byte("# browse\n")},
				{Path: "setup", Content: []byte("#!/usr/bin/env sh\necho installed\n")},
			},
		},
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "gstack",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:   "gstack",
			Source: "github:Naoray/gstack@main",
		},
	}}

	if err := syncer.RunWithDiff(context.Background(), "Naoray/gstack", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	pkgsDir, err := paths.PackagesDir()
	if err != nil {
		t.Fatalf("PackagesDir: %v", err)
	}
	pkgDir := filepath.Join(pkgsDir, "gstack")
	if _, err := os.Stat(pkgDir); err != nil {
		t.Fatalf("package dir not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pkgDir, "browse", "SKILL.md")); err != nil {
		t.Errorf("nested SKILL.md missing: %v", err)
	}

	// The install command should have been executed exactly once.
	if len(executor.commands) != 1 {
		t.Fatalf("expected 1 install command, got %d: %v", len(executor.commands), executor.commands)
	}

	// State should carry Kind=package with empty tool projections.
	installed, ok := st.Installed["gstack"]
	if !ok {
		t.Fatal("gstack not recorded in state")
	}
	if installed.Kind != state.KindPackage {
		t.Errorf("kind = %q, want %q", installed.Kind, state.KindPackage)
	}
	if len(installed.Tools) != 0 {
		t.Errorf("tools should be empty, got %v", installed.Tools)
	}
	if len(installed.Paths) != 0 {
		t.Errorf("paths should be empty, got %v", installed.Paths)
	}

	// Skills store must remain empty for this name — we didn't project.
	storeDir, _ := paths.StoreDir()
	if _, err := os.Stat(filepath.Join(storeDir, "gstack")); err == nil {
		t.Error("packages must not also land in ~/.scribe/skills/")
	}

	// Detection + install events should have fired in order.
	var sawDetected, sawInstalled bool
	for _, ev := range events {
		switch ev.(type) {
		case sync.PackageDetectedMsg:
			sawDetected = true
		case sync.PackageInstalledMsg:
			sawInstalled = true
		}
	}
	if !sawDetected {
		t.Error("expected PackageDetectedMsg")
	}
	if !sawInstalled {
		t.Error("expected PackageInstalledMsg")
	}
}

// TestApply_TreePackage_RollsBackOnInstallFailure verifies that a non-zero
// exit from the install command deletes the package dir so the machine
// doesn't end up with a half-extracted tree masquerading as installed.
func TestApply_TreePackage_RollsBackOnInstallFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{
		stderr: "boom",
		err:    fmt.Errorf("exit status 1"),
	}
	var events []any

	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# root\n")},
				{Path: "nested/SKILL.md", Content: []byte("# nested\n")},
				{Path: "install.sh", Content: []byte("#!/usr/bin/env sh\nexit 1\n")},
			},
		},
		Executor: executor,
		Emit:     func(msg any) { events = append(events, msg) },
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "crashy",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:   "crashy",
			Source: "github:ex/crashy@main",
		},
	}}

	if err := syncer.RunWithDiff(context.Background(), "ex/crashy", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	pkgsDir, _ := paths.PackagesDir()
	if _, err := os.Stat(filepath.Join(pkgsDir, "crashy")); err == nil {
		t.Error("package dir should be rolled back on install failure")
	}
	if _, ok := st.Installed["crashy"]; ok {
		t.Error("failed package should not be recorded in state")
	}

	var sawError bool
	for _, ev := range events {
		if _, ok := ev.(sync.PackageErrorMsg); ok {
			sawError = true
		}
	}
	if !sawError {
		t.Error("expected PackageErrorMsg event")
	}
}

// TestApply_TreePackage_NoInstallCommand_StillTracked verifies that a repo
// with nested SKILL.md but no install script is still stored as a package
// and recorded — but no command is executed.
func TestApply_TreePackage_NoInstallCommand_StillTracked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	executor := &mockExecutor{}
	syncer := &sync.Syncer{
		Client: &syncTestFetcher{
			files: []tools.SkillFile{
				{Path: "SKILL.md", Content: []byte("# root\n")},
				{Path: "nested/SKILL.md", Content: []byte("# nested\n")},
			},
		},
		Executor: executor,
		Emit:     func(msg any) {},
	}

	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	statuses := []sync.SkillStatus{{
		Name:   "bundle",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:   "bundle",
			Source: "github:ex/bundle@main",
		},
	}}

	if err := syncer.RunWithDiff(context.Background(), "ex/bundle", statuses, st); err != nil {
		t.Fatalf("RunWithDiff: %v", err)
	}

	if len(executor.commands) != 0 {
		t.Errorf("no install command should have fired, got %v", executor.commands)
	}
	installed, ok := st.Installed["bundle"]
	if !ok {
		t.Fatal("bundle not recorded")
	}
	if installed.Kind != state.KindPackage {
		t.Errorf("kind = %q, want package", installed.Kind)
	}
}
