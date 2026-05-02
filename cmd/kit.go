package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/paths"
)

type kitCreateOptions struct {
	description string
	skills      []string
	registry    string
	force       bool
	json        bool
}

type kitCreateOutput struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SkillsCount int    `json:"skills_count"`
}

func newKitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kit",
		Short: "Manage local skill kits",
	}
	cmd.AddCommand(newKitCreateCommand())
	return cmd
}

func newKitCreateCommand() *cobra.Command {
	opts := &kitCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a local skill kit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			return runKitCreate(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.description, "description", "", "Kit description")
	cmd.Flags().StringSliceVar(&opts.skills, "skills", nil, "Comma-separated skill names")
	cmd.Flags().StringVar(&opts.registry, "registry", "", "Source registry for this kit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite an existing kit")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runKitCreate(cmd *cobra.Command, name string, opts *kitCreateOptions) error {
	if opts == nil {
		opts = &kitCreateOptions{}
	}

	if err := validateKitName(name); err != nil {
		return err
	}

	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}
	kitPath := filepath.Join(scribeDir, "kits", name+".yaml")

	if !opts.force {
		if _, err := os.Stat(kitPath); err == nil {
			return clierrors.Wrap(fmt.Errorf("kit %q already exists", name), "KIT_EXISTS", clierrors.ExitConflict,
				clierrors.WithResource(kitPath),
				clierrors.WithRemediation("Use --force to overwrite the existing kit."),
			)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check kit path: %w", err)
		}
	}

	k := kit.Kit{
		Name:        name,
		Description: opts.description,
		Skills:      opts.skills,
	}
	if opts.registry != "" {
		k.Source = &kit.Source{Registry: opts.registry}
	}

	if err := kit.Save(kitPath, &k); err != nil {
		return err
	}

	out := kitCreateOutput{
		Name:        name,
		Path:        kitPath,
		SkillsCount: len(opts.skills),
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created kit %s at %s with %d skills\n", out.Name, out.Path, out.SkillsCount)
	return nil
}

func validateKitName(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.ContainsRune(name, rune(os.PathSeparator)) || strings.Contains(name, "..") {
		return clierrors.Wrap(fmt.Errorf("invalid kit name %q", name), "KIT_NAME_INVALID", clierrors.ExitValid,
			clierrors.WithResource(name),
			clierrors.WithRemediation("Use a simple kit name without path separators or parent directory segments."),
		)
	}
	return nil
}
