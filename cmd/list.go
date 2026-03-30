package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
	listCmd.Flags().StringVar(&registryFlag, "registry", "", "Show only this registry (owner/repo or repo name)")
	listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
	listCmd.Flags().MarkHidden("all")
}

func runList(cmd *cobra.Command, args []string) error {
	bag := &workflow.Bag{
		Args:     args,
		JSONFlag: listJSON,
		RepoFlag: registryFlag,
		FilterRegistries: func(flag string, repos []string) ([]string, error) {
			return filterRegistries(flag, repos)
		},
	}
	return workflow.Run(cmd.Context(), workflow.ListSteps(), bag)
}
