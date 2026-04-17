package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// packageReclassifyMigrationName names the one-shot state migration that
// reclassifies legacy skills/<name>/ installs into packages/<name>/ when
// their on-disk shape identifies them as packages. The state layer gates on
// state.Migrations[packageReclassifyMigrationName] so this runs exactly
// once per machine.
const packageReclassifyMigrationName = "packages_store_split_2026_04"

// ReclassifyLegacyPackages scans state entries classified as skills, detects
// any whose on-disk shape is actually a package (nested SKILL.md / install
// script), and moves them to ~/.scribe/packages/<name>/. Tool projections
// are removed and state is updated. Each move emits a
// PackageReclassifiedMsg so the UI can surface a hint.
//
// Safe to call repeatedly — the caller guards with state.HasMigration and
// marks it done after the first successful pass.
func (s *Syncer) ReclassifyLegacyPackages(st *state.State) error {
	if st.HasMigration(packageReclassifyMigrationName) {
		return nil
	}

	storeDir, err := tools.StoreDir()
	if err != nil {
		return err
	}
	pkgsDir, err := tools.PackagesDir()
	if err != nil {
		return err
	}

	for name, skill := range st.Installed {
		if skill.IsPackage() {
			continue
		}
		oldDir := filepath.Join(storeDir, name)
		info, statErr := os.Stat(oldDir)
		if statErr != nil || !info.IsDir() {
			continue
		}
		kind, detectErr := DetectKindFromDir(oldDir)
		if detectErr != nil || kind != KindPackage {
			continue
		}

		newDir := filepath.Join(pkgsDir, name)
		if err := reclassifyOnDisk(oldDir, newDir); err != nil {
			return fmt.Errorf("reclassify %s: %w", name, err)
		}
		removeStaleProjections(skill)

		skill.Kind = state.KindPackage
		skill.Tools = []string{}
		skill.Paths = []string{}
		skill.ManagedPaths = nil
		st.Installed[name] = skill

		s.emit(PackageReclassifiedMsg{
			Name:        name,
			OldPath:     oldDir,
			NewPath:     newDir,
			InstallHint: "run its setup/install script if it needs to re-wire tools",
		})
	}

	st.MarkMigration(packageReclassifyMigrationName)
	return st.Save()
}

// reclassifyOnDisk moves srcDir → dstDir. Parent of dstDir is created if
// needed. If dstDir already exists (e.g. from a half-completed prior run),
// it is removed first so the move is deterministic.
func reclassifyOnDisk(srcDir, dstDir string) error {
	if err := os.MkdirAll(filepath.Dir(dstDir), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dstDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(srcDir, dstDir)
}

// removeStaleProjections deletes every tool-facing symlink Scribe created
// for this skill. Best-effort — a missing link is fine. Errors other than
// ErrNotExist are ignored here because the migration should not abort on
// a single stale link.
func removeStaleProjections(skill state.InstalledSkill) {
	managed := skill.ManagedPaths
	if len(managed) == 0 {
		managed = skill.Paths
	}
	for _, p := range managed {
		if p == "" {
			continue
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Swallow — migration should keep going.
			_ = err
		}
	}
}
