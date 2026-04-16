package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [skill...]",
		Short: "Install skills from connected registries",
		Long: `Install one or more skills from your connected registries.

Without arguments, shows an interactive picker of all available skills.
Pass skill names to install specific skills, or use --all to install everything.`,
		Example: `  scribe install              # interactive picker
  scribe install tdd commit   # install specific skills
  scribe install --all        # install everything available`,
		RunE: runInstall,
	}
	cmd.Flags().Bool("all", false, "Install all available skills without prompting")
	cmd.Flags().String("registry", "", "Limit to a specific registry (owner/repo)")
	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")
	repoFlag, _ := cmd.Flags().GetString("registry")

	bag := &workflow.Bag{
		Args:           args,
		InstallAllFlag: allFlag,
		RepoFlag:       repoFlag,
		Factory:        newCommandFactory(),
		FilterRegistries: filterRegistries,
	}
	if err := workflow.Run(cmd.Context(), workflow.InstallSteps(), bag); err != nil {
		return err
	}
	return saveWorkflowState(bag)
}
