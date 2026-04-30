package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSkillPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tool := ClaudeTool{}
	got, err := tool.SkillPath("commit")
	if err != nil {
		t.Fatalf("SkillPath: %v", err)
	}
	want := filepath.Join(home, ".claude", "skills", "commit")
	if got != want {
		t.Errorf("SkillPath = %q, want %q", got, want)
	}
}

func TestClaudeInstall_ReplacesExistingSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	// Create an old canonical dir and install it first.
	oldCanonical := filepath.Join(home, ".scribe", "skills", "qa-old")
	if err := os.MkdirAll(oldCanonical, 0o755); err != nil {
		t.Fatalf("mkdir old canonical: %v", err)
	}

	tool := ClaudeTool{}
	if _, err := tool.Install("qa", oldCanonical, ""); err != nil {
		t.Fatalf("first Install: %v", err)
	}

	// Now install with a new canonical dir — should replace the symlink.
	newCanonical := filepath.Join(home, ".scribe", "skills", "qa-new")
	if err := os.MkdirAll(newCanonical, 0o755); err != nil {
		t.Fatalf("mkdir new canonical: %v", err)
	}
	if _, err := tool.Install("qa", newCanonical, ""); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	link := filepath.Join(home, ".claude", "skills", "qa")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != newCanonical {
		t.Errorf("symlink target = %q, want %q", target, newCanonical)
	}
}

func TestClaudeInstall_FailsOnRealDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	// Create a real (non-symlink) directory at the skill path — simulates a
	// skill installed by another tool or manually.
	skillsDir := filepath.Join(home, ".claude", "skills")
	realDir := filepath.Join(skillsDir, "qa")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# qa"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	canonical := filepath.Join(home, ".scribe", "skills", "qa")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatalf("mkdir canonical: %v", err)
	}

	tool := ClaudeTool{}
	_, err := tool.Install("qa", canonical, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrRealDirectoryExists) {
		t.Errorf("expected ErrRealDirectoryExists, got: %v", err)
	}
	if !strings.Contains(err.Error(), realDir) {
		t.Errorf("error should contain offending path %q, got: %v", realDir, err)
	}

	// Real directory must be preserved.
	if _, statErr := os.Stat(filepath.Join(realDir, "SKILL.md")); statErr != nil {
		t.Errorf("real directory was destroyed: %v", statErr)
	}
}

func TestClaudeInstallSymlinksToDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the .claude directory so Detect() works.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	// Set up a fake canonical dir with SKILL.md.
	canonicalDir := filepath.Join(home, ".scribe", "skills", "cleanup")
	if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
		t.Fatalf("mkdir canonical: %v", err)
	}
	skillMD := filepath.Join(canonicalDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("# Cleanup"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	tool := ClaudeTool{}
	paths, err := tool.Install("cleanup", canonicalDir, "")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	link := paths[0]

	// Verify symlink target points to the canonical directory.
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	if target != canonicalDir {
		t.Errorf("symlink target = %q, want %q", target, canonicalDir)
	}

	// Verify the symlink resolves to a directory (Claude Code expects this).
	info, err := os.Stat(link)
	if err != nil {
		t.Fatalf("stat symlink: %v", err)
	}
	if !info.IsDir() {
		t.Error("symlink should resolve to a directory, not a file")
	}

	// Verify SKILL.md is accessible through the symlink.
	linkedSkillMD := filepath.Join(link, "SKILL.md")
	if _, err := os.Stat(linkedSkillMD); err != nil {
		t.Errorf("SKILL.md not accessible through symlink: %v", err)
	}
}
