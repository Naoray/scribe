package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
	return &cobra.Command{
		Use:   "resync <owner/repo>",
		Short: "Clear mute state for a registry so it will be retried",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return resyncRegistry(args[0])
		},
	}
}

func forgetRegistry(repo string) error {
	factory := newCommandFactory()
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

	if err := cfg.Save(); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}

	fmt.Printf("Registry %s forgotten\n", resolved)
	return nil
}

func resyncRegistry(repo string) error {
	factory := newCommandFactory()
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

	st.ClearRegistryFailure(resolved)
	if err := st.Save(); err != nil {
		return err
	}

	fmt.Printf("Registry %s will be retried on the next sync\n", resolved)
	return nil
}
