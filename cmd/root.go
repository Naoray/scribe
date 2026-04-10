package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:           "scribe",
	Short:         "Team skill sync for AI coding agents",
	Long:          "Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.",
	Version:       Version,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true, // errors printed once below; prevents double-print when RunE re-enters Execute
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip first-run for meta commands.
		if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "migrate" {
			return nil
		}

		if !firstrun.IsFirstRun() {
			return nil
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		firstrun.ApplyBuiltins(cfg)

		if isatty.IsTerminal(os.Stdout.Fd()) {
			fmt.Println("Welcome to Scribe! Adding built-in community registries...")
			for _, r := range cfg.EnabledRegistries() {
				if r.Builtin {
					fmt.Printf("  + %s\n", r.Repo)
				}
			}
			fmt.Println()
		}

		return cfg.Save()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.RunE = runHub
	rootCmd.Flags().Bool("json", false, "Output machine-readable JSON")

	// Backward-compat aliases: hidden + deprecated, point to registry subcommands.
	aliasConnect := newConnectCommand()
	aliasConnect.Hidden = true
	aliasConnect.Deprecated = "use 'scribe registry connect' instead"
	rootCmd.AddCommand(aliasConnect)

	aliasMigrate := newMigrateCommand()
	aliasMigrate.Hidden = true
	aliasMigrate.Deprecated = "use 'scribe registry migrate' instead"
	rootCmd.AddCommand(aliasMigrate)

	// Top-level: daily skill management.
	rootCmd.AddCommand(
		newListCommand(),
		newAddCommand(),
		newRemoveCommand(),
		newSyncCommand(),
		newResolveCommand(),
		newRestoreCommand(),
		newToolsCommand(),
		newGuideCommand(),
		newConfigCommand(),
	)

	// Registry subcommand: administration & publishing.
	rootCmd.AddCommand(newRegistryCommand())

	// Other.
	rootCmd.AddCommand(
		newCreateCommand(),
		newExplainCommand(),
	)
}
