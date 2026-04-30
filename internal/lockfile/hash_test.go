package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFilesCanonicalizesOrder(t *testing.T) {
	a, err := HashFiles([]File{
		{Path: "b.txt", Content: []byte("b")},
		{Path: "a.txt", Content: []byte("a")},
	})
	if err != nil {
		t.Fatalf("HashFiles() error = %v", err)
	}
	b, err := HashFiles([]File{
		{Path: "a.txt", Content: []byte("a")},
		{Path: "b.txt", Content: []byte("b")},
	})
	if err != nil {
		t.Fatalf("HashFiles() error = %v", err)
	}
	if a != b {
		t.Fatalf("hashes differ: %s != %s", a, b)
	}
}

func TestHashDirSkipsVersionSnapshots(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "# deploy\n")
	mustWrite(t, filepath.Join(dir, "versions", "rev-1.md"), "old\n")

	withSnapshot, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "versions")); err != nil {
		t.Fatalf("remove versions: %v", err)
	}
	withoutSnapshot, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir() error = %v", err)
	}
	if withSnapshot != withoutSnapshot {
		t.Fatalf("versions directory should not affect hash: %s != %s", withSnapshot, withoutSnapshot)
	}
}

func TestHashFilesRejectsDuplicatePaths(t *testing.T) {
	_, err := HashFiles([]File{
		{Path: "SKILL.md", Content: []byte("a")},
		{Path: "./SKILL.md", Content: []byte("b")},
	})
	if err == nil {
		t.Fatal("HashFiles() should reject duplicate canonical paths")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
