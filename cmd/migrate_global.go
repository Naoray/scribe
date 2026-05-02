package cmd

import (
	"fmt"
	"os"
	"sort"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/projectmigrate"
	"github.com/Naoray/scribe/internal/tools"
)

type projectSelector interface {
	SelectProjects([]projectmigrate.ProjectCandidate) ([]string, error)
}

type huhProjectSelector struct{}

var globalToProjectsIsTerminal = func() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

func newGlobalToProjectsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "global-to-projects",
		Short: "Move legacy global skill links into project .scribe.yaml files",
		Long: `Detect Scribe-managed symlinks in legacy global tool skill directories,
let the user choose projects that should keep that skill set, write .scribe.yaml
files for those projects, and remove the global symlinks.`,
		Args: cobra.NoArgs,
		RunE: runGlobalToProjects,
	}
	cmd.Flags().Bool("dry-run", false, "Preview migration without writing .scribe.yaml or removing global symlinks")
	cmd.Flags().Bool("force", false, "Allow migration even if a project exceeds an agent skill budget")
	cmd.Flags().Bool("undo", false, "Restore the latest global-to-projects migration snapshot")
	cmd.Flags().Bool("yes", false, "Skip confirmation prompts")
	cmd.Flags().StringArray("project", nil, "Project directory to keep the current global skill set (repeatable; skips prompt)")
	return markJSONSupported(cmd)
}

func runGlobalToProjects(cmd *cobra.Command, args []string) error {
	return runGlobalToProjectsWithSelector(cmd, args, huhProjectSelector{})
}

func runGlobalToProjectsWithSelector(cmd *cobra.Command, _ []string, selector projectSelector) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	forceBudget, _ := cmd.Flags().GetBool("force")
	undo, _ := cmd.Flags().GetBool("undo")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonFlag := jsonFlagPassed(cmd)
	projectFlags, _ := cmd.Flags().GetStringArray("project")

	if undo {
		if dryRun || len(projectFlags) > 0 {
			return clierrors.Wrap(fmt.Errorf("--undo cannot be combined with --project or --dry-run"), "USAGE_FLAG_CONFLICT", clierrors.ExitUsage)
		}
		path, err := projectmigrate.LatestSnapshotPath()
		if err != nil {
			return err
		}
		snapshot, err := projectmigrate.LoadSnapshot(path)
		if err != nil {
			return err
		}
		result, err := projectmigrate.Undo(snapshot, path)
		if err != nil {
			return err
		}
		if jsonFlag {
			return renderMutatorEnvelope(cmd, result, envelope.StatusOK)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "restored %d global symlink(s)\n", result.RestoredLinks)
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("store dir: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	factory := commandFactory()
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	discovery, err := projectmigrate.Discover(projectmigrate.DiscoveryOptions{
		HomeDir:     home,
		StoreDir:    storeDir,
		SearchRoots: []string{wd},
		State:       st,
	})
	if err != nil {
		return fmt.Errorf("discover migration candidates: %w", err)
	}

	selected := projectFlags
	if len(selected) == 0 && !jsonFlag && globalToProjectsIsTerminal() {
		selected, err = selector.SelectProjects(discovery.Projects)
		if err != nil {
			return err
		}
	}
	if len(selected) == 0 && len(discovery.GlobalSymlinks) > 0 {
		return clierrors.Wrap(
			fmt.Errorf("must pass --project <path>; refusing to remove global symlinks"),
			"USAGE",
			clierrors.ExitUsage,
			clierrors.WithRemediation("scribe migrate global-to-projects --project <path> --dry-run"),
		)
	}
	if !dryRun && !jsonFlag && !yes && len(projectFlags) > 0 && globalToProjectsIsTerminal() {
		var confirm bool
		err := huh.NewConfirm().
			Title("Remove legacy global symlinks and write selected project .scribe.yaml files?").
			Value(&confirm).
			Run()
		if err != nil {
			return err
		}
		if !confirm {
			return clierrors.Wrap(fmt.Errorf("migration canceled"), "CANCELED", clierrors.ExitCanceled)
		}
	}

	plan, err := projectmigrate.BuildPlan(discovery, selected, dryRun, forceBudget)
	if err != nil {
		return err
	}
	result, err := projectmigrate.Apply(plan, discovery.Projects)
	if err != nil {
		return err
	}

	if jsonFlag {
		status := envelope.StatusOK
		if dryRun || result.WroteProjectFiles == 0 && result.RemovedGlobalLinks == 0 {
			status = envelope.StatusNoChange
		}
		return renderMutatorEnvelope(cmd, result, status)
	}

	printGlobalToProjectsResult(cmd, result)
	return nil
}

func (huhProjectSelector) SelectProjects(projects []projectmigrate.ProjectCandidate) ([]string, error) {
	if len(projects) == 0 {
		return nil, nil
	}
	opts := make([]huh.Option[string], len(projects))
	for i, project := range projects {
		opts[i] = huh.NewOption(project.Path, project.Path).Selected(true)
	}

	var selected []string
	err := huh.NewMultiSelect[string]().
		Title("Pick projects that should keep the current global skill set").
		Options(opts...).
		Value(&selected).
		Run()
	return selected, err
}

func projectPaths(projects []projectmigrate.ProjectCandidate) []string {
	paths := make([]string, 0, len(projects))
	for _, project := range projects {
		paths = append(paths, project.Path)
	}
	return paths
}

func printGlobalToProjectsResult(cmd *cobra.Command, result projectmigrate.MigrationResult) {
	out := cmd.OutOrStdout()
	prefix := ""
	if result.DryRun {
		prefix = "Dry run: "
	}
	fmt.Fprintf(out, "%sfound %d globally projected skill link(s) for %d skill(s)\n", prefix, result.FoundGlobalLinks, result.FoundSkills)
	if len(result.ProjectFiles) > 0 {
		projectWrites := result.WroteProjectFiles
		if result.DryRun {
			projectWrites = result.PlannedProjectFileWrites
		}
		fmt.Fprintf(out, "%swrote .scribe.yaml in %d project(s)\n", prefix, projectWrites)
		for _, change := range result.ProjectFiles {
			action := "unchanged"
			if change.Changed {
				action = "write"
			}
			fmt.Fprintf(out, "  %s %s (%d skill%s, %d added)\n", action, change.File, len(change.Skills), plural(len(change.Skills)), len(change.AddedSkills))
			printBudgetLines(out, change.BudgetPerAgent)
		}
	}
	linkRemovals := result.RemovedGlobalLinks
	if result.DryRun {
		linkRemovals = result.PlannedGlobalLinkRemovals
	}
	fmt.Fprintf(out, "%sremoved %d global symlink(s)\n", prefix, linkRemovals)
	links := result.RemovedLinks
	if result.DryRun {
		links = result.RemovedLinks
	}
	sample := links
	if len(sample) > 5 {
		sample = sample[:5]
	}
	for _, link := range sample {
		fmt.Fprintf(out, "    - %s → %s\n", link.Path, link.CanonicalPath)
	}
	if len(links) > 5 {
		fmt.Fprintf(out, "    ... and %d more\n", len(links)-5)
	}
	if result.SkippedGlobalLinks > 0 {
		fmt.Fprintf(out, "skipped %d global path(s) that were already gone or no longer symlinks\n", result.SkippedGlobalLinks)
	}
}

func printBudgetLines(out interface{ Write([]byte) (int, error) }, results map[string]budget.Result) {
	agents := make([]string, 0, len(results))
	for agent := range results {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	for _, agent := range agents {
		result := results[agent]
		if result.Limit <= 0 {
			continue
		}
		status := "PASS"
		if result.Status == budget.StatusRefuse {
			status = "REFUSE"
		}
		fmt.Fprintf(out, "  budget: %s %s %s / %s", agent, status, formatBudgetAmount(result.Used), formatBudgetAmount(result.Limit))
		if result.Status == budget.StatusRefuse {
			fmt.Fprintf(out, " (+%s)", formatBudgetAmount(result.Used-result.Limit))
		}
		fmt.Fprintln(out)
	}
}

func formatBudgetAmount(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	value := float64(bytes) / 1024
	if value == float64(int(value)) {
		return fmt.Sprintf("%dKB", int(value))
	}
	return fmt.Sprintf("%.1fKB", value)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
