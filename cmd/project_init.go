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

	available, err := discoverProjectInitKits(cmd)
	if err != nil {
		return err
	}
	selected, err := selectProjectInitKits(cmd, opts.kits, available)
	if err != nil {
		return err
	}

	selectedKits := make([]string, 0, len(selected))
	for _, k := range selected {
		selectedKits = append(selectedKits, k.LocalName)
	}

	// Write .scribe.yaml *before* installing remote kits. If a remote install
	// fails mid-loop, the project file still points at the intended kits so
	// the user can re-run `scribe kit install` (or `project init --force`)
	// without losing the selection.
	projectPath := filepath.Join(cwd, projectfile.Filename)
	if err := projectfile.Save(projectPath, &projectfile.ProjectFile{Kits: selectedKits}); err != nil {
		return err
	}
	if err := installRemoteProjectInitKits(cmd, selected); err != nil {
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

type projectInitKit struct {
	Display   string
	Selector  string
	LocalName string
	Registry  string
	Remote    bool
}

func discoverProjectInitKits(cmd *cobra.Command) ([]projectInitKit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	loaded, err := kitpkg.LoadAll(filepath.Join(home, ".scribe", "kits"))
	if err != nil {
		return nil, err
	}

	kits := make([]projectInitKit, 0, len(loaded))
	for name, k := range loaded {
		if strings.TrimSpace(name) == "" {
			continue
		}
		entry := projectInitKit{Display: name, Selector: name, LocalName: name}
		if k != nil && k.Source != nil {
			entry.Registry = k.Source.Registry
		}
		kits = append(kits, entry)
	}

	remote, err := remoteKitListItems(cmd, &kitListOptions{}, loaded)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: skip remote kits: %v\n", err)
	}
	for _, r := range remote {
		ref := r.Registry + ":" + r.Name
		kits = append(kits, projectInitKit{
			Display:   ref + " (remote)",
			Selector:  ref,
			LocalName: r.Name,
			Registry:  r.Registry,
			Remote:    true,
		})
	}

	sort.Slice(kits, func(i, j int) bool {
		if kits[i].Remote != kits[j].Remote {
			return !kits[i].Remote
		}
		return kits[i].Display < kits[j].Display
	})
	return kits, nil
}

func selectProjectInitKits(cmd *cobra.Command, rawKits string, available []projectInitKit) ([]projectInitKit, error) {
	bySelector := make(map[string]projectInitKit, len(available))
	for _, k := range available {
		bySelector[k.Selector] = k
	}

	if strings.TrimSpace(rawKits) != "" {
		selected := splitProjectInitKits(rawKits)
		byLocalName := make(map[string]projectInitKit, len(available))
		for _, k := range available {
			if !k.Remote {
				byLocalName[k.LocalName] = k
			}
		}
		out := make([]projectInitKit, 0, len(selected))
		for _, raw := range selected {
			if k, ok := bySelector[raw]; ok {
				out = append(out, k)
				continue
			}
			if strings.Contains(raw, ":") {
				idx := strings.LastIndex(raw, ":")
				registryName := raw[:idx]
				localName := raw[idx+1:]
				// Only fall back to a locally installed kit when its source
				// registry matches the explicitly requested registry —
				// otherwise the user typed `acme/skills:foo` and would
				// silently get a `foo` kit sourced from another registry.
				if k, ok := byLocalName[localName]; ok && strings.EqualFold(k.Registry, registryName) {
					out = append(out, k)
					continue
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: unknown remote kit %s — registry not connected or kit missing\n", raw)
				continue
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: unknown kit %s\n", raw)
			out = append(out, projectInitKit{Display: raw, Selector: raw, LocalName: raw})
		}
		return out, nil
	}

	if len(available) == 0 || !isatty.IsTerminal(os.Stdin.Fd()) {
		return nil, nil
	}

	opts := make([]huh.Option[string], len(available))
	for i, k := range available {
		opts[i] = huh.NewOption(k.Display, k.Selector)
	}
	var selectors []string
	if err := huh.NewMultiSelect[string]().
		Title("Select kits for this project").
		Options(opts...).
		Value(&selectors).
		Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, clierrors.Wrap(err, "USER_CANCELED", clierrors.ExitCanceled,
				clierrors.WithMessage("user canceled project init"),
			)
		}
		return nil, err
	}
	out := make([]projectInitKit, 0, len(selectors))
	for _, sel := range selectors {
		if k, ok := bySelector[sel]; ok {
			out = append(out, k)
		}
	}
	return out, nil
}

func installRemoteProjectInitKits(cmd *cobra.Command, selected []projectInitKit) error {
	for _, k := range selected {
		if !k.Remote {
			continue
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Installing remote kit %s\n", k.Selector)
		if err := runKitInstall(cmd, k.Selector, &kitInstallOptions{noInteraction: true, silent: true}); err != nil {
			return clierrors.Wrap(err, "PROJECT_INIT_KIT_INSTALL_FAILED", clierrors.ExitGeneral,
				clierrors.WithMessage(fmt.Sprintf("install remote kit %s: %v", k.Selector, err)),
				clierrors.WithRemediation("Run `scribe kit install "+k.Selector+"` manually to inspect the failure."),
			)
		}
	}
	return nil
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
