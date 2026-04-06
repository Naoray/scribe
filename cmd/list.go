package cmd

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/workflow"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	RunE:  runList,
}

func init() {
	listCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	listCmd.Flags().Bool("local", false, "Show locally installed skills (offline, no registry needed)")
	listCmd.Flags().String("registry", "", "Show only this registry (owner/repo or repo name)")
	listCmd.Flags().String("group", "", "Jump directly to a group (e.g. gstack, standalone)")
	listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
	listCmd.Flags().MarkHidden("all")
	listCmd.MarkFlagsMutuallyExclusive("local", "registry")
}

func runList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	localFlag, _ := cmd.Flags().GetBool("local")
	repoFlag, _ := cmd.Flags().GetString("registry")
	groupFlag, _ := cmd.Flags().GetString("group")

	isTTY := isatty.IsTerminal(os.Stdout.Fd())

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		LocalFlag:        localFlag,
		RepoFlag:         repoFlag,
		FilterRegistries: filterRegistries,
	}

	// Wire up TUI for local list when running in a terminal.
	if isTTY && !jsonFlag {
		bag.ListTUI = func(skills []discovery.Skill) error {
			m := newListModel(skills, groupFlag, bag.State)
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
	}

	if err := workflow.Run(cmd.Context(), workflow.ListSteps(), bag); err != nil {
		return err
	}

	// Render results populated by the workflow step.
	if bag.LocalSkills != nil {
		return printLocalTable(os.Stdout, bag.LocalSkills)
	}
	if bag.RegistryDiffs != nil {
		return printMultiListTable(os.Stdout, bag.Repos, bag.RegistryDiffs, bag.State, bag.MultiRegistry)
	}
	return nil
}
