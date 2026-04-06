package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type addResult struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Source   string `json:"source,omitempty"`
	Uploaded bool   `json:"uploaded"`
	Error    string `json:"error,omitempty"`
}

var addCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a skill to a team registry",
	Long: `Add a skill to a team registry on GitHub.

If the skill has a known source (synced from another registry), adds a
source reference. If it's a local-only skill, uploads the files to the
registry.

With no arguments in a terminal, shows an interactive browser to select
skills. In non-TTY mode, the skill name is required.

Examples:
  scribe add cleanup
  scribe add gstack --registry ArtistfyHQ/team-skills
  scribe add --yes cleanup`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	addCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	addCmd.Flags().String("registry", "", "Target registry (owner/repo)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	addYes, _ := cmd.Flags().GetBool("yes")
	addJSON, _ := cmd.Flags().GetBool("json")
	addRegistry, _ := cmd.Flags().GetString("registry")

	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
	useJSON := addJSON || !isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.TeamRepos()) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	client := gh.NewClient(cmd.Context(), cfg.Token)
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
	}

	targets := []tools.Tool{tools.ClaudeTool{}, tools.CursorTool{}}
	adder := &add.Adder{Client: client, Tools: targets}

	// Resolve target registry.
	targetRepo, err := resolveTargetRegistry(addRegistry, cfg.TeamRepos(), isTTY)
	if err != nil {
		return fmt.Errorf("resolve target registry: %w", err)
	}

	// Mode 3: no args, non-TTY.
	if len(args) == 0 && !isTTY {
		return fmt.Errorf("skill name required when not running interactively")
	}

	// Discover candidates.
	localCandidates, err := adder.DiscoverLocal(st)
	if err != nil {
		return fmt.Errorf("discover local skills: %w", err)
	}

	// Fetch target registry manifest to filter already-added skills.
	ctx := cmd.Context()
	targetOwner, targetRepoName, err := manifest.ParseOwnerRepo(targetRepo)
	if err != nil {
		return fmt.Errorf("invalid target registry: %w", err)
	}
	targetManifest, err := fetchRegistryManifest(ctx, client, targetOwner, targetRepoName)
	if err != nil {
		return fmt.Errorf("fetch target registry: %w", err)
	}

	// Fetch other registries for remote discovery.
	otherManifests := map[string]*manifest.Manifest{}
	for _, repo := range cfg.TeamRepos() {
		if repo == targetRepo {
			continue
		}
		o, r, err := manifest.ParseOwnerRepo(repo)
		if err != nil {
			continue // skip malformed registry entries
		}
		m, err := fetchRegistryManifest(ctx, client, o, r)
		if err != nil || !m.IsRegistry() {
			continue
		}
		otherManifests[repo] = m
	}

	remoteCandidates := adder.DiscoverRemote(targetManifest, otherManifests)

	// Merge and filter: remove skills already in target.
	allCandidates := filterAlreadyInTarget(
		append(localCandidates, remoteCandidates...),
		targetManifest,
	)

	if len(args) == 1 {
		return runAddByName(ctx, args[0], allCandidates, adder, targetRepo, cfg, st, client, targets, useJSON, isTTY, addYes)
	}

	// Mode 2: interactive browse (TTY, no args) — Task 7.
	return runAddInteractive(ctx, allCandidates, adder, targetRepo, cfg, st, client, targets, useJSON, addYes)
}

func runAddByName(
	ctx context.Context,
	name string,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	isTTY bool,
	skipConfirm bool,
) error {
	// Find the candidate.
	var found *add.Candidate
	for _, c := range candidates {
		if c.Name == name {
			found = &c
			break
		}
	}
	if found == nil {
		return fmt.Errorf("skill %q not found locally or in connected registries", name)
	}

	// Confirmation.
	if !skipConfirm && isTTY {
		action := "add reference"
		if found.NeedsUpload() {
			action = "upload files"
		}
		var confirm bool
		err := huh.NewConfirm().
			Title(fmt.Sprintf("%s %q to %s?", action, name, targetRepo)).
			Value(&confirm).
			Run()
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	results := wireAddEmit(adder, targetRepo, useJSON)

	if err := adder.Add(ctx, targetRepo, []add.Candidate{*found}); err != nil {
		return err
	}

	return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
}

func runAddInteractive(
	ctx context.Context,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	skipConfirm bool,
) error {
	if len(candidates) == 0 {
		fmt.Printf("All available skills are already in %s.\n", targetRepo)
		return nil
	}

	// Sort: local first, then remote, alphabetical within each.
	sortCandidates(candidates)

	m := newAddModel(candidates, targetRepo)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fm, ok := finalModel.(addModel)
	if !ok || fm.quitting || !fm.confirmed {
		return nil
	}

	selected := fm.selectedCandidates()
	if len(selected) == 0 {
		return nil
	}

	// Confirmation (unless --yes).
	if !skipConfirm {
		fmt.Printf("\nAdding %d skill(s) to %s:\n", len(selected), targetRepo)
		for _, c := range selected {
			action := "reference"
			if c.NeedsUpload() {
				action = "upload"
			}
			fmt.Printf("  • %s (%s)\n", c.Name, action)
		}

		var confirm bool
		if err := huh.NewConfirm().Title("Proceed?").Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	results := wireAddEmit(adder, targetRepo, useJSON)

	if err := adder.Add(ctx, targetRepo, selected); err != nil {
		return err
	}

	return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
}

// sortCandidates groups by origin (local first, then remote), then by package,
// alphabetical within each group.
func sortCandidates(candidates []add.Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		iLocal := candidates[i].Origin == "local"
		jLocal := candidates[j].Origin == "local"
		if iLocal != jLocal {
			return iLocal
		}
		// Within local: standalone first, then by package name.
		pkgI, pkgJ := candidates[i].Package, candidates[j].Package
		if pkgI != pkgJ {
			if pkgI == "" {
				return true
			}
			if pkgJ == "" {
				return false
			}
			return pkgI < pkgJ
		}
		return candidates[i].Name < candidates[j].Name
	})
}

// resolveTargetRegistry determines which registry to add skills to.
func resolveTargetRegistry(flag string, repos []string, isTTY bool) (string, error) {
	if flag != "" {
		return resolveRegistry(flag, repos)
	}
	if len(repos) == 1 {
		return repos[0], nil
	}
	// Multiple registries, no flag.
	if !isTTY {
		return "", fmt.Errorf("multiple registries connected — pass --registry owner/repo")
	}
	// Interactive picker.
	var selected string
	opts := make([]huh.Option[string], len(repos))
	for i, r := range repos {
		opts[i] = huh.NewOption(r, r)
	}
	err := huh.NewSelect[string]().
		Title("Which registry?").
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}

// filterAlreadyInTarget removes candidates that are already in the target registry.
func filterAlreadyInTarget(candidates []add.Candidate, targetManifest *manifest.Manifest) []add.Candidate {
	// Also deduplicate by name (local wins — it appears first).
	seen := map[string]bool{}
	var filtered []add.Candidate
	for _, c := range candidates {
		if seen[c.Name] {
			continue
		}
		if targetManifest.FindByName(c.Name) != nil {
			continue
		}
		seen[c.Name] = true
		filtered = append(filtered, c)
	}
	return filtered
}

// wireAddEmit sets up the Adder's Emit callback to collect results. Returns
// a pointer to the results slice so the caller can read it after Add completes.
func wireAddEmit(adder *add.Adder, targetRepo string, useJSON bool) *[]addResult {
	results := &[]addResult{}
	adder.Emit = func(msg any) {
		switch m := msg.(type) {
		case add.SkillAddingMsg:
			if !useJSON {
				verb := "adding reference"
				if m.Upload {
					verb = "uploading"
				}
				fmt.Printf("  %s %s...\n", verb, m.Name)
			}
		case add.SkillAddedMsg:
			if useJSON {
				*results = append(*results, addResult{
					Name: m.Name, Registry: m.Registry, Source: m.Source, Uploaded: m.Upload,
				})
			} else {
				fmt.Printf("  ✓ %s added to %s\n", m.Name, m.Registry)
			}
		case add.SkillAddErrorMsg:
			if useJSON {
				*results = append(*results, addResult{Name: m.Name, Registry: targetRepo, Error: m.Err.Error()})
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", m.Name, m.Err)
			}
		}
	}
	return results
}

// finishAdd runs auto-sync and optionally outputs JSON after add completes.
func finishAdd(ctx context.Context, results []addResult, targetRepo string, st *state.State, client *gh.Client, targets []tools.Tool, useJSON bool) error {
	synced := autoSync(ctx, targetRepo, st, client, targets, useJSON)
	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"added":  results,
			"synced": synced,
		})
	}
	return nil
}

// autoSync runs a sync for the target registry after adding skills.
func autoSync(ctx context.Context, targetRepo string, st *state.State, client *gh.Client, targets []tools.Tool, useJSON bool) bool {
	syncer := &sync.Syncer{
		Client:  sync.WrapGitHubClient(client),
		Tools: targets,
		Emit: func(msg any) {
			if useJSON {
				return
			}
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				verb := "installed"
				if m.Updated {
					verb = "updated to"
				}
				fmt.Printf("  %-20s %s %s\n", m.Name, verb, m.Version)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
			}
		},
	}

	if !useJSON {
		fmt.Printf("\nsyncing %s...\n\n", targetRepo)
	}
	if err := syncer.Run(ctx, targetRepo, st); err != nil {
		fmt.Fprintf(os.Stderr, "warning: sync failed: %v\nrun `scribe sync` to retry\n", err)
		return false
	}
	return true
}

// fetchRegistryManifest fetches a manifest, trying scribe.yaml first then
// falling back to scribe.toml (converting TOML to the new format via migrate).
func fetchRegistryManifest(ctx context.Context, client *gh.Client, owner, repo string) (*manifest.Manifest, error) {
	m, _, err := manifest.FetchWithFallback(ctx, client, owner, repo, migrate.Convert)
	return m, err
}
