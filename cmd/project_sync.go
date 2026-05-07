package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/projectstore"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type projectSyncOptions struct {
	check   bool
	force   bool
	json    bool
	vendors []string
}

type projectSyncOutput struct {
	ProjectRoot      string   `json:"project_root"`
	KitsWritten      []string `json:"kits_written,omitempty"`
	SkillsVendored   []string `json:"skills_vendored,omitempty"`
	RegistryPinned   []string `json:"registry_pinned,omitempty"`
	BootstrapSkipped []string `json:"bootstrap_skipped,omitempty"`
	Drift            []string `json:"drift,omitempty"`
}

func newProjectSyncCommand() *cobra.Command {
	opts := &projectSyncOptions{}
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Publish project kits and skill pins into .ai",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			return runProjectSync(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.check, "check", false, "Validate .ai artifacts without writing")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite changed project artifacts")
	cmd.Flags().StringArrayVar(&opts.vendors, "vendor", nil, "Vendor a local skill by name (repeatable)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runProjectSync(cmd *cobra.Command, opts *projectSyncOptions) error {
	if opts == nil {
		opts = &projectSyncOptions{}
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	projectPath, err := projectfile.Find(wd)
	if err != nil {
		return err
	}
	if projectPath == "" {
		return clierrors.Wrap(errors.New("no .scribe.yaml found"), "PROJECT_FILE_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithRemediation("Run inside a scribe project or create .scribe.yaml first."),
		)
	}
	projectRoot := filepath.Dir(projectPath)
	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return err
	}
	factory := commandFactory()
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}
	globalKits, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return err
	}
	storeDir, err := tools.StoreDir()
	if err != nil {
		return err
	}
	out := projectSyncOutput{ProjectRoot: projectRoot}
	if err := syncProjectKits(projectRoot, scribeDir, pf.Kits, globalKits, opts, &out); err != nil {
		return err
	}
	skillNames := resolveProjectSkillNames(pf, globalKits)
	vendorSet := map[string]bool{}
	for _, name := range opts.vendors {
		vendorSet[projectSkillName(name)] = true
	}
	lockEntries, err := syncProjectSkills(projectRoot, storeDir, skillNames, vendorSet, st, opts, &out)
	if err != nil {
		return err
	}
	if err := writeProjectLock(projectRoot, lockEntries, opts, &out); err != nil {
		return err
	}
	if opts.check && len(out.Drift) > 0 {
		if opts.json {
			_ = renderMutatorEnvelope(cmd, out, envelope.StatusError)
		}
		return clierrors.Wrap(errors.New("project artifacts are out of date"), "PROJECT_SYNC_DRIFT", clierrors.ExitValid,
			clierrors.WithRendered(opts.json),
			clierrors.WithRemediation("Run `scribe project sync` and commit the resulting .ai changes."),
		)
	}
	if opts.json {
		status := envelope.StatusOK
		if len(out.Drift) > 0 {
			status = envelope.StatusNoChange
		}
		return renderMutatorEnvelope(cmd, out, status)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Synced project artifacts for %s\n", projectRoot)
	return nil
}

func syncProjectKits(projectRoot, scribeDir string, kitNames []string, global map[string]*kit.Kit, opts *projectSyncOptions, out *projectSyncOutput) error {
	for _, name := range sortedProjectStrings(kitNames) {
		k, ok := global[name]
		if !ok {
			return clierrors.Wrap(fmt.Errorf("kit %q not found", name), "PROJECT_KIT_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithRemediation("Create the kit locally or remove it from .scribe.yaml."),
			)
		}
		src := filepath.Join(scribeDir, "kits", name+".yaml")
		dst := filepath.Join(projectRoot, ".ai", "kits", name+".yaml")
		_ = k
		changed, err := copyFileIfChanged(src, dst, opts)
		if err != nil {
			return err
		}
		if changed {
			out.KitsWritten = append(out.KitsWritten, name)
		}
	}
	return nil
}

func syncProjectSkills(projectRoot, storeDir string, skillNames []string, vendorSet map[string]bool, st *state.State, opts *projectSyncOptions, out *projectSyncOutput) ([]lockfile.ProjectEntry, error) {
	entries := make([]lockfile.ProjectEntry, 0)
	for _, ref := range skillNames {
		name := projectSkillName(ref)
		if name == "" {
			continue
		}
		installed, ok := st.Installed[name]
		if !ok {
			return nil, clierrors.Wrap(fmt.Errorf("skill %q is not installed", name), "PROJECT_SKILL_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithRemediation("Install the skill locally before publishing project artifacts."),
			)
		}
		switch {
		case installed.Origin == state.OriginBootstrap:
			out.BootstrapSkipped = append(out.BootstrapSkipped, name)
		case installed.Origin == state.OriginProject || vendorSet[name]:
			if vendorSet[name] && installed.Origin == state.OriginLocal {
				installed.Origin = state.OriginProject
				st.Installed[name] = installed
				if !opts.check {
					if err := st.Save(); err != nil {
						return nil, err
					}
				}
			}
			drifted, err := vendorProjectSkill(projectRoot, filepath.Join(storeDir, name), name, opts)
			if err != nil {
				return nil, err
			}
			if drifted {
				out.Drift = append(out.Drift, filepath.Join(projectRoot, ".ai", "skills", name))
			}
			out.SkillsVendored = append(out.SkillsVendored, name)
		case installed.Origin == state.OriginLocal:
			return nil, clierrors.Wrap(fmt.Errorf("skill %q is local", name), "PROJECT_SKILL_LOCAL", clierrors.ExitConflict,
				clierrors.WithRemediation("Run `scribe project skill claim "+name+"` or pass `scribe project sync --vendor "+name+"`."),
			)
		default:
			entry, err := projectEntryFromState(name, installed)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
			out.RegistryPinned = append(out.RegistryPinned, name)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func vendorProjectSkill(projectRoot, srcDir, name string, opts *projectSyncOptions) (bool, error) {
	dstDir := filepath.Join(projectRoot, ".ai", "skills", name)
	if opts.check {
		srcHash, err := lockfile.HashSet(srcDir)
		if err != nil {
			return false, err
		}
		marker, err := projectstore.ReadMarker(dstDir)
		if err != nil || marker.Hash != srcHash {
			return true, nil
		}
		return false, nil
	}
	if err := copyDir(srcDir, dstDir, opts.force); err != nil {
		return false, err
	}
	hash, err := lockfile.HashSet(dstDir)
	if err != nil {
		return false, err
	}
	return false, projectstore.WriteMarker(dstDir, hash, "scribe", time.Now())
}

func projectEntryFromState(name string, installed state.InstalledSkill) (lockfile.ProjectEntry, error) {
	if len(installed.Sources) == 0 {
		return lockfile.ProjectEntry{}, clierrors.Wrap(fmt.Errorf("skill %q has no registry source", name), "PROJECT_SKILL_UNPINNABLE", clierrors.ExitConflict,
			clierrors.WithRemediation("Reinstall from a connected registry, or claim/vendor it as a project skill."),
		)
	}
	src := installed.Sources[0]
	if src.Registry == "" || src.LastSHA == "" {
		return lockfile.ProjectEntry{}, clierrors.Wrap(fmt.Errorf("skill %q has incomplete registry source", name), "PROJECT_SKILL_UNPINNABLE", clierrors.ExitConflict,
			clierrors.WithRemediation("Reinstall from a connected registry, or claim/vendor it as a project skill."),
		)
	}
	dir, err := installableDir(name, installed)
	if err != nil {
		return lockfile.ProjectEntry{}, err
	}
	hash, err := lockfile.HashSet(dir)
	if err != nil {
		return lockfile.ProjectEntry{}, err
	}
	entryType := "skill"
	if installed.IsPackage() {
		entryType = "package"
	}
	return lockfile.ProjectEntry{
		Entry: lockfile.Entry{
			Name:               name,
			SourceRegistry:     src.Registry,
			CommitSHA:          src.LastSHA,
			ContentHash:        hash,
			InstallCommandHash: sync.CommandHash(installed.InstallCmd, installed.UpdateCmd, nil, nil),
		},
		SourceRepo: src.SourceRepo,
		Path:       src.Path,
		Type:       entryType,
		Install:    installed.InstallCmd,
		Update:     installed.UpdateCmd,
	}, nil
}

func installableDir(name string, installed state.InstalledSkill) (string, error) {
	if installed.IsPackage() {
		dir, err := tools.PackagesDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, name), nil
	}
	dir, err := tools.StoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func writeProjectLock(projectRoot string, entries []lockfile.ProjectEntry, opts *projectSyncOptions, out *projectSyncOutput) error {
	store := projectstore.Project(projectRoot)
	lf := &lockfile.ProjectLockfile{
		FormatVersion: lockfile.SchemaVersion,
		Kind:          lockfile.ProjectKind,
		GeneratedBy:   "scribe",
		Entries:       entries,
	}
	data, err := lf.Encode()
	if err != nil {
		return err
	}
	path := store.LockfilePath()
	current, err := os.ReadFile(path)
	if err == nil && string(current) == string(data) {
		return nil
	}
	if opts.check {
		out.Drift = append(out.Drift, path)
		return nil
	}
	if err == nil && !opts.force {
		return clierrors.Wrap(fmt.Errorf("%s would change", path), "PROJECT_LOCK_DRIFT", clierrors.ExitConflict,
			clierrors.WithResource(path),
			clierrors.WithRemediation("Review the existing lockfile, then rerun with --force to overwrite."),
		)
	}
	return store.WriteProjectLockfile(lf)
}

func resolveProjectSkillNames(pf *projectfile.ProjectFile, kits map[string]*kit.Kit) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range pf.Add {
		add(name)
	}
	for _, kitName := range pf.Kits {
		if k, ok := kits[kitName]; ok {
			for _, skill := range k.Skills {
				add(skill)
			}
		}
	}
	sort.Strings(names)
	return names
}

func projectSkillName(ref string) string {
	ref = strings.TrimSpace(ref)
	if _, name, ok := strings.Cut(ref, ":"); ok {
		return strings.TrimSpace(name)
	}
	return ref
}

func copyFileIfChanged(src, dst string, opts *projectSyncOptions) (bool, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	current, err := os.ReadFile(dst)
	if err == nil && string(current) == string(data) {
		return false, nil
	}
	if opts.check {
		return true, nil
	}
	if err == nil && !opts.force {
		return false, clierrors.Wrap(fmt.Errorf("%s would be overwritten", dst), "PROJECT_ARTIFACT_CONFLICT", clierrors.ExitConflict,
			clierrors.WithResource(dst),
			clierrors.WithRemediation("Rerun with --force after reviewing the project copy."),
		)
	}
	if err == nil && opts.force {
		if backupErr := os.WriteFile(dst+".bak."+time.Now().UTC().Format("20060102150405"), current, 0o644); backupErr != nil {
			return false, backupErr
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(dst, data, 0o644)
}

func copyDir(src, dst string, force bool) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("read source skill %s: %w", src, err)
	}
	if _, err := os.Stat(dst); err == nil {
		if !force {
			same, err := dirsSameContent(src, dst)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
			return clierrors.Wrap(fmt.Errorf("%s already exists", dst), "PROJECT_ARTIFACT_CONFLICT", clierrors.ExitConflict,
				clierrors.WithResource(dst),
				clierrors.WithRemediation("Rerun with --force after reviewing the project copy."),
			)
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if d.Name() == "versions" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

func dirsSameContent(a, b string) (bool, error) {
	ha, err := lockfile.HashSet(a)
	if err != nil {
		return false, err
	}
	hb, err := lockfile.HashSet(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func sortedProjectStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
