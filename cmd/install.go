package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/app"
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
  scribe install --all        # install everything available, except skills you removed`,
		RunE: runInstall,
	}
	cmd.Flags().Bool("all", false, "Install all available skills without prompting")
	cmd.Flags().String("registry", "", "Limit to a specific registry (owner/repo)")
	cmd.Flags().Bool("force", false, "Project skills even when an agent budget is exceeded")
	cmd.Flags().String("alias", "", "Install incoming skill under this name when a local directory conflicts")
	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")
	repoFlag, _ := cmd.Flags().GetString("registry")
	forceBudget, _ := cmd.Flags().GetBool("force")
	aliasName, _ := cmd.Flags().GetString("alias")
	factory := newCommandFactory()

	if err := enforceCurrentBudget(factory, forceBudget); err != nil {
		return err
	}

	if len(args) > 0 {
		if err := clearRemovedBeforeInstall(factory, args, repoFlag); err != nil {
			return err
		}
	}

	bag := &workflow.Bag{
		Args:             args,
		InstallAllFlag:   allFlag,
		RepoFlag:         repoFlag,
		ForceBudget:      forceBudget,
		AliasName:        aliasName,
		Factory:          factory,
		FilterRegistries: filterRegistries,
	}
	if err := workflow.Run(cmd.Context(), workflow.InstallSteps(), bag); err != nil {
		return err
	}
	return saveWorkflowState(bag)
}

func clearRemovedBeforeInstall(factory *app.Factory, names []string, repoFlag string) error {
	if len(names) == 0 {
		return nil
	}
	st, err := factory.State()
	if err != nil {
		return err
	}

	registry := ""
	if repoFlag != "" {
		cfg, err := factory.Config()
		if err != nil {
			return err
		}
		repos := make([]string, 0, len(cfg.Registries))
		for _, r := range cfg.Registries {
			repos = append(repos, r.Repo)
		}
		resolved, err := resolveRegistry(repoFlag, repos)
		if err != nil {
			return err
		}
		registry = resolved
	}

	changed := false
	for _, name := range names {
		if st.ClearRemovedByUser(name, registry) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return st.Save()
}
