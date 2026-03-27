package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "scribe",
	Short: "Team skill sync for AI coding agents",
	Long: `Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.

Get started:
  scribe init --repo owner/team-skills
  scribe sync
  scribe list

Use "scribe <command> --help" for details on any command.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
}
