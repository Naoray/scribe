package cmd

import "github.com/spf13/cobra"

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create team resources",
	Long:  "Create team resources like skill registries.",
}

func init() {
	createCmd.AddCommand(createRegistryCmd)
}
