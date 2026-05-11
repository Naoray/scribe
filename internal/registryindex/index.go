package registryindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/paths"
)

const Version = 1

type MetadataClient interface {
	RepositoryDefaultBranch(ctx context.Context, owner, repo string) (string, error)
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
}

type ManifestMetadata struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind,omitempty"`
	TeamName   string `json:"team_name,omitempty"`
	Present    bool   `json:"present"`
}

type Registry struct {
	Repo            string           `json:"repo"`
	SourceRepo      string           `json:"source_repo"`
	Visibility      string           `json:"visibility"`
	DefaultBranch   string           `json:"default_branch,omitempty"`
	HeadSHA         string           `json:"head_sha,omitempty"`
	ManifestPresent bool             `json:"manifest_present"`
	Manifest        ManifestMetadata `json:"manifest"`
	SkillCount      int              `json:"skill_count"`
	KitCount        int              `json:"kit_count"`
	LastFetchedAt   time.Time        `json:"last_fetched_at"`
	Tags            []string         `json:"tags,omitempty"`
}

type Index struct {
	Version    int        `json:"version"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Registries []Registry `json:"registries"`
}

func Path() (string, error) {
	dir, err := paths.ScribeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index", "registries.json"), nil
}

func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyIndex(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry index %s: %w", path, err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse registry index %s: %w (remove the file or reconnect public registries to rebuild)", path, err)
	}
	if idx.Version == 0 {
		idx.Version = Version
	}
	if idx.Registries == nil {
		idx.Registries = []Registry{}
	}
	return &idx, nil
}

func Save(path string, idx *Index) error {
	if idx == nil {
		idx = emptyIndex()
	}
	idx.Version = Version
	idx.UpdatedAt = time.Now().UTC()
	sort.Slice(idx.Registries, func(i, j int) bool {
		return idx.Registries[i].Repo < idx.Registries[j].Repo
	})

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create registry index dir: %w", err)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registry index: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".registries.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp registry index: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp registry index: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp registry index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp registry index: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("save registry index: %w", err)
	}
	return nil
}

func Upsert(path string, entry Registry) error {
	idx, err := Load(path)
	if err != nil {
		return err
	}
	entry.Visibility = config.NormalizeRegistryVisibility(entry.Visibility)
	if entry.Visibility != config.RegistryVisibilityPublic {
		idx.remove(entry.Repo)
		return Save(path, idx)
	}
	if entry.SourceRepo == "" {
		entry.SourceRepo = entry.Repo
	}
	if entry.LastFetchedAt.IsZero() {
		entry.LastFetchedAt = time.Now().UTC()
	}
	for i := range idx.Registries {
		if idx.Registries[i].Repo == entry.Repo {
			idx.Registries[i] = entry
			return Save(path, idx)
		}
	}
	idx.Registries = append(idx.Registries, entry)
	return Save(path, idx)
}

func Remove(path, repo string) error {
	idx, err := Load(path)
	if err != nil {
		return err
	}
	idx.remove(repo)
	return Save(path, idx)
}

func BuildEntry(ctx context.Context, rc config.RegistryConfig, m *manifest.Manifest, client MetadataClient) (Registry, error) {
	rc.Normalize()
	entry := Registry{
		Repo:            rc.Repo,
		SourceRepo:      rc.Repo,
		Visibility:      rc.Visibility,
		ManifestPresent: m != nil,
		LastFetchedAt:   time.Now().UTC(),
	}
	if m != nil {
		entry.Manifest = ManifestMetadata{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
			Present:    true,
		}
		if m.Team != nil {
			entry.Manifest.TeamName = m.Team.Name
		}
		entry.SkillCount = len(m.Catalog)
		entry.KitCount = len(m.Kits)
	}
	if client != nil {
		owner, repo, err := manifest.ParseOwnerRepo(rc.Repo)
		if err == nil {
			branch, metaErr := client.RepositoryDefaultBranch(ctx, owner, repo)
			if metaErr != nil {
				return Registry{}, metaErr
			}
			entry.DefaultBranch = branch
			if branch != "" {
				sha, shaErr := client.LatestCommitSHA(ctx, owner, repo, branch)
				if shaErr != nil {
					return Registry{}, shaErr
				}
				entry.HeadSHA = sha
			}
		}
	}
	return entry, nil
}

func emptyIndex() *Index {
	return &Index{Version: Version, Registries: []Registry{}}
}

func (idx *Index) remove(repo string) {
	kept := idx.Registries[:0]
	for _, entry := range idx.Registries {
		if entry.Repo != repo {
			kept = append(kept, entry)
		}
	}
	idx.Registries = kept
}
