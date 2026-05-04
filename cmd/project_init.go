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
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/workflow"
)

type projectInitOptions struct {
	force bool
	kits  string
	json  bool
}

type projectInitResult struct {
	Kits             []string `json:"kits"`
	ProjectFile      string   `json:"project_file"`
	GitignoreUpdated bool     `json:"gitignore_updated"`
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

	gitignoreUpdated, err := ensureProjectFileGitignored(cwd)
	if err != nil {
		return err
	}

	result := projectInitResult{
		Kits:             selectedKits,
		ProjectFile:      projectfile.Filename,
		GitignoreUpdated: gitignoreUpdated,
	}
	if opts.json && workflow.UseJSONOutputForProcess(opts.json) {
		r := jsonRendererForCommand(cmd, opts.json)
		if err := r.Result(result); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", projectfile.Filename)
	if gitignoreUpdated {
		fmt.Fprintf(cmd.OutOrStdout(), "Added %s to .gitignore\n", projectfile.Filename)
	}
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
	entries, err := os.ReadDir(filepath.Join(home, ".scribe", "kits"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read kits directory: %w", err)
	}

	kits := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			kits = append(kits, entry.Name())
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

func ensureProjectFileGitignored(cwd string) (bool, error) {
	if _, err := os.Stat(filepath.Join(cwd, ".git")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check git repository: %w", err)
	}

	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(gitignorePath, []byte(projectfile.Filename+"\n"), 0o644); err != nil {
			return false, fmt.Errorf("write .gitignore: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read .gitignore: %w", err)
	}
	if gitignoreContainsProjectFile(data) {
		return false, nil
	}

	appendData := []byte(projectfile.Filename + "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		appendData = []byte("\n" + projectfile.Filename + "\n")
	}
	file, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("open .gitignore: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(appendData); err != nil {
		return false, fmt.Errorf("append .gitignore: %w", err)
	}
	return true, nil
}

func gitignoreContainsProjectFile(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == projectfile.Filename {
			return true
		}
	}
	return false
}
