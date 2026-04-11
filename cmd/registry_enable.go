package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRegistryEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <owner/repo>",
		Short: "Enable a connected registry",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegistryEnable,
	}
}

func newRegistryDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <owner/repo>",
		Short: "Disable a connected registry (keeps config, skips during sync)",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegistryDisable,
	}
}

func runRegistryEnable(cmd *cobra.Command, args []string) error {
	return setRegistryEnabled(args[0], true)
}

func runRegistryDisable(cmd *cobra.Command, args []string) error {
	return setRegistryEnabled(args[0], false)
}

func setRegistryEnabled(repo string, enabled bool) error {
	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}

	// Collect all known repos for resolution.
	allRepos := make([]string, 0, len(cfg.Registries))
	for _, r := range cfg.Registries {
		allRepos = append(allRepos, r.Repo)
	}

	resolved, err := resolveRegistry(repo, allRepos)
	if err != nil {
		return err
	}

	rc := cfg.FindRegistry(resolved)
	if rc == nil {
		return fmt.Errorf("registry %q not found in config", resolved)
	}

	rc.Enabled = enabled

	if err := cfg.Save(); err != nil {
		return err
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Registry %s %s\n", resolved, action)
	return nil
}
