package provider_test

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/tools"
)

// TestProviderInterfaceSatisfaction verifies that the interface contract
// is correct by checking a mock satisfies it.
func TestProviderInterfaceSatisfaction(t *testing.T) {
	var _ provider.Provider = &mockProvider{}
}

type mockProvider struct{}

func (m *mockProvider) Discover(ctx context.Context, repo string) ([]manifest.Entry, error) {
	return []manifest.Entry{{Name: "test"}}, nil
}

func (m *mockProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]tools.SkillFile, error) {
	return []tools.SkillFile{{Path: "SKILL.md", Content: []byte("# test")}}, nil
}

func TestFileTypeAlias(t *testing.T) {
	// Verify File is the same type as tools.SkillFile.
	var f provider.File
	f.Path = "test"
	f.Content = []byte("hello")

	sf := tools.SkillFile(f)
	if sf.Path != "test" {
		t.Errorf("File alias broken: got %q", sf.Path)
	}
}
