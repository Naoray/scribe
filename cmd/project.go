package cmd

import "github.com/spf13/cobra"

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Scribe project artifacts",
	}
	cmd.AddCommand(newProjectInitCommand(), newProjectSyncCommand(), newProjectSkillCommand())
	return cmd
}
