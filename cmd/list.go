package cmd

import (
	"github.com/spf13/cobra"

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
	listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
	listCmd.Flags().MarkHidden("all")
	listCmd.MarkFlagsMutuallyExclusive("local", "registry")
}

func runList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	localFlag, _ := cmd.Flags().GetBool("local")
	repoFlag, _ := cmd.Flags().GetString("registry")

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		LocalFlag:        localFlag,
		RepoFlag:         repoFlag,
		FilterRegistries: filterRegistries,
	}
	return workflow.Run(cmd.Context(), workflow.ListSteps(), bag)
}
