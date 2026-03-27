package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncJSON bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local skills to match team loadout",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: sync")
		return nil
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output machine-readable JSON (non-TTY mode for CI/agents)")
}
