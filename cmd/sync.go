package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	syncJSON   bool
	syncDryRun bool
	syncYes    bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local skills to match team loadout",
	Long: `Sync local skills to match team loadout.

Compares installed skills against the team's scribe.toml and installs
missing skills, updates outdated ones. Safe to run repeatedly (idempotent).

Examples:
  scribe sync
  scribe sync --dry-run
  scribe sync --yes
  scribe sync --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: sync")
		return nil
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output machine-readable JSON (non-TTY mode for CI/agents)")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Preview changes without installing anything")
	syncCmd.Flags().BoolVar(&syncYes, "yes", false, "Skip confirmation prompts")
}
