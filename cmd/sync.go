package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	isync "github.com/Naoray/scribe/internal/sync"
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
	cmd.Flags().String("alias", "", "Install incoming skill under this name when a local directory conflicts")
	cmd.Flags().MarkHidden("all")
	return markJSONSupported(cmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)
	repoFlag, _ := cmd.Flags().GetString("registry")
	trustAllFlag, _ := cmd.Flags().GetBool("trust-all")
	forceBudget, _ := cmd.Flags().GetBool("force")
	aliasName, _ := cmd.Flags().GetString("alias")
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
		AliasName:        aliasName,
		Factory:          factory,
		FilterRegistries: filterRegistries,
	}
	if err := workflow.Run(cmd.Context(), workflow.SyncSteps(), bag); err != nil {
		var lockErr *isync.LockMismatchError
		if errors.As(err, &lockErr) {
			if jsonFlag {
				_ = renderMutatorEnvelope(cmd, map[string]any{
					"refused":  lockErr.Refused,
					"registry": lockErr.Registry,
				}, envelope.StatusError)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "refused by scribe.lock for %s\n", lockErr.Registry)
				for _, item := range lockErr.Refused {
					fmt.Fprintf(cmd.ErrOrStderr(), "- %s: %s\n", item.Name, item.Reason)
				}
			}
			return clierrors.Wrap(err, "LOCKFILE_MISMATCH", clierrors.ExitConflict, clierrors.WithRendered(jsonFlag))
		}
		return handleNameConflictError(cmd, err)
	}
	if bag.Partial {
		if err := saveWorkflowState(bag); err != nil {
			return err
		}
		return clierrors.Wrap(clierrors.ErrPartialSuccess, "SYNC_PARTIAL", clierrors.ExitPartial, clierrors.WithRendered(true))
	}
	return saveWorkflowState(bag)
}
