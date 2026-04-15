package tools

import (
	"os"
	"path/filepath"
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
	paths, err := tool.Install("cleanup", canonicalDir)
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
