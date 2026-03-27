package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	listJSON   bool
	listStatus string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	Long: `Show installed skills and their status vs team loadout.

Examples:
  scribe list
  scribe list --json
  scribe list --status outdated
  scribe list --status missing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: list")
		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status: current, outdated, missing, extra")
}
