package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

var discoverEntriesFn = discoverEntries
var discoverSourceEntriesFn = discoverSourceEntries
var discoverKitEntriesFn = discoverKitEntries

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
	addSourceFlags(cmd, true)
	cmd.Flags().Bool("kits", false, "Browse registry kits instead of skills")
	cmd.Flags().Bool("resync", false, "Overwrite local edits with the upstream version for modified skills")
	addNoInteractionFlag(cmd, "Disable interactive prompts", false)
	return markJSONSupported(cmd)
}

func runBrowse(cmd *cobra.Command, _ []string) error {
	query, _ := cmd.Flags().GetString("query")
	installRef, _ := cmd.Flags().GetString("install")
	sourceFlags, err := readSourceFlags(cmd)
	if err != nil {
		return err
	}
	resync, _ := cmd.Flags().GetBool("resync")
	kits, _ := cmd.Flags().GetBool("kits")
	yes := noInteractionFlagPassed(cmd)
	useJSON, _ := cmd.Flags().GetBool("json")
	useJSON = useJSON || !isatty.IsTerminal(os.Stdout.Fd())

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
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

	sourceFilter := sourceFlags.source
	sources, err := browseSources(sourceFilter, cfg)
	if err != nil {
		return err
	}
	repos := legacyReposFromSources(sources)

	if kits {
		return runBrowseKitsWithDeps(cmd.Context(), repos, query, installRef, cfg, st, client, useJSON, yes)
	}

	return runBrowseWithDeps(cmd.Context(), sources, query, installRef, cfg, st, client, targets, useJSON, yes, resync)
}

type kitBrowseEntry struct {
	Kit       registry.ManifestKit
	Installed bool
}

func runBrowseKitsWithDeps(
	ctx context.Context,
	repos []string,
	query string,
	installRef string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	useJSON bool,
	skipConfirm bool,
) error {
	if installRef != "" {
		opts := &kitInstallOptions{json: useJSON, noInteraction: skipConfirm}
		if !strings.Contains(installRef, ":") && len(repos) == 1 {
			installRef = repos[0] + ":" + installRef
		}
		return runKitInstall(newKitInstallCommand(), installRef, opts)
	}

	if !useJSON {
		bag := &workflow.Bag{
			JSONFlag:         false,
			RemoteFlag:       true,
			BrowseFlag:       true,
			KitBrowseFlag:    true,
			InitialQuery:     query,
			LazyGitHub:       false,
			Factory:          newCommandFactory(),
			Config:           cfg,
			State:            st,
			Client:           client,
			FilterRegistries: func(_ string, _ []string) ([]string, error) { return repos, nil },
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

	entries, errs := discoverKitEntriesFn(ctx, repos, client, st)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}
	if query != "" {
		entries = filterKitEntries(entries, query)
	}
	return emitBrowseKitsJSON(entries)
}

func discoverKitEntries(ctx context.Context, repos []string, client registry.FileFetcher, st *state.State) ([]kitBrowseEntry, []error) {
	var entries []kitBrowseEntry
	var errs []error
	for _, repo := range repos {
		kits, err := registry.ListKits(ctx, client, repo)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", repo, err))
			continue
		}
		for _, remote := range kits {
			installed := false
			if st != nil {
				if local, ok := st.Kits[remote.Name]; ok {
					source := local.SourceRegistry
					if source == "" {
						source = local.Source
					}
					installed = source == repo
				}
			}
			entries = append(entries, kitBrowseEntry{Kit: remote, Installed: installed})
		}
	}
	return entries, errs
}

func filterKitEntries(entries []kitBrowseEntry, query string) []kitBrowseEntry {
	q := strings.ToLower(query)
	var out []kitBrowseEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Kit.Name), q) || strings.Contains(strings.ToLower(e.Kit.Description), q) {
			out = append(out, e)
		}
	}
	return out
}

func emitBrowseKitsJSON(entries []kitBrowseEntry) error {
	type row struct {
		Name             string `json:"name"`
		Registry         string `json:"registry"`
		Path             string `json:"path"`
		Description      string `json:"description,omitempty"`
		Author           string `json:"author,omitempty"`
		InstalledLocally bool   `json:"installed_locally"`
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{
			Name:             e.Kit.Name,
			Registry:         e.Kit.Registry,
			Path:             e.Kit.Path,
			Description:      e.Kit.Description,
			Author:           e.Kit.Author,
			InstalledLocally: e.Installed,
		})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"results": rows})
}

func browseRepos(registryFilter string, connected []string) ([]string, error) {
	if registryFilter == "" {
		if len(connected) == 0 {
			return nil, fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
		}
		return connected, nil
	}
	if repo, err := resolveRegistry(registryFilter, connected); err == nil {
		return []string{repo}, nil
	}
	repo, err := manifest.NormalizeGitHubRepo(registryFilter)
	if err != nil {
		return nil, err
	}
	return []string{repo}, nil
}

func browseSources(registryFilter string, cfg *config.Config) ([]config.RegistrySource, error) {
	connected := cfg.EnabledSources()
	if registryFilter == "" {
		if len(connected) == 0 {
			return nil, fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
		}
		return connected, nil
	}
	if rc := cfg.FindRegistryByKeyOrRepo(registryFilter); rc != nil {
		spec, ident, err := source.Canonicalize(rc.SourceSpec())
		if err != nil {
			return nil, err
		}
		rs := config.RegistrySource{Config: *rc, Source: spec, Identity: ident}
		rs.ID = registryDisplay(rs)
		return []config.RegistrySource{rs}, nil
	}
	spec, ident, err := sourceSpecForRegistry(registryFilter)
	if err != nil {
		return nil, err
	}
	return []config.RegistrySource{{ID: registryFilter, Source: spec, Identity: ident}}, nil
}

func legacyReposFromSources(sources []config.RegistrySource) []string {
	repos := make([]string, 0, len(sources))
	for _, src := range sources {
		if src.Source.Type == source.SourceGitHub && src.Source.Repo != "" {
			repos = append(repos, src.Source.Repo)
		}
	}
	return repos
}

func runBrowseWithDeps(
	ctx context.Context,
	sources []config.RegistrySource,
	query string,
	installRef string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	skipConfirm bool,
	resync bool,
) error {
	if installRef == "" && !useJSON {
		if cfg != nil {
			for _, repo := range legacyReposFromSources(sources) {
				if cfg.FindRegistry(repo) == nil {
					cfg.AddRegistry(config.RegistryConfig{Repo: repo, Enabled: true, Type: config.RegistryTypeCommunity})
				}
			}
		}
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
		repos := legacyReposFromSources(sources)
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

	entries, errs := discoverSourceEntriesFn(ctx, sources, client, targets, st)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}

	if query != "" {
		entries = filterEntries(entries, query)
	}

	if installRef != "" {
		return browseInstall(ctx, installRef, entries, cfg, st, client, targets, useJSON, skipConfirm, resync)
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
	resync bool,
) error {
	if strings.Contains(installRef, ":") {
		spec, ident, display, skillName, err := parseInstallRefForCommand(installRef)
		if err != nil {
			return err
		}
		return runAddDirectInstallSourceForCommand(nil, ctx, display, ident.Key, spec, skillName, cfg, st, newInstallSyncer(client, targets), client.IsAuthenticated(), useJSON, skipConfirm, resync)
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
			sourceID := match.Registry
			if match.SourceKey != "" {
				sourceID = match.SourceKey
			}
			options = append(options, fmt.Sprintf("%s:%s", sourceID, match.Status.Name))
		}
		return fmt.Errorf("skill %q is ambiguous — use one of: %s", installRef, strings.Join(options, ", "))
	}

	match := matches[0]
	if match.SourceKey != "" {
		return runAddDirectInstallSourceForCommand(nil, ctx, match.Registry, match.SourceKey, match.Source, match.Status.Name, cfg, st, newInstallSyncer(client, targets), client.IsAuthenticated(), useJSON, skipConfirm, resync)
	}
	return runAddDirectInstall(ctx, match.Registry, match.Status.Name, cfg, st, newInstallSyncer(client, targets), client.IsAuthenticated(), useJSON, skipConfirm, resync)
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
