package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	cmd.AddCommand(newConfigAdoptionCommand())

	return cmd
}

func runConfigSetEditor(cmd *cobra.Command, args []string) error {
	editor := args[0]
	factory := newCommandFactory()

	cfg, err := factory.Config()
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

func newConfigAdoptionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adoption",
		Short: "View or update adoption settings",
		Long: `View or update the skill adoption configuration.

Examples:
  scribe config adoption                       # print current settings
  scribe config adoption --mode auto
  scribe config adoption --mode prompt
  scribe config adoption --mode off
  scribe config adoption --add-path ~/src/my-skills
  scribe config adoption --remove-path ~/src/my-skills`,
		Args: cobra.NoArgs,
		RunE: runConfigAdoption,
	}

	cmd.Flags().String("mode", "", "Adoption mode: auto, prompt, or off")
	cmd.Flags().String("add-path", "", "Append a path to adoption.paths")
	cmd.Flags().String("remove-path", "", "Remove a path from adoption.paths")
	cmd.MarkFlagsMutuallyExclusive("mode", "add-path", "remove-path")

	return cmd
}

func runConfigAdoption(cmd *cobra.Command, args []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	addPath, _ := cmd.Flags().GetString("add-path")
	removePath, _ := cmd.Flags().GetString("remove-path")

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch {
	case mode != "":
		switch mode {
		case "auto", "prompt", "off":
			// valid
		default:
			return fmt.Errorf("invalid mode %q: must be auto, prompt, or off", mode)
		}
		cfg.Adoption.Mode = mode
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

	case addPath != "":
		for _, p := range cfg.Adoption.Paths {
			if p == addPath {
				fmt.Fprintf(cmd.OutOrStdout(), "adoption path already present: %s\n", addPath)
				return nil
			}
		}
		cfg.Adoption.Paths = append(cfg.Adoption.Paths, addPath)
		// Validate via re-load: save then load to surface boundary errors.
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		if _, err := cfg.AdoptionPaths(); err != nil {
			// Roll back: reload previous state and re-save without the bad path.
			cfg.Adoption.Paths = cfg.Adoption.Paths[:len(cfg.Adoption.Paths)-1]
			_ = cfg.Save()
			return fmt.Errorf("invalid path: %w", err)
		}

	case removePath != "":
		found := false
		filtered := make([]string, 0, len(cfg.Adoption.Paths))
		for _, p := range cfg.Adoption.Paths {
			if p == removePath {
				found = true
				continue
			}
			filtered = append(filtered, p)
		}
		if !found {
			return fmt.Errorf("path not in config: %s", removePath)
		}
		cfg.Adoption.Paths = filtered
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	// Print current state (bare invocation or after mutation).
	printAdoptionState(cmd, cfg)
	return nil
}

func printAdoptionState(cmd *cobra.Command, cfg interface {
	AdoptionMode() string
	AdoptionPaths() ([]string, error)
}) {
	mode := cfg.AdoptionMode()
	paths, _ := cfg.AdoptionPaths()

	fmt.Fprintf(cmd.OutOrStdout(), "mode:  %s\n", mode)
	if len(paths) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "paths: (none)\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "paths:\n")
		for _, p := range paths {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", p)
		}
	}
}

