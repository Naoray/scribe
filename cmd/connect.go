package cmd

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/workflow"
)

func newConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect [owner/repo]",
		Short: "Connect to a skill registry",
		Long: `Connect to a skill registry so Scribe can sync skills from it.

The repo must contain a scribe.yaml or scribe.toml with a [team] section.

Examples:
  scribe connect ArtistfyHQ/team-skills
  scribe connect mattpocock/skills
  scribe connect mattpocock/skills --install-all
  scribe connect                          # interactive prompt`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConnect,
	}
	cmd.Flags().Bool("install-all", false, "Install every discovered skill from the connected registry")
	cmd.Flags().Bool("force-kits", false, "Overwrite existing kit files from this registry")
	addSourceFlags(cmd, false)
	return markJSONSupported(cmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	sourceFlags, err := readSourceFlags(cmd)
	if err != nil {
		return err
	}
	repo, err := resolveRepoWithFlags(args, sourceFlags)
	if err != nil {
		return err
	}
	installAll, _ := cmd.Flags().GetBool("install-all")
	forceKits, _ := cmd.Flags().GetBool("force-kits")
	jsonFlag := jsonFlagPassed(cmd)
	spec, ident, display, err := sourceSpecFromFlags(sourceFlags)
	if err != nil {
		return err
	}
	if sourceFlags.hasTyped() && installAll && (spec.Type != source.SourceGitHub || spec.Path != "" || spec.Ref != "") {
		return fmt.Errorf("--install-all currently supports legacy GitHub owner/repo sources only")
	}

	bag := &workflow.Bag{
		RepoArg:        repo,
		JSONFlag:       jsonFlag,
		InstallAllFlag: installAll,
		ForceKits:      forceKits,
		Factory:        commandFactory(),
	}
	if sourceFlags.hasTyped() {
		bag.SourceArg = spec
		bag.SourceKey = ident.Key
		bag.SourceID = spec.ID
		if spec.Type == source.SourceGitHub {
			bag.RepoArg = spec.Repo
		} else {
			bag.RepoArg = display
		}
	}
	steps := workflow.ConnectSteps()
	if installAll {
		steps = workflow.ConnectInstallAllSteps()
	}
	if err := workflow.Run(cmd.Context(), steps, bag); err != nil {
		return err
	}
	if bag.Partial {
		if err := saveWorkflowState(bag); err != nil {
			return err
		}
		return clierrors.Wrap(clierrors.ErrPartialSuccess, "CONNECT_PARTIAL", clierrors.ExitPartial, clierrors.WithRendered(true))
	}
	return saveWorkflowState(bag)
}

// resolveRepo returns the owner/repo string from args or an interactive prompt.
func resolveRepo(args []string) (string, error) {
	return resolveRepoWithFlags(args, sourceFlagValues{})
}

func resolveRepoWithFlags(args []string, sourceFlags sourceFlagValues) (string, error) {
	if sourceFlags.hasTyped() {
		if len(args) > 0 {
			return "", fmt.Errorf("repo argument cannot be combined with source flags")
		}
		spec, _, _, err := sourceSpecFromFlags(sourceFlags)
		if err != nil {
			return "", err
		}
		if spec.Type == source.SourceGitHub {
			return spec.Repo, nil
		}
		if spec.Repo != "" {
			return spec.Repo, nil
		}
		if spec.URL != "" {
			return spec.URL, nil
		}
		return spec.Path, nil
	}
	if len(args) > 0 {
		return manifest.NormalizeGitHubRepo(args[0])
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		err := fmt.Errorf("no repo specified")
		return "", clierrors.Wrap(err, "USAGE_CONNECT_REPO_REQUIRED", clierrors.ExitUsage,
			clierrors.WithMessage(err.Error()),
			clierrors.WithRemediation("usage: scribe connect <owner/repo>"),
		)
	}

	var repo string
	err := huh.NewInput().
		Title("Skill registry repo").
		Placeholder("owner/repo").
		Validate(func(s string) error {
			_, _, err := manifest.ParseOwnerRepo(s)
			return err
		}).
		Value(&repo).
		Run()
	if err != nil {
		return "", err
	}
	return repo, nil
}
