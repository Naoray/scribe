package cmd

import "github.com/spf13/cobra"

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage team-sharable project artifacts",
	}
	cmd.AddCommand(newProjectSyncCommand(), newProjectSkillCommand())
	return cmd
}
