package cmd

import (
	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
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
	cmd.Flags().Bool("trust-all", false, "Approve all package install commands without prompting")
	cmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
	cmd.Flags().Bool("force", false, "Project skills even when an agent budget is exceeded")
	cmd.Flags().MarkHidden("all")
	return markJSONSupported(cmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)
	repoFlag, _ := cmd.Flags().GetString("registry")
	trustAllFlag, _ := cmd.Flags().GetBool("trust-all")
	forceBudget, _ := cmd.Flags().GetBool("force")
	factory := commandFactory()

	if err := enforceCurrentBudget(factory, forceBudget); err != nil {
		return err
	}

	bag := &workflow.Bag{
		Args:             args,
		JSONFlag:         jsonFlag,
		RepoFlag:         repoFlag,
		TrustAllFlag:     trustAllFlag,
		ForceBudget:      forceBudget,
		Factory:          factory,
		FilterRegistries: filterRegistries,
	}
	if err := workflow.Run(cmd.Context(), workflow.SyncSteps(), bag); err != nil {
		return err
	}
	if bag.Partial {
		if err := saveWorkflowState(bag); err != nil {
			return err
		}
		return clierrors.Wrap(clierrors.ErrPartialSuccess, "SYNC_PARTIAL", clierrors.ExitPartial, clierrors.WithRendered(true))
	}
	return saveWorkflowState(bag)
}
