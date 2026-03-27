package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	initPackage bool
	initRepo    string
	initName    string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Connect to a team skills repo or scaffold a new skill package",
	Long: `Connect to a team skills repo or scaffold a new skill package.

Without flags, starts an interactive wizard. Pass --repo to skip prompts.

Examples:
  scribe init --repo ArtistfyHQ/team-skills
  scribe init --repo ArtistfyHQ/team-skills --name artistfy
  scribe init --package`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: init")
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initPackage, "package", false, "Scaffold a new skill package instead of connecting to a team repo")
	initCmd.Flags().StringVar(&initRepo, "repo", "", "GitHub owner/repo to connect to (e.g. ArtistfyHQ/team-skills)")
	initCmd.Flags().StringVar(&initName, "name", "", "Team name (default: inferred from repo)")
}
