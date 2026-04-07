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
		Short: "Show installed skills and their status vs team loadout",
		RunE:  runList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().String("registry", "", "Show only this registry (owner/repo or repo name)")
	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	repoFlag, _ := cmd.Flags().GetString("registry")

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         useJSON,
		RepoFlag:         repoFlag,
		FilterRegistries: filterRegistries,
	}

	if useJSON {
		return workflow.Run(cmd.Context(), workflow.ListJSONSteps(), bag)
	}

	if err := workflow.Run(cmd.Context(), workflow.ListLoadSteps(), bag); err != nil {
		return err
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
