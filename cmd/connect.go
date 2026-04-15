package cmd

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/workflow"
)

func newConnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "connect [owner/repo]",
		Short: "Connect to a team skills repo",
		Long: `Connect to a team skills repo so Scribe can sync your local skills.

The repo must contain a scribe.toml with a [team] section.

Examples:
  scribe connect ArtistfyHQ/team-skills
  scribe connect                          # interactive prompt`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConnect,
	}
}

func runConnect(cmd *cobra.Command, args []string) error {
	repo, err := resolveRepo(args)
	if err != nil {
		return err
	}

	bag := &workflow.Bag{
		RepoArg: repo,
		Factory: newCommandFactory(),
	}
	if err := workflow.Run(cmd.Context(), workflow.ConnectSteps(), bag); err != nil {
		return err
	}
	return saveWorkflowState(bag)
}

// resolveRepo returns the owner/repo string from args or an interactive prompt.
func resolveRepo(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("no repo specified — usage: scribe connect <owner/repo>")
	}

	var repo string
	err := huh.NewInput().
		Title("Team skills repo").
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
