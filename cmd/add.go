package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	addPath string
	addYes  bool
)

var addCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Add a skill to the team loadout",
	Long: `Add a skill to the team loadout's scribe.toml.

Source can be a GitHub reference or an already-installed skill name.
If the skill already exists at the same version, this is a no-op.

Examples:
  scribe add github:garrytan/gstack@v0.12.9.0
  scribe add github:Naoray/scribe-skills@v1.0.0 --path skills/laravel-init
  scribe add gstack
  scribe add github:garrytan/gstack@v0.12.9.0 --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("TODO: add %s\n", args[0])
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addPath, "path", "", "Sub-path within the repo for multi-skill packages")
	addCmd.Flags().BoolVar(&addYes, "yes", false, "Skip confirmation prompts")
}
