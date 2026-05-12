package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/tools"
)

// File is a single file within a downloaded skill directory.
// Re-exported from tools for caller clarity.
type File = tools.SkillFile

// DiscoverResult holds the output of a Discover call.
type DiscoverResult struct {
	Entries   []manifest.Entry
	Kits      []KitFile
	KitErrors KitFetchErrors
	IsTeam    bool // true if discovery found a scribe.yaml/toml with a team section
	Manifest  *manifest.Manifest
}

// KitFile is a registry-published kit body fetched during discovery.
type KitFile struct {
	Name string
	Path string
	Body []byte
	Ref  string
}

// KitFetchError records one non-fatal kit fetch/pre-parse failure.
type KitFetchError struct {
	Name string
	Path string
	Err  error
}

func (e KitFetchError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("%s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("%s (%s): %v", e.Name, e.Path, e.Err)
}

func (e KitFetchError) Unwrap() error { return e.Err }

// KitFetchErrors is a typed partial-error list returned alongside fetched kits.
type KitFetchErrors []KitFetchError

func (e KitFetchErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, err := range e {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

// Provider abstracts how skills are discovered and fetched from a repository.
// Different repo formats (scribe.yaml, marketplace.json, bare SKILL.md trees)
// are handled transparently behind this interface.
type Provider interface {
	// Discover probes a repository and returns all discoverable catalog entries.
	// The repo argument is "owner/repo" format.
	Discover(ctx context.Context, repo string) (*DiscoverResult, error)

	// Fetch downloads all files for a single catalog entry.
	// Returns skill files ready to be written to the canonical store.
	Fetch(ctx context.Context, entry manifest.Entry) ([]File, error)
}

// SourceProvider is the transitional SourceSpec-aware provider API.
// Provider remains stable so existing tests and callers can keep using repo strings.
type SourceProvider interface {
	DiscoverSource(ctx context.Context, spec source.SourceSpec) (*DiscoverResult, error)
	FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]File, error)
}
