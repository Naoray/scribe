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

func TestHasConflictMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "top of file",
			content: "<<<<<<< HEAD\nlocal\n=======\nupstream\n>>>>>>> feature\n",
			want:    true,
		},
		{
			name:    "mid file",
			content: "before\n<<<<<<< feature\nlocal\n=======\nupstream\n>>>>>>> feature\n",
			want:    true,
		},
		{
			name:    "indented marker",
			content: "  <<<<<<< nope\n",
			want:    false,
		},
		{
			name:    "prose marker",
			content: "this line mentions <<<<<<< as prose\n",
			want:    false,
		},
		{
			name:    "crlf marker",
			content: "before\r\n<<<<<<< foo\n",
			want:    false,
		},
		{
			name:    "empty",
			content: "",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasConflictMarkers([]byte(tt.content)); got != tt.want {
				t.Fatalf("HasConflictMarkers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLocallyModified_SidecarMatchesSkill_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	content := []byte("original content")
	os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644)
	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), content, 0o644)

	if IsLocallyModified(dir, "deadbeef") {
		t.Error("should not report modified when SKILL.md matches .scribe-base.md")
	}
}

func TestIsLocallyModified_SidecarDiffersFromSkill_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("changed content"), 0o644)
	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), []byte("original content"), 0o644)

	if !IsLocallyModified(dir, "") {
		t.Error("should report modified when SKILL.md differs from .scribe-base.md")
	}
}

func TestIsLocallyModified_SidecarMissingUsesFallbackHash(t *testing.T) {
	dir := t.TempDir()

	content := []byte("original content")
	hash := ComputeFileHash(content)

	os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644)

	if IsLocallyModified(dir, hash) {
		t.Error("should not report modified when content matches fallback hash")
	}

	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("changed content"), 0o644)
	if !IsLocallyModified(dir, hash) {
		t.Error("should report modified when content differs from fallback hash")
	}
}

func TestIsLocallyModified_SidecarMissingEmptyFallback_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)

	if IsLocallyModified(dir, "") {
		t.Error("empty hash should not report modified")
	}
}

func TestIsLocallyModified_CRLFNormalizedComparison(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("line1\r\nline2\r\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), []byte("line1\nline2\n"), 0o644)

	if IsLocallyModified(dir, "") {
		t.Error("line ending differences should not report modified")
	}
}

func TestIsLocallyModified_DriftedFallbackHash_TrustsSidecar(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# claude-api\n\nunchanged skill\n")
	os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644)
	os.WriteFile(filepath.Join(dir, ".scribe-base.md"), content, 0o644)

	if IsLocallyModified(dir, "deadbeef") {
		t.Error("stale fallback hash should not report modified when sidecar matches")
	}
}

func TestIsLocallyModified_SidecarReadError_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	content := []byte("original content")
	os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644)
	os.Mkdir(filepath.Join(dir, ".scribe-base.md"), 0o755)

	if !IsLocallyModified(dir, ComputeFileHash(content)) {
		t.Error("sidecar read errors other than not-exist should report modified")
	}
}

func TestIsLocallyModified_MissingSkill_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	hash := ComputeFileHash([]byte("content"))

	if IsLocallyModified(filepath.Join(dir, "nonexistent"), hash) {
		t.Error("missing file should not report modified")
	}
}
