package cmd

import "github.com/spf13/cobra"

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Scribe status for this machine",
		Long: `Show Scribe status for this machine.

Includes connected registries, installed skill count, and last sync time.

Examples:
  scribe status
  scribe status --json`,
		Args: cobra.NoArgs,
		RunE: runStatus,
	}
	return markJSONSupported(cmd)
}
