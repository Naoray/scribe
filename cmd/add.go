package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

// installResult is the per-skill JSON output for `scribe add`.
type installResult struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type installRunResults struct {
	Installed  []installResult
	Resolution *nameConflictResolutionPayload
}

// browseEntry pairs a SkillStatus with the registry it came from.
type browseEntry struct {
	Status    sync.SkillStatus
	Registry  string
	SourceKey string
	Source    source.SourceSpec
}

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [query]",
		Short: "Find and install skills from connected registries",
		Long: `Browse, search, and install skills from connected registries.

With no argument, opens an interactive browser of every skill across
every connected registry. With a query, filters by name and description.
With "owner/repo:skillname", installs that specific skill directly,
auto-connecting the registry first if needed.

Examples:
  scribe add                          # browse everything
  scribe add react                    # search "react"
  scribe add antfu/skills:nuxt        # direct install
  scribe add antfu/skills:nuxt --no-interaction  # non-interactive
  scribe add antfu/skills:nuxt --resync  # overwrite local edits from upstream
  scribe add react --json             # machine-readable search`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAdd,
	}
	addNoInteractionFlag(cmd, "Disable interactive prompts", false)
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	addSourceFlags(cmd, true)
	cmd.Flags().Bool("force", false, "Deprecated: budget guardrails are warn-only (no-op)")
	cmd.Flags().Bool("resync", false, "Overwrite local edits with the upstream version for modified skills")
	cmd.Flags().String("alias", "", "Install incoming skill under this name when a local directory conflicts")
	return markJSONSupported(cmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	skipConfirm := noInteractionFlagPassed(cmd)
	jsonFlag := jsonFlagPassed(cmd)
	sourceFlags, err := readSourceFlags(cmd)
	if err != nil {
		return err
	}
	forceBudget, _ := cmd.Flags().GetBool("force")
	warnDeprecatedForceBudget(cmd)
	resync, _ := cmd.Flags().GetBool("resync")
	aliasName, _ := cmd.Flags().GetString("alias")

	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
	conflictMode := workflow.ConflictModeForProcess(jsonFlag)
	useJSON := workflow.UseJSONOutputForProcess(jsonFlag)
	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if err := enforceCurrentBudget(factory); err != nil {
		return err
	}

	ctx := cmd.Context()
	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}
	targets, err := tools.ResolveActive(cfg)
	if err != nil {
		return fmt.Errorf("resolve tools: %w", err)
	}

	syncer := newInstallSyncerWithOptions(client, targets, forceBudget, aliasName)
	if resync {
		syncer.ModifiedStrategy = sync.ModifiedStrategyPreferTheirs
	}
	configureInstallNameConflictResolver(syncer, conflictMode, aliasName)

	if sourceFlags.hasTyped() && !sourceFlagsMatchConnectedSource(sourceFlags, cfg) {
		if len(args) != 1 {
			return fmt.Errorf("skill name required when using source flags")
		}
		spec, ident, display, err := sourceSpecFromFlags(sourceFlags)
		if err != nil {
			return err
		}
		authenticated := client.IsAuthenticated() || !requiresSourceGitHubAuth(spec)
		if err := runAddDirectInstallSourceForCommand(cmd, ctx, display, ident.Key, spec, args[0], cfg, st, syncer, authenticated, useJSON, skipConfirm, resync); err != nil {
			return handleNameConflictError(cmd, err)
		}
		return nil
	}

	// Direct install: owner/repo:skillname or [source]:skillname.
	if len(args) == 1 && looksLikeInstallRef(args[0]) {
		spec, ident, display, skillName, err := parseInstallRefForCommand(args[0])
		if err != nil {
			return err
		}
		authenticated := client.IsAuthenticated() || !requiresSourceGitHubAuth(spec)
		if err := runAddDirectInstallSourceForCommand(cmd, ctx, display, ident.Key, spec, skillName, cfg, st, syncer, authenticated, useJSON, skipConfirm, resync); err != nil {
			return handleNameConflictError(cmd, err)
		}
		return nil
	}

	// Need at least one connected registry to search/browse.
	if len(cfg.EnabledSources()) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}
	if requiresGitHubAuth(cfg.EnabledSources()) && !client.IsAuthenticated() {
		return clierrors.Wrap(
			fmt.Errorf("authentication required"),
			"GH_AUTH_FAILED",
			clierrors.ExitPerm,
			clierrors.WithRemediation("run `gh auth login` or set GITHUB_TOKEN"),
		)
	}

	// Determine which registries to browse.
	sources := cfg.EnabledSources()
	if sourceFlags.source != "" {
		sources, err = browseSources(sourceFlags.source, cfg)
		if err != nil {
			return err
		}
	}

	// Build query from arg.
	query := ""
	if len(args) == 1 {
		query = args[0]
	}

	// Non-TTY without JSON requires either a direct ref or --json.
	if !isTTY && !useJSON {
		return fmt.Errorf("interactive browse requires a terminal — pass owner/repo:skillname or --json")
	}

	// Discover all skills across the selected registries.
	entries, errs := discoverSourceEntries(ctx, sources, client, targets, st)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}

	// Filter by query.
	if query != "" {
		entries = filterEntries(entries, query)
	}

	// JSON or non-TTY: just emit results.
	if useJSON {
		return emitBrowseJSONForCommand(cmd, entries)
	}

	if len(entries) == 0 {
		if query != "" {
			fmt.Printf("No skills matching %q in connected registries.\n", query)
		} else {
			fmt.Println("No skills found in connected registries.")
		}
		return nil
	}

	// Interactive browser.
	sortEntries(entries)
	selected, err := runInstallBrowser(entries, query)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}

	if err := installSelected(ctx, selected, cfg, st, client, targets, skipConfirm, forceBudget, aliasName, conflictMode); err != nil {
		return handleNameConflictError(cmd, err)
	}
	return nil
}

func sourceFlagsMatchConnectedSource(v sourceFlagValues, cfg *config.Config) bool {
	if cfg == nil || v.source == "" || v.repo != "" || v.url != "" || v.ref != "" || v.path != "" || v.id != "" {
		return false
	}
	return cfg.FindRegistryByKeyOrRepo(v.source) != nil
}

// parseSkillRef parses "owner/repo:skillname" into its parts.
func parseSkillRef(ref string) (registryRepo, skillName string, err error) {
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		err := fmt.Errorf("invalid skill reference %q: expected owner/repo:skillname", ref)
		return "", "", clierrors.Wrap(err, "USAGE_INVALID_SKILL_REF", clierrors.ExitUsage)
	}
	registryRepo = ref[:idx]
	skillName = ref[idx+1:]
	if _, _, perr := manifest.ParseOwnerRepo(registryRepo); perr != nil {
		err := fmt.Errorf("invalid skill reference %q: %w", ref, perr)
		return "", "", clierrors.Wrap(err, "USAGE_INVALID_SKILL_REF", clierrors.ExitUsage)
	}
	if skillName == "" {
		err := fmt.Errorf("invalid skill reference %q: skill name is empty", ref)
		return "", "", clierrors.Wrap(err, "USAGE_INVALID_SKILL_REF", clierrors.ExitUsage)
	}
	return registryRepo, skillName, nil
}

func looksLikeInstallRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "[") || strings.Contains(ref, "://") {
		return true
	}
	parsed, err := source.ParseInstallRef(ref)
	return err == nil && parsed.Source.Type == source.SourceGitHub && parsed.Source.Path == "" && parsed.Source.Ref == ""
}

// runAddDirectInstall installs a single skill from owner/repo:skillname.
// Auto-connects the registry if it isn't already in config, but only after
// validating that the skill actually exists in the registry.
func runAddDirectInstall(
	ctx context.Context,
	registryRepo, skillName string,
	cfg *config.Config,
	st *state.State,
	syncer *sync.Syncer,
	authenticated bool,
	useJSON bool,
	skipConfirm bool,
	resyncOpt ...bool,
) error {
	resync := len(resyncOpt) > 0 && resyncOpt[0]
	if resync {
		syncer.ModifiedStrategy = sync.ModifiedStrategyPreferTheirs
	}
	return runAddDirectInstallForCommand(nil, ctx, registryRepo, skillName, cfg, st, syncer, authenticated, useJSON, skipConfirm, resync)
}

func runAddDirectInstallForCommand(
	cmd *cobra.Command,
	ctx context.Context,
	registryRepo, skillName string,
	cfg *config.Config,
	st *state.State,
	syncer *sync.Syncer,
	authenticated bool,
	useJSON bool,
	skipConfirm bool,
	resync bool,
) error {
	spec, ident, err := sourceSpecForRegistry(registryRepo)
	if err != nil {
		return err
	}
	return runAddDirectInstallSourceForCommand(cmd, ctx, registryRepo, ident.Key, spec, skillName, cfg, st, syncer, authenticated, useJSON, skipConfirm, resync)
}

func runAddDirectInstallSourceForCommand(
	cmd *cobra.Command,
	ctx context.Context,
	registryDisplay, sourceKey string,
	spec source.SourceSpec,
	skillName string,
	cfg *config.Config,
	st *state.State,
	syncer *sync.Syncer,
	authenticated bool,
	useJSON bool,
	skipConfirm bool,
	resync bool,
) error {
	if !authenticated {
		return clierrors.Wrap(
			fmt.Errorf("authentication required"),
			"GH_AUTH_FAILED",
			clierrors.ExitPerm,
			clierrors.WithRemediation("run `gh auth login` or set GITHUB_TOKEN"),
		)
	}

	var statuses []sync.SkillStatus
	var err error
	if isLegacyGitHubSource(spec) {
		statuses, _, err = syncer.Diff(ctx, spec.Repo, st)
	} else {
		statuses, _, err = syncer.DiffSource(ctx, sourceKey, spec, st)
	}
	if err != nil {
		return fmt.Errorf("diff %s: %w", registryDisplay, err)
	}

	var target *sync.SkillStatus
	for i := range statuses {
		if statuses[i].Name == skillName {
			target = &statuses[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("skill %q not found in %s", skillName, registryDisplay)
	}

	// Skill exists — safe to auto-connect the registry now.
	if cfg.FindRegistryByKeyOrRepo(sourceKey) == nil && cfg.FindRegistryByKeyOrRepo(registryDisplay) == nil {
		cfg.AddRegistry(registryConfigForSource(spec))
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		if !useJSON {
			fmt.Printf("connected %s\n", registryDisplay)
		}
	}

	if target.Status == sync.StatusCurrent {
		if useJSON {
			return emitInstallJSON([]installResult{{
				Name: target.Name, Registry: registryDisplay, Status: "already-installed",
			}}, nil, cmd)
		}
		fmt.Printf("%s is already installed (current).\n", skillName)
		return nil
	}
	if target.Status == sync.StatusModified && !resync {
		if useJSON {
			return emitInstallJSON([]installResult{{
				Name: target.Name, Registry: registryDisplay, Status: "modified",
			}}, nil, cmd)
		}
		fmt.Printf("%s has local edits. Run with --resync to overwrite them from upstream.\n", skillName)
		return nil
	}

	// Confirmation.
	if !skipConfirm && !useJSON {
		var confirm bool
		title := fmt.Sprintf("Install %s from %s?", skillName, registryDisplay)
		if target.Status == sync.StatusOutdated {
			title = fmt.Sprintf("Update %s from %s?", skillName, registryDisplay)
		}
		if target.Status == sync.StatusModified && resync {
			title = fmt.Sprintf("Resync %s from %s and overwrite local edits?", skillName, registryDisplay)
		}
		if err := huh.NewConfirm().Title(title).Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	results := wireInstallSyncer(syncer, registryDisplay, useJSON)
	if isLegacyGitHubSource(spec) {
		err = syncer.RunWithDiff(ctx, spec.Repo, []sync.SkillStatus{*target}, st)
	} else {
		err = syncer.RunWithDiffSource(ctx, sourceKey, spec, []sync.SkillStatus{*target}, st)
	}
	if err != nil {
		return fmt.Errorf("install %s: %w", skillName, err)
	}

	if useJSON {
		return emitInstallJSON(results.Installed, results.Resolution, cmd)
	}
	return nil
}

func sourceSpecForRegistry(input string) (source.SourceSpec, source.SourceIdentity, error) {
	spec, err := source.ParseSourceArg(input)
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, err
	}
	spec, ident, err := source.Canonicalize(spec)
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, err
	}
	if spec.Type == source.SourceGitHub && spec.Path == "" && spec.Ref == "" {
		ident.Key = spec.Repo
	}
	return spec, ident, nil
}

func registryConfigForSource(spec source.SourceSpec) config.RegistryConfig {
	if isLegacyGitHubSource(spec) {
		return config.RegistryConfig{Repo: spec.Repo, Enabled: true, Type: config.RegistryTypeCommunity}
	}
	return config.RegistryConfig{ID: spec.ID, Enabled: true, Type: config.RegistryTypeCommunity, Source: &spec}
}

func isLegacyGitHubSource(spec source.SourceSpec) bool {
	return spec.Type == source.SourceGitHub && spec.Path == "" && spec.Ref == ""
}

func requiresSourceGitHubAuth(spec source.SourceSpec) bool {
	return spec.Type == source.SourceGitHub
}

func requiresGitHubAuth(sources []config.RegistrySource) bool {
	for _, src := range sources {
		if requiresSourceGitHubAuth(src.Source) {
			return true
		}
	}
	return false
}

// installSelected installs the user-selected entries from the browser. Each
// entry may belong to a different registry; auto-connects as needed.
func installSelected(
	ctx context.Context,
	selected []browseEntry,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	skipConfirm bool,
	forceBudget bool,
	aliasName string,
	conflictMode workflow.ConflictMode,
) error {
	// Group by registry.
	byRegistry := map[string][]sync.SkillStatus{}
	sourceByRegistry := map[string]source.SourceSpec{}
	sourceKeyByRegistry := map[string]string{}
	order := []string{}
	for _, e := range selected {
		if _, seen := byRegistry[e.Registry]; !seen {
			order = append(order, e.Registry)
		}
		byRegistry[e.Registry] = append(byRegistry[e.Registry], e.Status)
		if e.SourceKey != "" {
			sourceByRegistry[e.Registry] = e.Source
			sourceKeyByRegistry[e.Registry] = e.SourceKey
		}
	}

	// Confirmation summary.
	if !skipConfirm {
		fmt.Printf("\nInstalling %d skill(s):\n", len(selected))
		for _, e := range selected {
			marker := "install"
			switch e.Status.Status {
			case sync.StatusCurrent:
				marker = "already current — skip"
			case sync.StatusOutdated:
				marker = "update"
			case sync.StatusModified:
				marker = "modified — skip unless resyncing from registry"
			}
			fmt.Printf("  • %s  (%s)  [%s]\n", e.Status.Name, e.Registry, marker)
		}
		var confirm bool
		if err := huh.NewConfirm().Title("Proceed?").Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	syncer := newInstallSyncerWithOptions(client, targets, forceBudget, aliasName)
	configureInstallNameConflictResolver(syncer, conflictMode, aliasName)

	var installErr error
	for _, registryRepo := range order {
		// Auto-connect if needed.
		if cfg.FindRegistryByKeyOrRepo(registryRepo) == nil {
			if spec, ok := sourceByRegistry[registryRepo]; ok {
				cfg.AddRegistry(registryConfigForSource(spec))
			} else {
				cfg.AddRegistry(config.RegistryConfig{Repo: registryRepo, Enabled: true, Type: config.RegistryTypeCommunity})
			}
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("connected %s\n", registryRepo)
		}

		// Filter out already-current skills.
		var toInstall []sync.SkillStatus
		for _, s := range byRegistry[registryRepo] {
			if s.Status == sync.StatusCurrent {
				fmt.Printf("  - %s already installed, skipping\n", s.Name)
				continue
			}
			toInstall = append(toInstall, s)
		}
		if len(toInstall) == 0 {
			continue
		}

		fmt.Printf("\ninstalling from %s...\n\n", registryRepo)
		_ = wireInstallSyncer(syncer, registryRepo, false)
		var err error
		if spec, ok := sourceByRegistry[registryRepo]; ok {
			err = syncer.RunWithDiffSource(ctx, sourceKeyByRegistry[registryRepo], spec, toInstall, st)
		} else {
			err = syncer.RunWithDiff(ctx, registryRepo, toInstall, st)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			installErr = err
		}
	}
	return installErr
}

// newInstallSyncer constructs a Syncer ready to install skills.
func newInstallSyncer(client *gh.Client, targets []tools.Tool, forceBudgetOpt ...bool) *sync.Syncer {
	forceBudget := false
	if len(forceBudgetOpt) > 0 {
		forceBudget = forceBudgetOpt[0]
	}
	return newInstallSyncerWithOptions(client, targets, forceBudget, "")
}

func newInstallSyncerWithOptions(client *gh.Client, targets []tools.Tool, forceBudget bool, aliasName string) *sync.Syncer {
	return &sync.Syncer{
		Client:      sync.WrapGitHubClient(client),
		Provider:    provider.NewCompositeProvider(provider.NewGitHubProvider(provider.WrapGitHubClient(client))),
		Tools:       targets,
		Executor:    &sync.ShellExecutor{},
		ProjectRoot: resolveCurrentProjectRoot(),
		ForceBudget: forceBudget,
		AliasName:   aliasName,
	}
}

func configureInstallNameConflictResolver(syncer *sync.Syncer, conflictMode workflow.ConflictMode, aliasName string) {
	if syncer != nil && conflictMode == workflow.ConflictModeInteractive && aliasName == "" {
		syncer.NameConflictResolver = workflow.PromptNameConflictResolution
	}
}

func resolveCurrentProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	projectFile, err := projectfile.Find(wd)
	if err != nil || projectFile == "" {
		return ""
	}
	return filepath.Dir(projectFile)
}

// wireInstallSyncer attaches an Emit callback that prints progress (or
// collects results for JSON output) and returns the result slice pointer.
func wireInstallSyncer(syncer *sync.Syncer, registryRepo string, useJSON bool) *installRunResults {
	results := &installRunResults{}
	syncer.Emit = func(msg any) {
		switch m := msg.(type) {
		case sync.SkillInstalledMsg:
			if useJSON {
				status := "installed"
				if m.Updated {
					status = "updated"
				}
				results.Installed = append(results.Installed, installResult{
					Name: m.Name, Registry: registryRepo, Status: status,
				})
			} else {
				verb := "installed"
				if m.Updated {
					verb = "updated"
				}
				fmt.Printf("  ✓ %-24s %s\n", m.Name, verb)
			}
		case sync.SkillErrorMsg:
			if useJSON {
				results.Installed = append(results.Installed, installResult{
					Name: m.Name, Registry: registryRepo, Status: "error", Error: m.Err.Error(),
				})
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %-24s error: %v\n", m.Name, m.Err)
			}
		case sync.BudgetWarningMsg:
			if !useJSON {
				fmt.Fprintf(os.Stderr, "warning: %s\n", m.Message)
			}
		case sync.NameConflictResolvedMsg:
			payload := conflictResolutionPayload(m.Conflict, m.Resolution)
			results.Resolution = &payload
		}
	}
	return results
}

// discoverEntries fetches the diff for each registry and tags every result
// with its source registry.
func discoverEntries(
	ctx context.Context,
	repos []string,
	client *gh.Client,
	targets []tools.Tool,
	st *state.State,
) ([]browseEntry, []error) {
	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(client),
		Provider: provider.NewCompositeProvider(provider.NewGitHubProvider(provider.WrapGitHubClient(client))),
		Tools:    targets,
	}

	var entries []browseEntry
	var errs []error
	for _, repo := range repos {
		statuses, _, err := syncer.Diff(ctx, repo, st)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", repo, err))
			continue
		}
		for _, s := range statuses {
			// Skip extras (local-only) — `add` is for installing FROM registries.
			if s.Status == sync.StatusExtra {
				continue
			}
			entries = append(entries, browseEntry{Status: s, Registry: repo})
		}
	}
	return entries, errs
}

func discoverSourceEntries(
	ctx context.Context,
	sources []config.RegistrySource,
	client *gh.Client,
	targets []tools.Tool,
	st *state.State,
) ([]browseEntry, []error) {
	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(client),
		Provider: provider.NewCompositeProvider(provider.NewGitHubProvider(provider.WrapGitHubClient(client))),
		Tools:    targets,
	}

	var entries []browseEntry
	var errs []error
	for _, src := range sources {
		sourceKey := registryStateKey(src)
		statuses, _, err := syncer.DiffSource(ctx, sourceKey, src.Source, st)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.ID, err))
			continue
		}
		for _, s := range statuses {
			// Skip extras (local-only) — `add` is for installing FROM registries.
			if s.Status == sync.StatusExtra {
				continue
			}
			entries = append(entries, browseEntry{
				Status:    s,
				Registry:  registryDisplay(src),
				SourceKey: sourceKey,
				Source:    src.Source,
			})
		}
	}
	return entries, errs
}

// filterEntries returns entries whose name or description contains the query
// (case-insensitive).
func filterEntries(entries []browseEntry, query string) []browseEntry {
	q := strings.ToLower(query)
	var out []browseEntry
	for _, e := range entries {
		name := strings.ToLower(e.Status.Name)
		desc := ""
		if e.Status.Entry != nil {
			desc = strings.ToLower(e.Status.Entry.Description)
		}
		if strings.Contains(name, q) || strings.Contains(desc, q) {
			out = append(out, e)
		}
	}
	return out
}

// sortEntries orders entries by registry, then alphabetically by name.
func sortEntries(entries []browseEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Registry != entries[j].Registry {
			return entries[i].Registry < entries[j].Registry
		}
		return entries[i].Status.Name < entries[j].Status.Name
	})
}

// emitBrowseJSON emits the discovered entries as JSON for non-TTY/--json mode.
func emitBrowseJSON(entries []browseEntry) error {
	return emitBrowseJSONForCommand(nil, entries)
}

func emitBrowseJSONForCommand(cmd *cobra.Command, entries []browseEntry) error {
	type row struct {
		Name        string `json:"name"`
		Registry    string `json:"registry"`
		SourceKey   string `json:"source_key,omitempty"`
		Source      any    `json:"source,omitempty"`
		Status      string `json:"status"`
		Version     string `json:"version,omitempty"`
		Description string `json:"description,omitempty"`
		Author      string `json:"author,omitempty"`
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		desc := ""
		if e.Status.Entry != nil {
			desc = e.Status.Entry.Description
		}
		rows = append(rows, row{
			Name:        e.Status.Name,
			Registry:    e.Registry,
			SourceKey:   e.SourceKey,
			Source:      jsonSource(e.Source),
			Status:      e.Status.Status.String(),
			Version:     e.Status.DisplayVersion(),
			Description: desc,
			Author:      e.Status.Maintainer,
		})
	}
	payload := map[string]any{"results": rows}
	if cmd == nil {
		return json.NewEncoder(os.Stdout).Encode(payload)
	}
	return renderMutatorEnvelope(cmd, payload, envelope.StatusOK)
}

func registryDisplay(src config.RegistrySource) string {
	if src.Config.Repo != "" {
		return src.Config.Repo
	}
	if src.ID != "" {
		return src.ID
	}
	return src.Identity.Key
}

func registryStateKey(src config.RegistrySource) string {
	if src.Config.Source == nil && src.Config.Repo != "" && isLegacyGitHubSource(src.Source) {
		return src.Config.Repo
	}
	return src.Identity.Key
}

func jsonSource(spec source.SourceSpec) any {
	if spec.Type == "" {
		return nil
	}
	return spec
}

// emitInstallJSON emits per-skill install results as JSON.
func emitInstallJSON(results []installResult, resolution *nameConflictResolutionPayload, cmd *cobra.Command) error {
	payload := map[string]any{"installed": results}
	if resolution != nil {
		payload["resolution"] = resolution
	}
	if cmd == nil {
		return json.NewEncoder(os.Stdout).Encode(payload)
	}
	status := envelope.StatusOK
	for _, result := range results {
		if result.Status == "error" {
			status = envelope.StatusPartialSuccess
			break
		}
	}
	return renderMutatorEnvelope(cmd, payload, status)
}

// runInstallBrowser launches the interactive install browser.
func runInstallBrowser(entries []browseEntry, initialQuery string) ([]browseEntry, error) {
	m := newInstallModel(entries, initialQuery)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("TUI error: %w", err)
	}
	fm, ok := finalModel.(installModel)
	if !ok || fm.quitting || !fm.confirmed {
		return nil, nil
	}
	return fm.selectedEntries(), nil
}
