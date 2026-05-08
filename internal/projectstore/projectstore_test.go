package projectstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
)

func TestResolverLoadKitsProjectPrecedence(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	mustSaveKit(t, filepath.Join(globalRoot, "kits", "base.yaml"), &kit.Kit{Name: "base", Skills: []string{"global"}})
	mustSaveKit(t, filepath.Join(projectRoot, ".ai", "kits", "base.yaml"), &kit.Kit{Name: "base", Skills: []string{"project"}})
	mustSaveKit(t, filepath.Join(globalRoot, "kits", "global-only.yaml"), &kit.Kit{Name: "global-only", Skills: []string{"x"}})

	resolver := NewResolver(projectRoot, globalRoot)
	kits, err := resolver.LoadKits()
	if err != nil {
		t.Fatalf("LoadKits() error = %v", err)
	}
	if got := kits["base"].Skills; len(got) != 1 || got[0] != "project" {
		t.Fatalf("base skills = %v, want project override", got)
	}
	if _, ok := kits["global-only"]; !ok {
		t.Fatal("global-only kit missing")
	}
}

func TestResolverSkillDirProjectPrecedence(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	projectSkill := filepath.Join(projectRoot, ".ai", "skills", "review")
	globalSkill := filepath.Join(globalRoot, "skills", "review")
	mustWrite(t, filepath.Join(globalSkill, "SKILL.md"), "# global\n")
	mustWrite(t, filepath.Join(projectSkill, "SKILL.md"), "# project\n")

	got, ok := NewResolver(projectRoot, globalRoot).SkillDir("review")
	if !ok {
		t.Fatal("SkillDir() found=false")
	}
	if got != projectSkill {
		t.Fatalf("SkillDir() = %q, want %q", got, projectSkill)
	}
}

func TestProjectLockfileRoundTrip(t *testing.T) {
	store := Project(t.TempDir())
	lf := &lockfile.ProjectLockfile{
		FormatVersion: lockfile.SchemaVersion,
		Kind:          lockfile.ProjectKind,
		Entries: []lockfile.ProjectEntry{{
			Entry: lockfile.Entry{
				Name:           "review",
				SourceRegistry: "acme/skills",
				CommitSHA:      "abc",
				ContentHash:    strings.Repeat("a", 64),
			},
			SourceRepo: "acme/source",
			Path:       "skills/review",
		}},
	}
	if err := store.WriteProjectLockfile(lf); err != nil {
		t.Fatalf("WriteProjectLockfile() error = %v", err)
	}
	got, err := store.LoadProjectLockfile()
	if err != nil {
		t.Fatalf("LoadProjectLockfile() error = %v", err)
	}
	entry, ok := got.Entry("review")
	if !ok || entry.SourceRepo != "acme/source" {
		t.Fatalf("project entry = %+v, ok=%v", entry, ok)
	}
}

func TestContentMarkerVerify(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "# review\n")
	hash, err := lockfile.HashSet(dir)
	if err != nil {
		t.Fatalf("HashSet() error = %v", err)
	}
	if err := WriteMarker(dir, hash, "scribe@test", time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("WriteMarker() error = %v", err)
	}
	marker, actual, err := VerifyMarker(dir)
	if err != nil {
		t.Fatalf("VerifyMarker() error = %v", err)
	}
	if marker.Hash != hash || actual != hash {
		t.Fatalf("marker=%+v actual=%s want %s", marker, actual, hash)
	}
	mustWrite(t, filepath.Join(dir, "README.md"), "changed\n")
	_, _, err = VerifyMarker(dir)
	if err == nil {
		t.Fatal("VerifyMarker() should reject changed content")
	}
}

func mustSaveKit(t *testing.T, path string, k *kit.Kit) {
	t.Helper()
	if err := kit.Save(path, k); err != nil {
		t.Fatalf("save kit: %v", err)
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
