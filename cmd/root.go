package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "scribe",
	Short:   "Team skill sync for AI coding agents",
	Long:    "Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.",
	Version: Version,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// connectCmd moved under registry — add hidden alias for backward compat
	aliasConnect := *connectCmd
	aliasConnect.Hidden = true
	aliasConnect.Deprecated = "use 'scribe registry connect' instead"
	rootCmd.AddCommand(&aliasConnect)

	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(guideCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(migrateCmd)
}
