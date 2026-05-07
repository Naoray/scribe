package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	kitpkg "github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/workflow"
)

type projectInitOptions struct {
	force bool
	kits  string
	json  bool
}

type projectInitResult struct {
	Kits        []string `json:"kits"`
	ProjectFile string   `json:"project_file"`
}

func newProjectInitCommand() *cobra.Command {
	opts := &projectInitOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a .scribe.yaml project file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			return runProjectInit(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite an existing project file")
	cmd.Flags().StringVar(&opts.kits, "kits", "", "Comma-separated kits to include")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runProjectInit(cmd *cobra.Command, opts *projectInitOptions) error {
	if opts == nil {
		opts = &projectInitOptions{}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	if err := ensureProjectFileWritable(cwd, opts.force); err != nil {
		return err
	}

	availableKits, err := discoverProjectInitKits()
	if err != nil {
		return err
	}
	selectedKits, err := selectProjectInitKits(cmd, opts.kits, availableKits)
	if err != nil {
		return err
	}

	projectPath := filepath.Join(cwd, projectfile.Filename)
	if err := projectfile.Save(projectPath, &projectfile.ProjectFile{Kits: selectedKits}); err != nil {
		return err
	}

	result := projectInitResult{
		Kits:        selectedKits,
		ProjectFile: projectfile.Filename,
	}
	if opts.json && workflow.UseJSONOutputForProcess(opts.json) {
		r := jsonRendererForCommand(cmd, opts.json)
		if err := r.Result(result); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", projectfile.Filename)
	return nil
}

func ensureProjectFileWritable(cwd string, force bool) error {
	existing, err := projectfile.Find(cwd)
	if err != nil {
		return err
	}
	if existing != "" && !force {
		return clierrors.Wrap(fmt.Errorf("%s already exists", projectfile.Filename), "PROJECT_FILE_EXISTS", clierrors.ExitConflict,
			clierrors.WithMessage(fmt.Sprintf("%s already exists", projectfile.Filename)),
			clierrors.WithRemediation("Pass --force to overwrite the existing project file."),
			clierrors.WithResource(existing),
		)
	}
	return nil
}

func discoverProjectInitKits() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	loaded, err := kitpkg.LoadAll(filepath.Join(home, ".scribe", "kits"))
	if err != nil {
		return nil, err
	}

	kits := make([]string, 0, len(loaded))
	for name := range loaded {
		if strings.TrimSpace(name) != "" {
			kits = append(kits, name)
		}
	}
	sort.Strings(kits)
	return kits, nil
}

func selectProjectInitKits(cmd *cobra.Command, rawKits string, availableKits []string) ([]string, error) {
	if strings.TrimSpace(rawKits) != "" {
		selected := splitProjectInitKits(rawKits)
		warnUnknownProjectInitKits(cmd, selected, availableKits)
		return selected, nil
	}

	if len(availableKits) == 0 || !isatty.IsTerminal(os.Stdin.Fd()) {
		return []string{}, nil
	}

	opts := make([]huh.Option[string], len(availableKits))
	for i, kit := range availableKits {
		opts[i] = huh.NewOption(kit, kit)
	}
	var selected []string
	if err := huh.NewMultiSelect[string]().
		Title("Select kits for this project").
		Options(opts...).
		Value(&selected).
		Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, clierrors.Wrap(err, "USER_CANCELED", clierrors.ExitCanceled,
				clierrors.WithMessage("user canceled project init"),
			)
		}
		return nil, err
	}
	return selected, nil
}

func splitProjectInitKits(raw string) []string {
	parts := strings.Split(raw, ",")
	kits := make([]string, 0, len(parts))
	for _, part := range parts {
		if kit := strings.TrimSpace(part); kit != "" {
			kits = append(kits, kit)
		}
	}
	return kits
}

func warnUnknownProjectInitKits(cmd *cobra.Command, selected, available []string) {
	known := make(map[string]bool, len(available))
	for _, kit := range available {
		known[kit] = true
	}
	for _, kit := range selected {
		if !known[kit] {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: unknown kit %s\n", kit)
		}
	}
}
