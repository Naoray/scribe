package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContentHash(t *testing.T) {
	// Create a temp skill directory with known content.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---\n# Test"), 0o644)
	os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash\necho hello"), 0o644)

	hash, err := contentHash(dir)
	if err != nil {
		t.Fatalf("contentHash: %v", err)
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q (len %d)", hash, len(hash))
	}

	// Same content → same hash.
	hash2, err := contentHash(dir)
	if err != nil {
		t.Fatalf("contentHash second call: %v", err)
	}
	if hash != hash2 {
		t.Errorf("determinism: got %q then %q", hash, hash2)
	}
}

func TestContentHash_Excludes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)

	hashBefore, _ := contentHash(dir)

	// Add excluded files — hash should not change.
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("binary"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("module.exports = {}"), 0o644)

	hashAfter, _ := contentHash(dir)
	if hashBefore != hashAfter {
		t.Errorf("excluded files changed hash: %q → %q", hashBefore, hashAfter)
	}
}

func TestContentHash_CRLFNormalization(t *testing.T) {
	dirLF := t.TempDir()
	os.WriteFile(filepath.Join(dirLF, "file.md"), []byte("line1\nline2\n"), 0o644)

	dirCRLF := t.TempDir()
	os.WriteFile(filepath.Join(dirCRLF, "file.md"), []byte("line1\r\nline2\r\n"), 0o644)

	hashLF, _ := contentHash(dirLF)
	hashCRLF, _ := contentHash(dirCRLF)

	if hashLF != hashCRLF {
		t.Errorf("CRLF normalization failed: LF=%q CRLF=%q", hashLF, hashCRLF)
	}
}

func TestContentHash_GithubDirIncluded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)
	hashBefore, _ := contentHash(dir)

	// .github/ should be included (it's legitimate skill content).
	os.MkdirAll(filepath.Join(dir, ".github"), 0o755)
	os.WriteFile(filepath.Join(dir, ".github", "template.md"), []byte("template"), 0o644)

	hashAfter, _ := contentHash(dir)
	if hashBefore == hashAfter {
		t.Error(".github/ content should be included in hash but was ignored")
	}
}

func TestContentHash_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)
	os.Symlink("/nonexistent/path", filepath.Join(dir, "broken.md"))

	// Should not error — just skip the broken symlink.
	hash, err := contentHash(dir)
	if err != nil {
		t.Fatalf("broken symlink should be skipped, got error: %v", err)
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q", hash)
	}
}
