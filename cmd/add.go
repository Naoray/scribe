package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// skillRefPattern matches "owner/repo:skillname" — direct install reference.
var skillRefPattern = regexp.MustCompile(`^\w[\w.-]*/[\w.-]+:\S+$`)

// installResult is the per-skill JSON output for `scribe add`.
type installResult struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// browseEntry pairs a SkillStatus with the registry it came from.
type browseEntry struct {
	Status   sync.SkillStatus
	Registry string
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
  scribe add antfu/skills:nuxt --yes  # non-interactive
  scribe add react --json             # machine-readable search`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAdd,
	}
	cmd.Flags().Bool("yes", false, "Skip confirmation prompts")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().String("registry", "", "Limit search to a specific registry (owner/repo)")
	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	registryFilter, _ := cmd.Flags().GetString("registry")

	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	ctx := cmd.Context()
	client := gh.NewClient(ctx, cfg.Token)
	targets := []tools.Tool{tools.ClaudeTool{}, tools.CursorTool{}}

	// Direct install: owner/repo:skillname.
	if len(args) == 1 && skillRefPattern.MatchString(args[0]) {
		registryRepo, skillName, err := parseSkillRef(args[0])
		if err != nil {
			return err
		}
		if !client.IsAuthenticated() {
			return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
		}
		return runAddDirectInstall(ctx, registryRepo, skillName, cfg, st, client, targets, useJSON, skipConfirm)
	}

	// Need at least one connected registry to search/browse.
	if len(cfg.TeamRepos()) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
	}

	// Determine which registries to browse.
	repos := cfg.TeamRepos()
	if registryFilter != "" {
		repo, err := resolveRegistry(registryFilter, repos)
		if err != nil {
			return err
		}
		repos = []string{repo}
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
	entries, errs := discoverEntries(ctx, repos, client, targets, st)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}

	// Filter by query.
	if query != "" {
		entries = filterEntries(entries, query)
	}

	// JSON or non-TTY: just emit results.
	if useJSON {
		return emitBrowseJSON(entries)
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

	return installSelected(ctx, selected, cfg, st, client, targets, skipConfirm)
}

// parseSkillRef parses "owner/repo:skillname" into its parts.
func parseSkillRef(ref string) (registryRepo, skillName string, err error) {
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid skill reference %q: expected owner/repo:skillname", ref)
	}
	registryRepo = ref[:idx]
	skillName = ref[idx+1:]
	if _, _, perr := manifest.ParseOwnerRepo(registryRepo); perr != nil {
		return "", "", fmt.Errorf("invalid skill reference %q: %w", ref, perr)
	}
	if skillName == "" {
		return "", "", fmt.Errorf("invalid skill reference %q: skill name is empty", ref)
	}
	return registryRepo, skillName, nil
}

// runAddDirectInstall installs a single skill from owner/repo:skillname.
// Auto-connects the registry if it isn't already in config, but only after
// validating that the skill actually exists in the registry.
func runAddDirectInstall(
	ctx context.Context,
	registryRepo, skillName string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	skipConfirm bool,
) error {
	syncer := newInstallSyncer(client, targets)
	statuses, _, err := syncer.Diff(ctx, registryRepo, st)
	if err != nil {
		return fmt.Errorf("diff %s: %w", registryRepo, err)
	}

	var target *sync.SkillStatus
	for i := range statuses {
		if statuses[i].Name == skillName {
			target = &statuses[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("skill %q not found in %s", skillName, registryRepo)
	}

	// Skill exists — safe to auto-connect the registry now.
	if cfg.FindRegistry(registryRepo) == nil {
		cfg.AddRegistry(config.RegistryConfig{Repo: registryRepo})
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		if !useJSON {
			fmt.Printf("connected %s\n", registryRepo)
		}
	}

	if target.Status == sync.StatusCurrent {
		if useJSON {
			return emitInstallJSON([]installResult{{
				Name: target.Name, Registry: registryRepo, Status: "already-installed",
			}})
		}
		fmt.Printf("%s is already installed (current).\n", skillName)
		return nil
	}

	// Confirmation.
	if !skipConfirm && !useJSON {
		var confirm bool
		title := fmt.Sprintf("Install %s from %s?", skillName, registryRepo)
		if target.Status == sync.StatusOutdated {
			title = fmt.Sprintf("Update %s from %s?", skillName, registryRepo)
		}
		if err := huh.NewConfirm().Title(title).Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	results := wireInstallSyncer(syncer, registryRepo, useJSON)
	if err := syncer.RunWithDiff(ctx, registryRepo, []sync.SkillStatus{*target}, st); err != nil {
		return fmt.Errorf("install %s: %w", skillName, err)
	}

	if useJSON {
		return emitInstallJSON(*results)
	}
	return nil
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
) error {
	// Group by registry.
	byRegistry := map[string][]sync.SkillStatus{}
	order := []string{}
	for _, e := range selected {
		if _, seen := byRegistry[e.Registry]; !seen {
			order = append(order, e.Registry)
		}
		byRegistry[e.Registry] = append(byRegistry[e.Registry], e.Status)
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

	syncer := newInstallSyncer(client, targets)

	var installErr error
	for _, registryRepo := range order {
		// Auto-connect if needed.
		if cfg.FindRegistry(registryRepo) == nil {
			cfg.AddRegistry(config.RegistryConfig{Repo: registryRepo})
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
		if err := syncer.RunWithDiff(ctx, registryRepo, toInstall, st); err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			installErr = err
		}
	}
	return installErr
}

// newInstallSyncer constructs a Syncer ready to install skills.
func newInstallSyncer(client *gh.Client, targets []tools.Tool) *sync.Syncer {
	return &sync.Syncer{
		Client:   sync.WrapGitHubClient(client),
		Provider: provider.NewGitHubProvider(provider.WrapGitHubClient(client)),
		Tools:    targets,
		Executor: &sync.ShellExecutor{},
	}
}

// wireInstallSyncer attaches an Emit callback that prints progress (or
// collects results for JSON output) and returns the result slice pointer.
func wireInstallSyncer(syncer *sync.Syncer, registryRepo string, useJSON bool) *[]installResult {
	results := &[]installResult{}
	syncer.Emit = func(msg any) {
		switch m := msg.(type) {
		case sync.SkillInstalledMsg:
			if useJSON {
				status := "installed"
				if m.Updated {
					status = "updated"
				}
				*results = append(*results, installResult{
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
				*results = append(*results, installResult{
					Name: m.Name, Registry: registryRepo, Status: "error", Error: m.Err.Error(),
				})
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %-24s error: %v\n", m.Name, m.Err)
			}
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
		Provider: provider.NewGitHubProvider(provider.WrapGitHubClient(client)),
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
	type row struct {
		Name        string `json:"name"`
		Registry    string `json:"registry"`
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
			Status:      e.Status.Status.String(),
			Version:     e.Status.DisplayVersion(),
			Description: desc,
			Author:      e.Status.Maintainer,
		})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"results": rows})
}

// emitInstallJSON emits per-skill install results as JSON.
func emitInstallJSON(results []installResult) error {
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"installed": results})
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
