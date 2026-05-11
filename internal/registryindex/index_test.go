package registryindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
)

type metadataClient struct{}

func (metadataClient) RepositoryDefaultBranch(context.Context, string, string) (string, error) {
	return "main", nil
}

func (metadataClient) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "abc123", nil
}

func TestUpsertWritesAndReadsPublicRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index", "registries.json")
	entry := Registry{
		Repo:          "acme/skills",
		SourceRepo:    "acme/skills",
		Visibility:    config.RegistryVisibilityPublic,
		DefaultBranch: "main",
		HeadSHA:       "abc123",
		SkillCount:    2,
		KitCount:      1,
		LastFetchedAt: time.Now().UTC(),
	}
	if err := Upsert(path, entry); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(idx.Registries) != 1 {
		t.Fatalf("registries len = %d, want 1", len(idx.Registries))
	}
	got := idx.Registries[0]
	if got.Repo != "acme/skills" || got.HeadSHA != "abc123" || got.SkillCount != 2 || got.KitCount != 1 {
		t.Fatalf("registry = %#v", got)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestUpsertSkipsNonPublicRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registries.json")
	if err := Upsert(path, Registry{Repo: "acme/private", Visibility: config.RegistryVisibilityPrivate}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(idx.Registries) != 0 {
		t.Fatalf("registries len = %d, want 0", len(idx.Registries))
	}
}

func TestLoadMissingAndCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	idx, err := Load(missing)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if idx.Version != Version || len(idx.Registries) != 0 {
		t.Fatalf("missing index = %#v", idx)
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if _, err := Load(corrupt); err == nil {
		t.Fatal("Load corrupt error = nil")
	}
}

func TestBuildEntryCopiesManifestMetadata(t *testing.T) {
	m := &manifest.Manifest{
		APIVersion: "scribe/v1",
		Kind:       "Registry",
		Team:       &manifest.Team{Name: "Acme"},
		Catalog: []manifest.Entry{
			{Name: "deploy"},
			{Name: "review"},
		},
		Kits: []manifest.KitEntry{{Name: "ops"}},
	}
	entry, err := BuildEntry(context.Background(), config.RegistryConfig{
		Repo:       "acme/skills",
		Visibility: config.RegistryVisibilityPublic,
	}, m, metadataClient{})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if !entry.ManifestPresent || !entry.Manifest.Present || entry.Manifest.TeamName != "Acme" {
		t.Fatalf("manifest metadata = %#v", entry.Manifest)
	}
	if entry.SkillCount != 2 || entry.KitCount != 1 || entry.HeadSHA != "abc123" {
		t.Fatalf("entry = %#v", entry)
	}
}
