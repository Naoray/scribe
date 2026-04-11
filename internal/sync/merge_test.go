package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThreeWayMergeClean(t *testing.T) {
	dir := t.TempDir()

	// Base: original content.
	base := "line1\nline2\nline3\n"
	// Local: added line at end.
	local := "line1\nline2\nline3\nlocal-addition\n"
	// Upstream: changed line1.
	upstream := "line1-changed\nline2\nline3\n"

	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), []byte(base), 0o644)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(local), 0o644)

	result, err := ThreeWayMerge(dir, []byte(upstream))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != MergeClean {
		t.Errorf("expected MergeClean, got %d", result)
	}

	// Check merged content has both changes.
	merged, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if !strings.Contains(string(merged), "line1-changed") {
		t.Error("merged content should contain upstream change")
	}
	if !strings.Contains(string(merged), "local-addition") {
		t.Error("merged content should contain local addition")
	}

	// Check .scribe-base.md was updated to upstream.
	baseContent, _ := os.ReadFile(filepath.Join(dir, ".scribe-base.md"))
	if string(baseContent) != upstream {
		t.Error(".scribe-base.md should be updated to upstream content")
	}
}

func TestThreeWayMergeConflict(t *testing.T) {
	dir := t.TempDir()

	// Base: original content.
	base := "line1\nline2\nline3\n"
	// Local: changed line1 one way.
	local := "local-change\nline2\nline3\n"
	// Upstream: changed line1 another way.
	upstream := "upstream-change\nline2\nline3\n"

	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), []byte(base), 0o644)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(local), 0o644)

	result, err := ThreeWayMerge(dir, []byte(upstream))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != MergeConflict {
		t.Errorf("expected MergeConflict, got %d", result)
	}

	// Check conflict markers are present.
	merged, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	content := string(merged)
	if !strings.Contains(content, "<<<<<<<") || !strings.Contains(content, ">>>>>>>") {
		t.Error("expected conflict markers in merged file")
	}
}

func TestComputeFileHash(t *testing.T) {
	content := []byte("hello world")
	hash1 := ComputeFileHash(content)
	hash2 := ComputeFileHash(content)

	if hash1 != hash2 {
		t.Errorf("hash should be deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 8 {
		t.Errorf("hash should be 8 chars, got %d", len(hash1))
	}

	// Different content → different hash.
	hash3 := ComputeFileHash([]byte("different"))
	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestIsLocallyModified(t *testing.T) {
	dir := t.TempDir()

	content := []byte("original content")
	hash := ComputeFileHash(content)

	os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644)

	// Unmodified.
	if IsLocallyModified(dir, hash) {
		t.Error("should not report modified when content matches hash")
	}

	// Modified.
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("changed content"), 0o644)
	if !IsLocallyModified(dir, hash) {
		t.Error("should report modified when content differs from hash")
	}

	// Empty hash → not modified (first install, no hash recorded yet).
	if IsLocallyModified(dir, "") {
		t.Error("empty hash should not report modified")
	}

	// Missing file → not modified.
	if IsLocallyModified(filepath.Join(dir, "nonexistent"), hash) {
		t.Error("missing file should not report modified")
	}
}
