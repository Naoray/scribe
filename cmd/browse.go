package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

var discoverEntriesFn = discoverEntries

func newBrowseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Browse skills available from connected registries",
		Args:  cobra.NoArgs,
		RunE:  runBrowse,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().String("query", "", "Filter remote skills by query")
	cmd.Flags().String("install", "", "Install a skill by exact name or owner/repo:skill")
	cmd.Flags().String("registry", "", "Limit browse/install to one connected registry")
	addNoInteractionFlag(cmd, "Disable interactive prompts", false)
	return cmd
}

func runBrowse(cmd *cobra.Command, _ []string) error {
	query, _ := cmd.Flags().GetString("query")
	installRef, _ := cmd.Flags().GetString("install")
	registryFilter, _ := cmd.Flags().GetString("registry")
	yes := noInteractionFlagPassed(cmd)
	useJSON, _ := cmd.Flags().GetBool("json")
	useJSON = useJSON || !isatty.IsTerminal(os.Stdout.Fd())

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.TeamRepos()) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}
	targets, err := tools.ResolveActive(cfg)
	if err != nil {
		return fmt.Errorf("resolve active tools: %w", err)
	}

	repos := cfg.TeamRepos()
	if registryFilter != "" {
		repo, err := resolveRegistry(registryFilter, repos)
		if err != nil {
			return err
		}
		repos = []string{repo}
	}

	return runBrowseWithDeps(cmd.Context(), repos, query, installRef, cfg, st, client, targets, useJSON, yes)
}

func runBrowseWithDeps(
	ctx context.Context,
	repos []string,
	query string,
	installRef string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	skipConfirm bool,
) error {
	if installRef == "" && !useJSON {
		bag := &workflow.Bag{
			JSONFlag:         false,
			RemoteFlag:       true,
			BrowseFlag:       true,
			InitialQuery:     query,
			LazyGitHub:       false,
			Factory:          newCommandFactory(),
			Config:           cfg,
			State:            st,
			Client:           client,
			Tools:            targets,
			FilterRegistries: filterRegistries,
		}
		bag.Provider = provider.NewGitHubProvider(provider.WrapGitHubClient(client))
		if len(repos) == 1 {
			bag.RepoFlag = repos[0]
		}
		m := newListModel(ctx, bag)
		prog := tea.NewProgram(m, tea.WithContext(ctx))
		_, err := prog.Run()
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}
		return nil
	}

	entries, errs := discoverEntriesFn(ctx, repos, client, targets, st)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}

	if query != "" {
		entries = filterEntries(entries, query)
	}

	if installRef != "" {
		return browseInstall(ctx, installRef, entries, cfg, st, client, targets, useJSON, skipConfirm)
	}

	entries = filterBrowseEntries(entries)

	if useJSON {
		return emitBrowseJSON(entries)
	}
	return nil
}

func browseInstall(
	ctx context.Context,
	installRef string,
	entries []browseEntry,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	skipConfirm bool,
) error {
	if strings.Contains(installRef, ":") {
		registryRepo, skillName, err := parseSkillRef(installRef)
		if err != nil {
			return err
		}
		return runAddDirectInstall(ctx, registryRepo, skillName, cfg, st, newInstallSyncer(client, targets), client.IsAuthenticated(), useJSON, skipConfirm)
	}

	var matches []browseEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Status.Name, installRef) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("skill %q not found in connected registries", installRef)
	}
	if len(matches) > 1 {
		var options []string
		for _, match := range matches {
			options = append(options, fmt.Sprintf("%s:%s", match.Registry, match.Status.Name))
		}
		return fmt.Errorf("skill %q is ambiguous — use one of: %s", installRef, strings.Join(options, ", "))
	}

	match := matches[0]
	return runAddDirectInstall(ctx, match.Registry, match.Status.Name, cfg, st, newInstallSyncer(client, targets), client.IsAuthenticated(), useJSON, skipConfirm)
}

func filterBrowseEntries(entries []browseEntry) []browseEntry {
	out := make([]browseEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Status.Status == sync.StatusCurrent {
			continue
		}
		out = append(out, entry)
	}
	return out
}
