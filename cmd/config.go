package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Scribe configuration",
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set a configuration value",
	}

	setEditorCmd := &cobra.Command{
		Use:   "editor <value>",
		Short: "Set the preferred editor",
		Long: `Set the editor used for editing skills.

Example:
  scribe config set editor cursor
  scribe config set editor vim`,
		Args: cobra.ExactArgs(1),
		RunE: runConfigSetEditor,
	}

	setCmd.AddCommand(setEditorCmd)
	cmd.AddCommand(setCmd)

	return cmd
}

func runConfigSetEditor(cmd *cobra.Command, args []string) error {
	editor := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.Editor = editor

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Editor set to %q\n", editor)
	return nil
}
