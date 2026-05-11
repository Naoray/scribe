package provider

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/source"
)

// FilesystemProvider discovers and fetches skills from a directory tree.
type FilesystemProvider struct {
	sourceName string
	author     string
}

func NewFilesystemProvider() *FilesystemProvider {
	return &FilesystemProvider{sourceName: "local", author: "local"}
}

func (p *FilesystemProvider) Discover(ctx context.Context, repo string) (*DiscoverResult, error) {
	return p.DiscoverSource(ctx, source.SourceSpec{Type: source.SourceLocal, Path: repo})
}

func (p *FilesystemProvider) Fetch(ctx context.Context, entry manifest.Entry) ([]File, error) {
	return nil, fmt.Errorf("local fetch requires SourceSpec path")
}

func (p *FilesystemProvider) DiscoverSource(ctx context.Context, spec source.SourceSpec) (*DiscoverResult, error) {
	root, sourceID, err := filesystemRoot(spec)
	if err != nil {
		return nil, err
	}
	return p.discoverRoot(ctx, root, sourceID)
}

func (p *FilesystemProvider) FetchSource(ctx context.Context, spec source.SourceSpec, entry manifest.Entry) ([]File, error) {
	root, _, err := filesystemRoot(spec)
	if err != nil {
		return nil, err
	}
	skillPath := entry.Path
	if skillPath == "" {
		skillPath = entry.Name
	}
	fullPath, err := safeJoin(root, skillPath)
	if err != nil {
		return nil, err
	}
	return fetchPath(ctx, fullPath)
}

func (p *FilesystemProvider) discoverRoot(ctx context.Context, root, sourceID string) (*DiscoverResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if st, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("open local source %s: %w", root, err)
	} else if !st.IsDir() {
		return nil, fmt.Errorf("local source %s is not a directory", root)
	}

	if m, err := readManifest(root, manifest.ManifestFilename); err == nil {
		return &DiscoverResult{Entries: m.Catalog, IsTeam: true, Manifest: m}, nil
	}
	if m, err := readLegacyManifest(root); err == nil {
		return &DiscoverResult{Entries: m.Catalog, IsTeam: true, Manifest: m}, nil
	}
	if entries, err := readMarketplace(root, p.author, sourceID); err == nil && len(entries) > 0 {
		return &DiscoverResult{Entries: entries, IsTeam: false}, nil
	}
	entries, err := scanFilesystem(ctx, root, p.author, filepath.Base(root), sourceID)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return &DiscoverResult{Entries: entries, IsTeam: false}, nil
	}
	return nil, fmt.Errorf("%s: no skills found (looked for scribe.yaml, scribe.toml, marketplace.json, and SKILL.md files)", root)
}

func filesystemRoot(spec source.SourceSpec) (string, string, error) {
	spec, err := source.CanonicalSpec(spec)
	if err != nil {
		return "", "", err
	}
	if spec.Type != source.SourceLocal {
		return "", "", fmt.Errorf("unsupported source type %q for local provider", spec.Type)
	}
	return spec.Path, "local:" + spec.Path, nil
}

func readManifest(root, name string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return nil, err
	}
	return manifest.Parse(data)
}

func readLegacyManifest(root string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, manifest.LegacyManifestFilename))
	if err != nil {
		return nil, err
	}
	return migrate.Convert(data)
}

func readMarketplace(root, author, sourceID string) ([]manifest.Entry, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(marketplacePath)))
	if err != nil {
		return nil, err
	}
	return parseMarketplaceWithSource(data, author, sourceID)
}

func scanFilesystem(ctx context.Context, root, owner, repo, sourceID string) ([]manifest.Entry, error) {
	var tree []TreeEntry
	err := filepath.WalkDir(root, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		tree = append(tree, TreeEntry{Path: filepath.ToSlash(rel), Type: "blob"})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	entries := scanTreeForSkillsWithSource(tree, owner, repo, sourceID)
	for i := range entries {
		skillPath := entries[i].Path
		if path.Base(skillPath) != skillFileName {
			skillPath = path.Join(skillPath, skillFileName)
		}
		fullPath, err := safeJoin(root, skillPath)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		enriched, err := EnrichTreeSkillEntry(entries[i], data)
		if err == nil {
			entries[i] = enriched
		}
	}
	return entries, nil
}

func fetchPath(ctx context.Context, fullPath string) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read skill path %s: %w", fullPath, err)
	}
	if !info.IsDir() {
		if filepath.Base(fullPath) != skillFileName {
			return nil, fmt.Errorf("skill path %s is a file, expected SKILL.md or directory", fullPath)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fullPath, err)
		}
		return []File{{Path: skillFileName, Content: data}}, nil
	}
	var files []File
	err = filepath.WalkDir(fullPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fullPath, filePath)
		if err != nil {
			return err
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Content: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read skill directory %s: %w", fullPath, err)
	}
	return files, nil
}

func safeJoin(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return filepath.Clean(root), nil
	}
	if strings.Contains(rel, "\\") {
		return "", fmt.Errorf("source path %q must use slash separators", rel)
	}
	cleaned := path.Clean(rel)
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("source path %q cannot escape source root", rel)
	}
	return filepath.Join(root, filepath.FromSlash(cleaned)), nil
}
