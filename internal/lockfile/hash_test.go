package lockfile

import (
	"os"
	"os/exec"
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

func TestHashSetNormalizesLineEndingsAndDenylist(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "# deploy\r\nbody\r\n")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "ok\n")
	mustWrite(t, filepath.Join(dir, ".scribe-content-hash"), "sha256:old\n")
	mustWrite(t, filepath.Join(dir, ".DS_Store"), "noise\n")
	mustWrite(t, filepath.Join(dir, "versions", "rev-1.md"), "old\n")

	got, err := HashSet(dir)
	if err != nil {
		t.Fatalf("HashSet() error = %v", err)
	}
	want, err := HashFiles([]File{
		{Path: "SKILL.md", Content: []byte("# deploy\nbody\n")},
		{Path: "notes.txt", Content: []byte("ok\n")},
	})
	if err != nil {
		t.Fatalf("HashFiles() error = %v", err)
	}
	if got != want {
		t.Fatalf("HashSet() = %s, want %s", got, want)
	}
}

func TestHashSetGitModeIncludesUntrackedThenTrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "scribe@example.test")
	runGit(t, repo, "config", "user.name", "Scribe Test")

	skillDir := filepath.Join(repo, ".ai", "skills", "deploy")
	mustWrite(t, filepath.Join(skillDir, "SKILL.md"), "# deploy\n")
	mustWrite(t, filepath.Join(skillDir, "README.md"), "docs\n")

	beforeAdd, err := HashSet(skillDir)
	if err != nil {
		t.Fatalf("HashSet() before add error = %v", err)
	}
	runGit(t, repo, "add", ".ai/skills/deploy")
	afterAdd, err := HashSet(skillDir)
	if err != nil {
		t.Fatalf("HashSet() after add error = %v", err)
	}
	runGit(t, repo, "commit", "-m", "add skill")
	afterCommit, err := HashSet(skillDir)
	if err != nil {
		t.Fatalf("HashSet() after commit error = %v", err)
	}
	if beforeAdd != afterAdd || beforeAdd != afterCommit {
		t.Fatalf("HashSet() changed across git lifecycle: before=%s afterAdd=%s afterCommit=%s", beforeAdd, afterAdd, afterCommit)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
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
