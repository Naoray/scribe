package manifest

import (
	"context"
	"errors"
	"fmt"
)

// FileFetcher abstracts fetching a single file from a remote repository.
type FileFetcher interface {
	FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
}

// LegacyConverter converts raw legacy TOML bytes into a Manifest.
// Supplied by the caller to avoid an import cycle with the migrate package.
type LegacyConverter func(data []byte) (*Manifest, error)

// FetchWithFallback fetches and parses a manifest, trying scribe.yaml first
// then falling back to legacy scribe.toml with automatic conversion.
// Returns (manifest, isLegacy, error).
func FetchWithFallback(ctx context.Context, f FileFetcher, owner, repo string, convert LegacyConverter) (*Manifest, bool, error) {
	raw, err := f.FetchFile(ctx, owner, repo, ManifestFilename, "HEAD")
	if err == nil {
		m, parseErr := Parse(raw)
		if parseErr != nil {
			return nil, false, fmt.Errorf("parse %s/%s manifest: %w", owner, repo, parseErr)
		}
		return m, false, nil
	}

	// YAML failed — try legacy TOML.
	raw, legacyErr := f.FetchFile(ctx, owner, repo, LegacyManifestFilename, "HEAD")
	if legacyErr != nil {
		return nil, false, fmt.Errorf("fetch manifest from %s/%s: %w", owner, repo, errors.Join(err, legacyErr))
	}

	m, convertErr := convert(raw)
	if convertErr != nil {
		return nil, false, fmt.Errorf("convert legacy manifest from %s/%s: %w", owner, repo, convertErr)
	}
	return m, true, nil
}
