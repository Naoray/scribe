package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

type singleFileProvider struct{}

func (p *singleFileProvider) Discover(_ context.Context, repo string) (*provider.DiscoverResult, error) {
	return &provider.DiscoverResult{
		IsTeam: true,
		Entries: []manifest.Entry{{
			Name:   "scribe-agent",
			Source: "github:Naoray/scribe@HEAD",
			Path:   "SKILL.md",
		}},
	}, nil
}

func (p *singleFileProvider) Fetch(_ context.Context, entry manifest.Entry) ([]provider.File, error) {
	return []provider.File{{
		Path:    "SKILL.md",
		Content: []byte("---\nname: scribe-agent\ndescription: test\n---\nbody\n"),
	}}, nil
}

type singleFileFetcher struct{}

func (f *singleFileFetcher) FetchFile(context.Context, string, string, string, string) ([]byte, error) {
	return nil, nil
}

func (f *singleFileFetcher) FetchDirectory(context.Context, string, string, string, string) ([]sync.SkillFile, error) {
	return nil, nil
}

func (f *singleFileFetcher) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (f *singleFileFetcher) GetTree(context.Context, string, string, string) ([]provider.TreeEntry, error) {
	return []provider.TreeEntry{{Path: "SKILL.md", Type: "blob", SHA: "root-sha"}}, nil
}

// Drives direct install through runAddDirectInstall, the highest-level
// non-cobra seam in cmd/add.go for owner/repo:skill installs.
func TestAddScribeAgent_SingleFileSkill_EndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{}
	st := &state.State{Installed: make(map[string]state.InstalledSkill)}
	syncer := &sync.Syncer{
		Client:   &singleFileFetcher{},
		Provider: &singleFileProvider{},
	}

	if err := runAddDirectInstall(
		context.Background(),
		"Naoray/scribe",
		"scribe-agent",
		cfg,
		st,
		syncer,
		true,
		true,
		true,
	); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	storeDir := filepath.Join(os.Getenv("HOME"), ".scribe", "skills", "scribe-agent")
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(entries) != 2 {
		t.Fatalf("store dir should contain SKILL.md plus merge metadata, got %d: %v", len(entries), names)
	}
	if names[0] != ".scribe-base.md" || names[1] != "SKILL.md" {
		t.Errorf("unexpected store contents: %v", names)
	}

	content, err := os.ReadFile(filepath.Join(storeDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !bytes.Contains(content, []byte("name: scribe-agent")) {
		t.Errorf("SKILL.md missing expected frontmatter: %q", string(content))
	}
}
