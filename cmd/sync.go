package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

func newSyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync local skills to match team loadout",
		RunE:  runSync,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON (for CI/agents)")
	cmd.Flags().String("registry", "", "Sync only this registry (owner/repo or repo name)")
	cmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
	cmd.Flags().MarkHidden("all")
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	repoFlag, _ := cmd.Flags().GetString("registry")

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		RepoFlag:         repoFlag,
		FilterRegistries: filterRegistries,
	}
	return workflow.Run(cmd.Context(), workflow.SyncSteps(), bag)
}
