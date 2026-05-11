package registry

import (
	"context"
	"fmt"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
)

type FileFetcher interface {
	FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
}

type ManifestKit struct {
	Registry    string
	Name        string
	Path        string
	Description string
	Author      string
}

func ListKits(ctx context.Context, client FileFetcher, registryRepo string) ([]ManifestKit, error) {
	owner, repo, err := manifest.ParseOwnerRepo(registryRepo)
	if err != nil {
		return nil, err
	}
	m, _, err := manifest.FetchWithFallback(ctx, client, owner, repo, migrate.Convert)
	if err != nil {
		return nil, err
	}
	out := make([]ManifestKit, 0, len(m.Kits))
	for _, entry := range m.Kits {
		out = append(out, ManifestKit{
			Registry:    registryRepo,
			Name:        entry.Name,
			Path:        entry.PathOrDefault(),
			Description: entry.Description,
			Author:      entry.Author,
		})
	}
	return out, nil
}

func FetchKitBody(ctx context.Context, client FileFetcher, registryRepo string, entry manifest.KitEntry) (*kit.Kit, error) {
	owner, repo, err := manifest.ParseOwnerRepo(registryRepo)
	if err != nil {
		return nil, err
	}
	path := entry.PathOrDefault()
	raw, err := client.FetchFile(ctx, owner, repo, path, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("fetch kit %s:%s at %s: %w", registryRepo, entry.Name, path, err)
	}
	k, err := kit.Parse(raw)
	if err != nil {
		return nil, err
	}
	if k.Name == "" {
		k.Name = entry.Name
	}
	if k.Source == nil {
		k.Source = &kit.Source{Registry: registryRepo}
	}
	return k, nil
}

func FindKit(ctx context.Context, client FileFetcher, registryRepo, name string) (manifest.KitEntry, error) {
	owner, repo, err := manifest.ParseOwnerRepo(registryRepo)
	if err != nil {
		return manifest.KitEntry{}, err
	}
	m, _, err := manifest.FetchWithFallback(ctx, client, owner, repo, migrate.Convert)
	if err != nil {
		return manifest.KitEntry{}, err
	}
	for _, entry := range m.Kits {
		if entry.Name == name {
			return entry, nil
		}
	}
	return manifest.KitEntry{}, fmt.Errorf("kit %q not found in registry %s", name, registryRepo)
}
