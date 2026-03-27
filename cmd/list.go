package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: list")
		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
}
