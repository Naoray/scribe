package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initPackage bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Connect to a team skills repo or scaffold a new skill package",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: init")
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initPackage, "package", false, "Scaffold a new skill package instead of connecting to a team repo")
}
