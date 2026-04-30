package manifest

import (
	"context"
	"errors"
	"fmt"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
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
			return nil, false, clierrors.Wrap(parseErr, "MANIFEST_INVALID", clierrors.ExitValid,
				clierrors.WithMessage(fmt.Sprintf("parse %s/%s manifest: %v", owner, repo, parseErr)),
				clierrors.WithResource(owner+"/"+repo),
			)
		}
		return m, false, nil
	}

	// YAML failed — try legacy TOML.
	raw, legacyErr := f.FetchFile(ctx, owner, repo, LegacyManifestFilename, "HEAD")
	if legacyErr != nil {
		joined := errors.Join(err, legacyErr)
		return nil, false, clierrors.Wrap(joined, "REGISTRY_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithMessage(fmt.Sprintf("fetch manifest from %s/%s: %v", owner, repo, joined)),
			clierrors.WithResource(owner+"/"+repo),
		)
	}

	m, convertErr := convert(raw)
	if convertErr != nil {
		return nil, false, clierrors.Wrap(convertErr, "MANIFEST_INVALID", clierrors.ExitValid,
			clierrors.WithMessage(fmt.Sprintf("convert legacy manifest from %s/%s: %v", owner, repo, convertErr)),
			clierrors.WithResource(owner+"/"+repo),
		)
	}
	return m, true, nil
}
