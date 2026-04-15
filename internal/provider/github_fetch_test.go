package provider_test

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/tools"
)

func TestGitHubProvider_Fetch_SingleFileSkill(t *testing.T) {
	client := &stubClient{
		files: map[string][]byte{
			"Naoray/scribe/SKILL.md": []byte("---\nname: scribe-agent\n---\nbody\n"),
		},
	}
	p := provider.NewGitHubProvider(client)

	entry := manifest.Entry{
		Name:   "scribe-agent",
		Source: "github:Naoray/scribe@HEAD",
		Path:   "SKILL.md",
	}
	files, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	if files[0].Path != "SKILL.md" {
		t.Errorf("single-file skill must normalize Path to \"SKILL.md\", got %q", files[0].Path)
	}
	if len(files[0].Content) == 0 {
		t.Errorf("content empty")
	}
}

func TestGitHubProvider_Fetch_DirectoryNamedFooMd_IsNotSingleFile(t *testing.T) {
	client := &stubClient{
		dirs: map[string][]tools.SkillFile{
			"Naoray/scribe/foo.md": {{Path: "SKILL.md", Content: []byte("x")}},
		},
	}
	p := provider.NewGitHubProvider(client)

	entry := manifest.Entry{
		Name:   "foo",
		Source: "github:Naoray/scribe@HEAD",
		Path:   "foo.md",
	}
	files, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(files) != 1 || files[0].Path != "SKILL.md" {
		t.Errorf("directory fetch should return files from FetchDirectory, got %+v", files)
	}
}
