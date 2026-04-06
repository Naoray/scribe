package provider

import (
	"context"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/tools"
)

// File is a single file within a downloaded skill directory.
// Re-exported from tools for caller clarity.
type File = tools.SkillFile

// Provider abstracts how skills are discovered and fetched from a repository.
// Different repo formats (scribe.yaml, marketplace.json, bare SKILL.md trees)
// are handled transparently behind this interface.
type Provider interface {
	// Discover probes a repository and returns all discoverable catalog entries.
	// The repo argument is "owner/repo" format.
	Discover(ctx context.Context, repo string) ([]manifest.Entry, error)

	// Fetch downloads all files for a single catalog entry.
	// Returns skill files ready to be written to the canonical store.
	Fetch(ctx context.Context, entry manifest.Entry) ([]File, error)
}
