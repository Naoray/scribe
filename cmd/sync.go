package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local skills to match team loadout",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().Bool("json", false, "Output machine-readable JSON (for CI/agents)")
	syncCmd.Flags().String("registry", "", "Sync only this registry (owner/repo or repo name)")
	syncCmd.Flags().Bool("trust-all", false, "Approve all package install commands without prompting")
	syncCmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
	syncCmd.Flags().MarkHidden("all")
}

func runSync(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	repoFlag, _ := cmd.Flags().GetString("registry")
	trustAllFlag, _ := cmd.Flags().GetBool("trust-all")

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		RepoFlag:         repoFlag,
		TrustAllFlag:     trustAllFlag,
		FilterRegistries: filterRegistries,
	}
	return workflow.Run(cmd.Context(), workflow.SyncSteps(), bag)
}
