package provider

import (
	"context"
	"fmt"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/source"
)

// CompositeProvider dispatches SourceSpec-aware calls to the matching backend.
type CompositeProvider struct {
	github *GitHubProvider
	git    *GitProvider
	local  *FilesystemProvider
}

func NewCompositeProvider(github *GitHubProvider) *CompositeProvider {
	return &CompositeProvider{
		github: github,
		git:    NewGitProvider(),
		local:  NewFilesystemProvider(),
	}
}

func (p *CompositeProvider) Discover(ctx context.Context, repo string) (*DiscoverResult, error) {
	spec, err := source.ParseSourceArg(repo)
	if err != nil {
		return nil, err
	}
	return p.DiscoverSource(ctx, spec)
}

func (p *CompositeProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]File, error) {
	return p.github.Fetch(ctx, entry)
}

func (p *CompositeProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*DiscoverResult, error) {
	backend, err := p.backend(spec)
	if err != nil {
		return nil, err
	}
	return backend.DiscoverSource(ctx, spec)
}

func (p *CompositeProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]File, error) {
	backend, err := p.backend(spec)
	if err != nil {
		return nil, err
	}
	return backend.FetchSource(ctx, spec, entry)
}

func (p *CompositeProvider) backend(spec source.SourceSpec) (SourceProvider, error) {
	spec, err := source.CanonicalSpec(spec)
	if err != nil {
		return nil, err
	}
	switch spec.Type {
	case source.SourceGitHub:
		return p.github, nil
	case source.SourceGitLab, source.SourceGit:
		return p.git, nil
	case source.SourceLocal:
		return p.local, nil
	default:
		return nil, fmt.Errorf("unsupported source type %q", spec.Type)
	}
}
