package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:           "scribe",
	Short:         "Team skill sync for AI coding agents",
	Long:          "Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.",
	Version:       Version,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true, // errors printed once below; prevents double-print when RunE re-enters Execute
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.RunE = runHub
	rootCmd.Flags().Bool("json", false, "Output machine-readable JSON")

	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(guideCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(newExplainCommand())
}
