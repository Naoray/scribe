package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
)

type parsedKitFile struct {
	file provider.KitFile
	kit  *kit.Kit
	dst  string
}

// StepInstallKits materializes registry-published kits into ~/.scribe/kits.
func StepInstallKits(_ context.Context, b *Bag) error {
	if len(b.Kits) == 0 {
		return nil
	}
	kitsDir, err := registryKitsDir()
	if err != nil {
		return err
	}
	// preflight validates incoming bodies only; pre-existing disk corruption is out-of-contract.
	parsed, err := preflightKitFiles(kitsDir, b.Kits)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		return fmt.Errorf("create kits dir: %w", err)
	}

	written := make([]string, 0, len(parsed))
	for _, item := range parsed {
		sourceRegistry := kitSourceRegistry(b)
		ok, existingSource, err := canWriteKit(item.dst, item.kit.Name, sourceRegistry, b.ForceKits)
		if err != nil {
			return err
		}
		if !ok {
			b.Partial = true
			b.Formatter.OnKitConflict(item.kit.Name, existingSource)
			continue
		}

		item.kit.Source = &kit.Source{Registry: sourceRegistry, Rev: item.file.Ref}
		if err := kit.Save(item.dst, item.kit); err != nil {
			return fmt.Errorf("save kit %q: %w", item.kit.Name, err)
		}
		if err := stampKitState(b, item, sourceRegistry); err != nil {
			return err
		}
		written = append(written, item.kit.Name)
	}

	if len(written) > 0 {
		b.KitsInstalled = append(b.KitsInstalled, written...)
		b.Formatter.OnKitsInstalled(b.RepoArg, written)
	}
	return nil
}

func registryKitsDir() (string, error) {
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return "", fmt.Errorf("resolve scribe dir: %w", err)
	}
	return filepath.Join(scribeDir, "kits"), nil
}

func preflightKitFiles(kitsDir string, files []provider.KitFile) ([]parsedKitFile, error) {
	parsed := make([]parsedKitFile, 0, len(files))
	for _, file := range files {
		k, err := kit.ParseYAML(file.Body)
		if err != nil {
			return nil, fmt.Errorf("parse kit %q: %w", file.Name, err)
		}
		if k.Name != file.Name {
			return nil, fmt.Errorf("kit %q: body name %q doesn't match manifest ref", file.Name, k.Name)
		}
		dst := filepath.Join(kitsDir, k.Name+".yaml")
		rel, err := filepath.Rel(kitsDir, dst)
		if err != nil {
			return nil, fmt.Errorf("check kit path %q: %w", k.Name, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("kit %q: destination escapes kits dir", k.Name)
		}
		parsed = append(parsed, parsedKitFile{file: file, kit: k, dst: dst})
	}
	return parsed, nil
}

func canWriteKit(dst, name, repo string, force bool) (bool, string, error) {
	existing, err := kit.Load(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return true, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("load existing kit %q: %w", name, err)
	}
	existingSource := ""
	if existing.Source != nil {
		existingSource = existing.Source.Registry
	}
	if force || existingSource == repo {
		return true, existingSource, nil
	}
	return false, existingSource, nil
}

func stampKitState(b *Bag, item parsedKitFile, sourceRegistry string) error {
	if b.State == nil {
		return nil
	}
	if b.State.Kits == nil {
		b.State.Kits = map[string]state.InstalledKit{}
	}
	contentHash, err := lockfile.HashFiles([]lockfile.File{{Path: item.kit.Name + ".yaml", Content: item.file.Body}})
	if err != nil {
		return fmt.Errorf("hash kit %q: %w", item.kit.Name, err)
	}
	b.State.Kits[item.kit.Name] = state.InstalledKit{
		Name:           item.kit.Name,
		SourceRegistry: sourceRegistry,
		Rev:            item.file.Ref,
		ContentHash:    contentHash,
		InstalledAt:    time.Now(),
		Source:         sourceRegistry,
		Version:        item.file.Ref,
		Skills:         append([]string(nil), item.kit.Skills...),
	}
	b.MarkStateDirty()
	return nil
}

func kitSourceRegistry(b *Bag) string {
	if b.SourceKey != "" {
		return b.SourceKey
	}
	if b.SourceID != "" {
		return b.SourceID
	}
	return b.RepoArg
}
