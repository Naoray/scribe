package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/workflow"
)

type initResult struct {
	Package    manifest.PackageMeta `json:"package"`
	Skills     []manifest.Skill     `json:"skills"`
	ScribeFile string               `json:"scribe_file"`
}

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a skill package manifest",
		Long: `Scaffold a package manifest for publishing your own skills.

The command discovers SKILL.md files under the current directory and writes a
top-level scribe.yaml package manifest.`,
		Args: cobra.NoArgs,
		RunE: runInit,
	}
	cmd.Flags().Bool("force", false, "Overwrite an existing scribe manifest")
	return markJSONSupported(cmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	force, _ := cmd.Flags().GetBool("force")
	jsonFlag := jsonFlagPassed(cmd)
	useJSON := workflow.UseJSONOutputForProcess(jsonFlag)

	result, err := buildInitResult(wd, useJSON)
	if err != nil {
		return err
	}

	if err := ensureInitManifestWritable(wd, force); err != nil {
		return err
	}
	data, err := manifest.ScaffoldPackageManifest(result.Package, result.Skills)
	if err != nil {
		return clierrors.Wrap(err, "PACKAGE_NAME_INVALID", clierrors.ExitValid,
			clierrors.WithMessage(err.Error()),
			clierrors.WithRemediation("Use letters, numbers, dots, underscores, or dashes."),
		)
	}
	if err := os.WriteFile(filepath.Join(wd, result.ScribeFile), data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", result.ScribeFile, err)
	}

	if useJSON {
		r := jsonRendererForCommand(cmd, jsonFlag)
		if err := r.Result(result); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found skills: %s\n", formatInitSkillList(result.Skills))
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", result.ScribeFile)
	return nil
}

func buildInitResult(wd string, skipPrompt bool) (initResult, error) {
	skills, err := discoverPackageSkills(wd)
	if err != nil {
		return initResult{}, err
	}
	meta := defaultInitPackageMeta(wd)
	if !skipPrompt {
		if err := promptInitPackageMeta(&meta); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return initResult{}, clierrors.Wrap(err, "USER_CANCELED", clierrors.ExitCanceled,
					clierrors.WithMessage("user canceled init"),
				)
			}
			return initResult{}, err
		}
	}
	return initResult{Package: meta, Skills: skills, ScribeFile: manifest.ManifestFilename}, nil
}

func defaultInitPackageMeta(wd string) manifest.PackageMeta {
	return manifest.PackageMeta{
		Name:   filepath.Base(wd),
		Author: gitConfigValue(wd, "user.name"),
	}
}

func promptInitPackageMeta(meta *manifest.PackageMeta) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Package name").
				Value(&meta.Name).
				Validate(manifest.ValidatePackageName),
			huh.NewInput().
				Title("Description").
				Value(&meta.Description),
			huh.NewInput().
				Title("Author").
				Value(&meta.Author),
		),
	).Run()
}

func gitConfigValue(wd, key string) string {
	cmd := exec.Command("git", "config", key)
	cmd.Dir = wd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func discoverPackageSkills(root string) ([]manifest.Skill, error) {
	var skills []manifest.Skill

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && shouldSkipInitDiscoveryDir(d.Name()) {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}

		skillFile := filepath.Join(path, "SKILL.md")
		info, err := os.Stat(skillFile)
		if err != nil || info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := filepath.Base(path)
		if meta, err := discovery.ReadSkillMetadata(path); err == nil && strings.TrimSpace(meta.Name) != "" {
			name = strings.TrimSpace(meta.Name)
		}
		skills = append(skills, manifest.Skill{Name: name, Path: rel})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name != skills[j].Name {
			return skills[i].Name < skills[j].Name
		}
		return skills[i].Path < skills[j].Path
	})
	return skills, nil
}

func shouldSkipInitDiscoveryDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == ".scribe" || name == "versions"
}

func ensureInitManifestWritable(wd string, force bool) error {
	for _, name := range []string{manifest.ManifestFilename, manifest.LegacyManifestFilename} {
		path := filepath.Join(wd, name)
		if _, err := os.Stat(path); err == nil && !force {
			return clierrors.Wrap(fmt.Errorf("%s already exists", name), "MANIFEST_EXISTS", clierrors.ExitConflict,
				clierrors.WithMessage(fmt.Sprintf("%s already exists", name)),
				clierrors.WithRemediation("Pass --force to overwrite the existing manifest."),
				clierrors.WithResource(name),
			)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", name, err)
		}
	}
	return nil
}

func formatInitSkillList(skills []manifest.Skill) string {
	if len(skills) == 0 {
		return "(none)"
	}
	paths := make([]string, len(skills))
	for i, skill := range skills {
		paths[i] = filepath.ToSlash(filepath.Join(skill.Path, "SKILL.md"))
	}
	return strings.Join(paths, ", ")
}
