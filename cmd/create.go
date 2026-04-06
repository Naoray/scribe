package cmd

import "github.com/spf13/cobra"

func newCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create team resources",
		Long:  "Create team resources like skill registries.",
	}
	cmd.AddCommand(newCreateRegistryCommand())
	return cmd
}
