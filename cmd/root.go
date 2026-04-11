package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/firstrun"
	"github.com/Naoray/scribe/internal/storemigrate"
	"github.com/Naoray/scribe/internal/tools"
)

// Version is set at build time via ldflags.
var Version = "dev"

func newCommandFactory() *app.Factory {
	return app.NewFactory()
}

var rootCmd = &cobra.Command{
	Use:           "scribe",
	Short:         "Manage local AI coding agent skills",
	Long:          "Scribe manages local AI coding agent skills and keeps shared team registries in sync.",
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
		factory := newCommandFactory()
		if err := runStoreMigration(factory); err != nil {
			return err
		}

		if !firstrun.IsFirstRun() {
			return nil
		}

		cfg, err := factory.Config()
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

		if err := cfg.Save(); err != nil {
			return err
		}

		// One-shot adoption prompt: ask user to adopt any existing unmanaged skills.
		// Only runs in interactive TTY. Ignores persisted cfg.Adoption.Mode — firstrun
		// always prompts so the user isn't silently skipped or silently auto-adopted.
		if isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
			st, stErr := factory.State()
			if stErr == nil {
				toolSet, _ := tools.ResolveActive(cfg)
				_ = firstrun.PromptAdoption(cfg, st, toolSet, os.Stdin, os.Stdout)
			}
		}

		return nil
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
func runStoreMigration(factory *app.Factory) error {
	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}

	// Fast path: marker already written, nothing to do.
	if storemigrate.AlreadyMigrated(storeDir) {
		return nil
	}

	st, err := factory.State()
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
	rootCmd.RunE = runDefault
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
		newAdoptCommand(),
		newStatusCommand(),
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
