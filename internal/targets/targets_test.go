package targets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/targets"
)

var testFiles = []targets.SkillFile{
	{Path: "SKILL.md", Content: []byte(`---
name: deploy
description: "Artistfy deployment workflow"
license: MIT
---

## Deploy

Run the deploy script.
`)},
	{Path: "scripts/deploy.sh", Content: []byte("#!/bin/sh\necho deploying\n")},
}

func setup(t *testing.T) (canonicalDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir, err := targets.WriteToStore("deploy", testFiles)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}
	return dir
}

func TestWriteToStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir, err := targets.WriteToStore("deploy", testFiles)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	// Source files written
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Error("SKILL.md not in store")
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts", "deploy.sh")); err != nil {
		t.Error("scripts/deploy.sh not in store")
	}
	// .cursor.mdc generated
	if _, err := os.Stat(filepath.Join(dir, ".cursor.mdc")); err != nil {
		t.Error(".cursor.mdc not generated in store")
	}
}

func TestClaudeInstall(t *testing.T) {
	canonicalDir := setup(t)

	target := targets.ClaudeTarget{}
	paths, err := target.Install("deploy", canonicalDir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 symlink path, got %d", len(paths))
	}

	// Link resolves to the canonical dir
	resolved, err := os.Readlink(paths[0])
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if resolved != canonicalDir {
		t.Errorf("symlink points to %q, want %q", resolved, canonicalDir)
	}

	// Files accessible through the symlink
	if _, err := os.Stat(filepath.Join(paths[0], "SKILL.md")); err != nil {
		t.Error("SKILL.md not accessible through claude symlink")
	}
}

func TestClaudeInstallReplaces(t *testing.T) {
	canonicalDir := setup(t)
	target := targets.ClaudeTarget{}

	// Install twice — second should replace the first without error.
	if _, err := target.Install("deploy", canonicalDir); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if _, err := target.Install("deploy", canonicalDir); err != nil {
		t.Fatalf("second Install: %v", err)
	}
}

func TestCursorInstall(t *testing.T) {
	canonicalDir := setup(t)
	workDir := t.TempDir()

	target := targets.CursorTarget{WorkDir: workDir}
	paths, err := target.Install("deploy", canonicalDir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 symlink path, got %d", len(paths))
	}

	// Symlink points into the store
	resolved, err := os.Readlink(paths[0])
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	expectedSrc := filepath.Join(canonicalDir, ".cursor.mdc")
	if resolved != expectedSrc {
		t.Errorf("symlink points to %q, want %q", resolved, expectedSrc)
	}

	// Content accessible through the symlink
	mdc, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read mdc through symlink: %v", err)
	}
	content := string(mdc)
	if !strings.Contains(content, "description: Artistfy deployment workflow") {
		t.Errorf("MDC missing description:\n%s", content)
	}
	if !strings.Contains(content, "alwaysApply: false") {
		t.Errorf("MDC missing alwaysApply:\n%s", content)
	}
	if !strings.Contains(content, "## Deploy") {
		t.Errorf("MDC missing body:\n%s", content)
	}
	if strings.Contains(content, "license: MIT") {
		t.Errorf("MDC should not contain SKILL.md frontmatter")
	}
}
