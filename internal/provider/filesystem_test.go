package provider_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/source"
)

func TestFilesystemProviderDiscoversLocalTreeScanAndFetchesSkill(t *testing.T) {
	root := t.TempDir()
	writeProviderFile(t, root, "skills/deploy/SKILL.md", "---\nname: deploy\nsummary: Deploy things\n---\n# Deploy\n")
	writeProviderFile(t, root, "skills/deploy/README.md", "# docs\n")

	p := provider.NewFilesystemProvider()
	result, err := p.DiscoverSource(context.Background(), source.SourceSpec{Type: source.SourceLocal, Path: root})
	if err != nil {
		t.Fatalf("DiscoverSource: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(result.Entries))
	}
	entry := result.Entries[0]
	if entry.Name != "deploy" || entry.Path != "skills/deploy" || entry.Source != "local:"+root {
		t.Fatalf("entry = %#v", entry)
	}

	files, err := p.FetchSource(context.Background(), source.SourceSpec{Type: source.SourceLocal, Path: root}, entry)
	if err != nil {
		t.Fatalf("FetchSource: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
}

func TestFilesystemProviderManifestTakesPrecedenceOverTreeScan(t *testing.T) {
	root := t.TempDir()
	writeProviderFile(t, root, "scribe.yaml", `apiVersion: scribe/v1
kind: Registry
team:
  name: local
catalog:
  - name: from-manifest
    path: custom/skill
`)
	writeProviderFile(t, root, "skills/from-tree/SKILL.md", "# Tree\n")

	p := provider.NewFilesystemProvider()
	result, err := p.DiscoverSource(context.Background(), source.SourceSpec{Type: source.SourceLocal, Path: root})
	if err != nil {
		t.Fatalf("DiscoverSource: %v", err)
	}
	if !result.IsTeam {
		t.Fatal("IsTeam = false, want true for scribe.yaml")
	}
	if len(result.Entries) != 1 || result.Entries[0].Name != "from-manifest" {
		t.Fatalf("entries = %#v", result.Entries)
	}
}

func TestGitProviderDiscoversGitLabSpecThroughClonePathAndRef(t *testing.T) {
	remote := createGitFixture(t)

	p := provider.NewGitProvider()
	spec := source.SourceSpec{
		Type: source.SourceGitLab,
		Host: "gitlab.example.test",
		Repo: "group/project",
		URL:  remote,
		Ref:  "main",
		Path: "nested",
	}
	result, err := p.DiscoverSource(context.Background(), spec)
	if err != nil {
		t.Fatalf("DiscoverSource: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Name != "review" || result.Entries[0].Path != "review" {
		t.Fatalf("entry = %#v", result.Entries[0])
	}
	files, err := p.FetchSource(context.Background(), spec, result.Entries[0])
	if err != nil {
		t.Fatalf("FetchSource: %v", err)
	}
	if len(files) != 1 || files[0].Path != "SKILL.md" {
		t.Fatalf("files = %#v", files)
	}
}

func createGitFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runProviderGit(t, repo, "init", "-b", "main")
	runProviderGit(t, repo, "config", "user.email", "test@example.com")
	runProviderGit(t, repo, "config", "user.name", "Test User")
	writeProviderFile(t, repo, "nested/review/SKILL.md", "---\nname: review\n---\n# Review\n")
	runProviderGit(t, repo, "add", "nested/review/SKILL.md")
	runProviderGit(t, repo, "commit", "-m", "add skill")
	return repo
}

func writeProviderFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runProviderGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
