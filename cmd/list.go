package cmd

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show all skills on this machine",
		Long: `Show all skills on this machine.

By default, lists every skill installed locally. Use --remote to see
available skills from connected registries instead.

Examples:
  scribe list                # local skills
  scribe list --remote       # registry diff view
  scribe list --registry r   # filter to one registry (implies --remote)
  scribe list --json         # machine-readable output`,
		RunE: runList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().Bool("remote", false, "Show available skills from registries (not installed)")
	cmd.Flags().String("registry", "", "Show only this registry (owner/repo or repo name)")
	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	remoteFlag, _ := cmd.Flags().GetBool("remote")
	repoFlag, _ := cmd.Flags().GetString("registry")

	// --registry implies --remote (you can't filter registries in local view).
	if repoFlag != "" {
		remoteFlag = true
	}

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         useJSON,
		RemoteFlag:       remoteFlag,
		RepoFlag:         repoFlag,
		LazyGitHub:       !remoteFlag,
		Factory:          newCommandFactory(),
		FilterRegistries: filterRegistries,
	}

	if useJSON {
		if err := workflow.Run(cmd.Context(), workflow.ListJSONSteps(), bag); err != nil {
			return err
		}
		return saveWorkflowState(bag)
	}

	m := newListModel(cmd.Context(), bag)
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
	_, err := p.Run()
	if errors.Is(err, tea.ErrInterrupted) {
		os.Exit(130)
	}
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
