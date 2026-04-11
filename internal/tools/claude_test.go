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

func TestClaudeInstallSymlinksToFile(t *testing.T) {
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
	// Also write .scribe-base.md to verify it's NOT visible through the symlink.
	baseMD := filepath.Join(canonicalDir, ".scribe-base.md")
	if err := os.WriteFile(baseMD, []byte("# Cleanup"), 0o644); err != nil {
		t.Fatalf("write .scribe-base.md: %v", err)
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

	// Verify symlink target points to SKILL.md file, not the directory.
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	wantTarget := filepath.Join(canonicalDir, "SKILL.md")
	if target != wantTarget {
		t.Errorf("symlink target = %q, want %q", target, wantTarget)
	}

	// Verify the symlink resolves to a file, not a directory.
	info, err := os.Stat(link)
	if err != nil {
		t.Fatalf("stat symlink: %v", err)
	}
	if info.IsDir() {
		t.Error("symlink should resolve to a file, not a directory")
	}
}
