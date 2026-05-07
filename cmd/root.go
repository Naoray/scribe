package cmd

//go:generate go run ../internal/cli/schema/cmd/gen-claudemd

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/agent"
	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/cli/output"
	"github.com/Naoray/scribe/internal/firstrun"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/storemigrate"
	"github.com/Naoray/scribe/internal/tools"
)

// Version is set at build time via ldflags.
var Version = "dev"

func readBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

func resolveVersion(initial string, info *debug.BuildInfo) string {
	if initial != "dev" || info == nil {
		return initial
	}
	if version := info.Main.Version; version != "" && version != "(devel)" {
		return version
	}
	return initial
}

func newCommandFactory() *app.Factory {
	return app.NewFactory()
}

var commandFactory = newCommandFactory

var rootCmd = newRootCmd()

const (
	jsonSupportedAnnotation = "json_supported"
	commandModeAnnotation   = "mode"
	commandModeReadOnly     = "read-only"
	commandModeApplyWrites  = "read-only-without-apply"
)

type legacyGlobalProjectionCompatKey struct{}

func addNoInteractionFlag(cmd *cobra.Command, usage string, keepLegacyShort bool) {
	cmd.Flags().BoolP("no-interaction", "n", false, usage)
	if keepLegacyShort {
		cmd.Flags().BoolP("yes", "y", false, "Deprecated alias for --no-interaction")
	} else {
		cmd.Flags().Bool("yes", false, "Deprecated alias for --no-interaction")
	}
	_ = cmd.Flags().MarkHidden("yes")
}

func noInteractionFlagPassed(cmd *cobra.Command) bool {
	noInteraction, _ := cmd.Flags().GetBool("no-interaction")
	yes, _ := cmd.Flags().GetBool("yes")
	return noInteraction || yes
}

func Execute() {
	err := rootCmd.Execute()
	mode := envFromArgs(os.Args)
	r := output.New(mode, os.Stdout, os.Stderr)
	if err != nil {
		err = classifyExecuteError(err)
		var ce *clierrors.Error
		if stderrors.As(err, &ce) {
			if !ce.Rendered {
				_ = r.Error(ce)
			}
			if ce.Exit == clierrors.ExitOK {
				ce.Exit = clierrors.ExitGeneral
			}
			os.Exit(ce.Exit)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(clierrors.ExitGeneral)
	}
	_ = r.Flush()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "scribe",
		Short:         "Manage local AI coding agent skills",
		Long:          "Scribe manages local AI coding agent skills and keeps shared team registries in sync.",
		Version:       resolveVersion(Version, readBuildInfo()),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			ctx := context.WithValue(c.Context(), envelope.BootstrapStartKey, time.Now())
			ctx = context.WithValue(ctx, envelope.ScribeVersionKey, resolveVersion(Version, readBuildInfo()))
			ctx = context.WithValue(ctx, envelope.CommandPathKey, c.CommandPath())
			c.SetContext(ctx)

			if jsonFlagPassed(c) && !commandSupportsJSON(c) {
				return &clierrors.Error{
					Code:        "JSON_NOT_SUPPORTED",
					Message:     c.CommandPath() + " does not support --json yet",
					Remediation: "scribe schema --all --json | jq 'keys' lists JSON-capable commands; this command is on the wave-3 migration list (see solo todo #469)",
					Exit:        clierrors.ExitUsage,
				}
			}

			if c.Name() == "help" || c.Name() == "version" || c.Name() == "migrate" || c.Name() == "upgrade" {
				return nil
			}

			factory := commandFactory()
			if commandReadOnly(c) {
				stateFileExists, err := state.FileExists()
				if err != nil {
					return err
				}
				if !stateFileExists {
					return nil
				}
			}
			st, err := factory.State()
			if err != nil {
				return err
			}
			migrationResult, err := state.MigrateEmbeddedSkillRename(st)
			if err != nil {
				return err
			}
			if migrationResult.Conflict {
				fmt.Fprintf(c.ErrOrStderr(), "scribe: embedded skill rename warning: both %q and %q exist in state; leaving both unchanged\n", state.OldEmbeddedSkillName, state.EmbeddedSkillName)
			}
			for _, warning := range migrationResult.Warnings {
				fmt.Fprintf(c.ErrOrStderr(), "scribe: embedded skill rename warning: %s\n", warning)
			}
			if migrationResult.Changed {
				if err := st.Save(); err != nil {
					return err
				}
			}
			if commandReadOnly(c) {
				return nil
			}
			if err := runStoreMigration(factory); err != nil {
				return err
			}

			isFirstRun := firstrun.IsFirstRun()

			cfg, err := factory.Config()
			if err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			compat, err := state.DetectLegacyGlobalProjectionCompat(st, wd)
			if err != nil {
				return err
			}
			c.SetContext(context.WithValue(c.Context(), legacyGlobalProjectionCompatKey{}, compat))
			if compat.Enabled {
				emit, err := state.ShouldEmitLegacyGlobalProjectionCompatBanner(time.Now())
				if err != nil {
					return err
				}
				if emit {
					fmt.Fprintln(c.ErrOrStderr(), state.LegacyGlobalProjectionCompatBanner)
				}
			}

			added, builtinsFirstRun := firstrun.ApplyBuiltins(cfg)
			removed, removedRan := firstrun.ApplyBuiltinsRemove(cfg, st, []string{"openai/codex-skills"})
			renamed, renamedRan := firstrun.ApplyBuiltinsRename(cfg, st, map[string]string{"anthropic/skills": "anthropics/skills"})
			naorayRemoved, naorayRan := firstrun.RemoveNaorayScribeRegistry(cfg, st)
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
			if len(naorayRemoved) > 0 {
				fmt.Fprintf(c.ErrOrStderr(), "scribe: removed Naoray/scribe from connected registries (scribe is now managed by the binary)\n")
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			if len(renamed) > 0 && len(added) == 0 && len(removed) == 0 && len(naorayRemoved) == 0 {
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			agentStateDirty := builtinsFirstRun || removedRan || renamedRan || naorayRan
			if agentStateDirty {
				if err := st.Save(); err != nil {
					return err
				}
			}

			// Auto-install or refresh the embedded scribe skill on every run.
			// EnsureScribeAgent is idempotent — it skips the write when the content
			// matches the embedded version, so there is no meaningful overhead.
			if storeDir, sdErr := tools.StoreDir(); sdErr == nil {
				if changed, ensureErr := agent.EnsureScribeAgent(storeDir, st, cfg); ensureErr != nil {
					fmt.Fprintf(c.ErrOrStderr(), "scribe: scribe bootstrap warning: %v\n", ensureErr)
				} else if changed {
					if err := st.Save(); err != nil {
						fmt.Fprintf(c.ErrOrStderr(), "scribe: scribe state save warning: %v\n", err)
					}
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
	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")
	cmd.CompletionOptions = cobra.CompletionOptions{HiddenDefaultCmd: true}

	aliasConnect := newConnectCommand()
	aliasConnect.Hidden = true
	aliasConnect.Deprecated = "use 'scribe registry connect' instead"
	cmd.AddCommand(aliasConnect)

	cmd.AddCommand(newMigrateCommand())

	cmd.AddCommand(
		newListCommand(),
		newBrowseCommand(),
		newInstallCommand(),
		newAddCommand(),
		newPushCommand(),
		newRemoveCommand(),
		newSyncCommand(),
		newCheckCommand(),
		newUpdateCommand(),
		newAdoptCommand(),
		newStatusCommand(),
		newInitCommand(),
		newShowCommand(),
		newSchemaCommand(cmd),
		newResolveCommand(),
		newRestoreCommand(),
		newSkillCommand(),
		newProjectCommand(),
		newKitCommand(),
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

	wrapRunECommands(cmd)

	return cmd
}

func classifyExecuteError(err error) error {
	var ce *clierrors.Error
	if stderrors.As(err, &ce) {
		return err
	}
	if isCobraUsageErr(err) {
		return clierrors.Wrap(err, "USAGE", clierrors.ExitUsage,
			clierrors.WithMessage(err.Error()),
			clierrors.WithRemediation("scribe --help"),
		)
	}
	return clierrors.Wrap(err, "GENERAL", clierrors.ExitGeneral, clierrors.WithMessage(err.Error()))
}

func isCobraUsageErr(err error) bool {
	msg := err.Error()
	for _, prefix := range []string{
		"unknown command ",
		"unknown flag ",
		"unknown flag:",
		"unknown shorthand flag",
		"flag provided but not defined",
		"required flag(s) ",
		"accepts ",
		"requires ",
	} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}

func markJSONSupported(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[jsonSupportedAnnotation] = "true"
	return cmd
}

func markReadOnly(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[commandModeAnnotation] = commandModeReadOnly
	return cmd
}

func markReadOnlyWithoutApply(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[commandModeAnnotation] = commandModeApplyWrites
	return cmd
}

func commandReadOnly(cmd *cobra.Command) bool {
	switch cmd.Annotations[commandModeAnnotation] {
	case commandModeReadOnly:
		return true
	case commandModeApplyWrites:
		apply, err := cmd.Flags().GetBool("apply")
		return err == nil && !apply
	default:
		return false
	}
}

func jsonFlagPassed(cmd *cobra.Command) bool {
	if root := cmd.Root(); root != nil {
		if flag := root.PersistentFlags().Lookup("json"); flag != nil && flag.Changed {
			return true
		}
	}
	flag := cmd.Flag("json")
	return flag != nil && flag.Changed
}

func commandSupportsJSON(cmd *cobra.Command) bool {
	return cmd.Annotations[jsonSupportedAnnotation] == "true"
}

func wrapRunECommands(cmd *cobra.Command) {
	if cmd.RunE != nil {
		cmd.RunE = wrapRunE(cmd.RunE)
	}
	for _, child := range cmd.Commands() {
		wrapRunECommands(child)
	}
}

func wrapRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		runStart := time.Now()
		ctx := context.WithValue(cmd.Context(), envelope.RunEStartKey, runStart)
		cmd.SetContext(ctx)

		err := fn(cmd, args)

		duration := time.Since(runStart).Milliseconds()
		bootstrap := int64(0)
		if start, ok := cmd.Context().Value(envelope.BootstrapStartKey).(time.Time); ok {
			bootstrap = runStart.Sub(start).Milliseconds()
			if bootstrap < 0 {
				bootstrap = 0
			}
		}
		ctx = context.WithValue(cmd.Context(), envelope.DurationMSKey, duration)
		ctx = context.WithValue(ctx, envelope.BootstrapMSKey, bootstrap)
		cmd.SetContext(ctx)

		return err
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
