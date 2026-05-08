package cmd

import "github.com/spf13/cobra"

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage the Scribe project file (.scribe.yaml)",
	}
	cmd.AddCommand(newProjectInitCommand())
	return cmd
}
