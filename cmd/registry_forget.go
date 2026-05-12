package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/registryindex"
	"github.com/Naoray/scribe/internal/workflow"
)

func newRegistryForgetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "forget <owner/repo>",
		Short: "Forget a connected registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return forgetRegistry(args[0])
		},
	}
}

func newRegistryResyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resync <owner/repo>",
		Short: "Clear mute state for a registry so it will be retried",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return resyncRegistry(cmd, args[0])
		},
	}
	cmd.Flags().Bool("force-kits", false, "Overwrite existing kit files while refreshing registry kits")
	cmd.Flags().Bool("refresh-kits", false, "Refresh registry-published kits during resync")
	return markJSONSupported(cmd)
}

func forgetRegistry(repo string) error {
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	st, err := factory.State()
	if err != nil {
		return err
	}

	allRepos := make([]string, 0, len(cfg.Registries))
	for _, r := range cfg.Registries {
		allRepos = append(allRepos, r.Repo)
	}
	resolved, err := resolveRegistry(repo, allRepos)
	if err != nil {
		return err
	}

	kept := cfg.Registries[:0]
	found := false
	for _, rc := range cfg.Registries {
		if rc.Repo == resolved {
			found = true
			continue
		}
		kept = append(kept, rc)
	}
	if !found {
		return fmt.Errorf("registry %q not found in config", resolved)
	}
	cfg.Registries = kept
	st.ClearRegistryFailure(resolved)
	st.ClearRemovedByRegistry(resolved)
	if path, err := registryindex.Path(); err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if err := registryindex.Remove(path, resolved); err != nil {
				return err
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}

	fmt.Printf("Registry %s forgotten\n", resolved)
	return nil
}

func resyncRegistry(cmd *cobra.Command, repo string) error {
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	st, err := factory.State()
	if err != nil {
		return err
	}

	allRepos := make([]string, 0, len(cfg.Registries))
	for _, r := range cfg.Registries {
		allRepos = append(allRepos, r.Repo)
	}
	resolved, err := resolveRegistry(repo, allRepos)
	if err != nil {
		return err
	}

	refreshKits, _ := cmd.Flags().GetBool("refresh-kits")
	forceKits, _ := cmd.Flags().GetBool("force-kits")
	jsonFlag := jsonFlagPassed(cmd)
	failureCleared := st.ClearRegistryFailure(resolved)

	if !refreshKits {
		if !jsonFlag {
			fmt.Fprintln(cmd.ErrOrStderr(), "W: `scribe registry resync` will refresh kits by default in the next minor release; pass --refresh-kits to opt in now")
		}
		if failureCleared {
			if err := st.Save(); err != nil {
				return err
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Registry %s will be retried on the next sync\n", resolved)
		return nil
	}

	bag := &workflow.Bag{
		RepoArg:     resolved,
		JSONFlag:    jsonFlag,
		ForceKits:   forceKits,
		RefreshKits: true,
		Factory:     factory,
	}
	if failureCleared {
		bag.MarkStateDirty()
	}
	steps := []workflow.Step{
		{Name: "LoadConfig", Fn: workflow.StepLoadConfig},
		{Name: "ResolveFormatter", Fn: workflow.StepResolveFormatter},
		{Name: "FetchManifest", Fn: workflow.StepFetchManifest},
		{Name: "ValidateManifest", Fn: workflow.StepValidateManifest},
		{Name: "LoadState", Fn: workflow.StepLoadState},
		{Name: "InstallKits", Fn: workflow.StepInstallKits},
		{Name: "ShowAvailable", Fn: workflow.StepShowAvailableSkills},
	}
	if err := workflow.Run(cmd.Context(), steps, bag); err != nil {
		return err
	}
	if err := saveWorkflowState(bag); err != nil {
		return err
	}
	if bag.Partial {
		return clierrors.Wrap(clierrors.ErrPartialSuccess, "RESYNC_PARTIAL", clierrors.ExitPartial, clierrors.WithRendered(true))
	}
	return nil
}
