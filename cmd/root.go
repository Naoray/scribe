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

var rootCmd = newRootCmd()

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "scribe",
		Short:         "Manage local AI coding agent skills",
		Long:          "Scribe manages local AI coding agent skills and keeps shared team registries in sync.",
		Version:       Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			if c.Name() == "help" || c.Name() == "version" || c.Name() == "migrate" || c.Name() == "upgrade" {
				return nil
			}

			factory := newCommandFactory()
			if err := runStoreMigration(factory); err != nil {
				return err
			}

			isFirstRun := firstrun.IsFirstRun()

			cfg, err := factory.Config()
			if err != nil {
				return err
			}
			st, err := factory.State()
			if err != nil {
				return err
			}

			added, builtinsFirstRun := firstrun.ApplyBuiltins(cfg)
			removed, removedRan := firstrun.ApplyBuiltinsRemove(cfg, st, []string{"openai/codex-skills"})
			renamed, renamedRan := firstrun.ApplyBuiltinsRename(cfg, st, map[string]string{"anthropic/skills": "anthropics/skills"})
			if len(added) > 0 {
				out := c.ErrOrStderr()
				if builtinsFirstRun {
					fmt.Fprintln(out, "Welcome to Scribe! Adding built-in registries...")
					for _, repo := range added {
						fmt.Fprintf(out, "  + %s\n", repo)
					}
					fmt.Fprintln(out)
				}
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			if len(removed) > 0 {
				for _, repo := range removed {
					fmt.Fprintf(c.ErrOrStderr(), "scribe: removed %s (no manifest) from connected registries\n", repo)
				}
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			if len(renamed) > 0 && len(added) == 0 && len(removed) == 0 {
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			if builtinsFirstRun || removedRan || renamedRan {
				if err := st.Save(); err != nil {
					return err
				}
			}

			if !isFirstRun {
				return nil
			}

			if isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
				toolSet, _ := tools.ResolveActive(cfg)
				_ = firstrun.PromptAdoption(cfg, st, toolSet, os.Stdin, os.Stdout)
			}

			return nil
		},
	}

	cmd.RunE = runDefault
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.CompletionOptions = cobra.CompletionOptions{HiddenDefaultCmd: true}

	aliasConnect := newConnectCommand()
	aliasConnect.Hidden = true
	aliasConnect.Deprecated = "use 'scribe registry connect' instead"
	cmd.AddCommand(aliasConnect)

	aliasMigrate := newMigrateCommand()
	aliasMigrate.Hidden = true
	aliasMigrate.Deprecated = "use 'scribe registry migrate' instead"
	cmd.AddCommand(aliasMigrate)

	cmd.AddCommand(
		newListCommand(),
		newBrowseCommand(),
		newInstallCommand(),
		newAddCommand(),
		newRemoveCommand(),
		newSyncCommand(),
		newAdoptCommand(),
		newStatusCommand(),
		newResolveCommand(),
		newRestoreCommand(),
		newSkillCommand(),
		newToolsCommand(),
		newGuideCommand(),
		newConfigCommand(),
	)

	cmd.AddCommand(newRegistryCommand())

	cmd.AddCommand(
		newCreateCommand(),
		newExplainCommand(),
		newDoctorCommand(),
		newUpgradeCommand(),
		newUpgradeAgentCommand(),
	)

	return cmd
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
