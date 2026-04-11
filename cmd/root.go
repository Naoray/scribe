package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/storemigrate"
	"github.com/Naoray/scribe/internal/tools"
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
		// Skip meta commands.
		if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "migrate" || cmd.Name() == "upgrade" {
			return nil
		}

		// Run on-disk store migration (v1 slug/<name>/ → v2 flat <name>/) before
		// any command touches the store. Idempotent — gated by a marker file.
		if err := runStoreMigration(); err != nil {
			return err
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

// runStoreMigration executes the v1 → v2 on-disk migration if the marker is
// absent. Warnings are surfaced on stderr; any error fails the command loud
// so we don't silently operate on a half-migrated store.
func runStoreMigration() error {
	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}

	// Fast path: marker already written, nothing to do.
	if storemigrate.AlreadyMigrated(storeDir) {
		return nil
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	warnings, err := storemigrate.Migrate(storeDir, st)
	if err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "scribe: %s\n", w)
	}

	// Persist the state so the v2 schema_version (set by parseAndMigrate on load)
	// is saved to disk alongside the on-disk migration.
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
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
		newUpgradeCommand(),
	)
}
