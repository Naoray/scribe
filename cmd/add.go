package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

var (
	addYes      bool
	addJSON     bool
	addRegistry string
)

var addCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a skill to a team registry",
	Long: `Add a skill to a team registry's scribe.toml on GitHub.

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
	addCmd.Flags().BoolVar(&addYes, "yes", false, "Skip confirmation prompt")
	addCmd.Flags().BoolVar(&addJSON, "json", false, "Output machine-readable JSON")
	addCmd.Flags().StringVar(&addRegistry, "registry", "", "Target registry (owner/repo)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	useJSON := addJSON || !isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
	}

	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}
	adder := &add.Adder{Client: client, Targets: tgts}

	// Resolve target registry.
	targetRepo, err := resolveTargetRegistry(addRegistry, cfg.TeamRepos, isTTY)
	if err != nil {
		return err
	}

	// Mode 3: no args, non-TTY.
	if len(args) == 0 && !isTTY {
		return fmt.Errorf("skill name required when not running interactively")
	}

	// Discover candidates.
	localCandidates, err := adder.DiscoverLocal(st)
	if err != nil {
		return err
	}

	// Fetch target registry manifest to filter already-added skills.
	ctx := context.Background()
	targetOwner, targetRepoName, _ := parseOwnerRepo(targetRepo)
	targetRaw, err := client.FetchFile(ctx, targetOwner, targetRepoName, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("fetch target registry: %w", err)
	}
	targetManifest, err := manifest.Parse(targetRaw)
	if err != nil {
		return fmt.Errorf("parse target registry: %w", err)
	}

	// Fetch other registries for remote discovery.
	otherManifests := map[string]*manifest.Manifest{}
	for _, repo := range cfg.TeamRepos {
		if repo == targetRepo {
			continue
		}
		o, r, _ := parseOwnerRepo(repo)
		raw, err := client.FetchFile(ctx, o, r, "scribe.toml", "HEAD")
		if err != nil {
			continue // skip unreachable registries
		}
		m, err := manifest.Parse(raw)
		if err != nil || !m.IsLoadout() {
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
		return runAddByName(ctx, args[0], allCandidates, adder, targetRepo, cfg, st, client, tgts, useJSON, isTTY)
	}

	// Mode 2: interactive browse (TTY, no args) — Task 7.
	return runAddInteractive(ctx, allCandidates, adder, targetRepo, cfg, st, client, tgts, useJSON)
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
	tgts []targets.Target,
	useJSON bool,
	isTTY bool,
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
	if !addYes && isTTY {
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

	// Wire events.
	type addResult struct {
		Name     string `json:"name"`
		Registry string `json:"registry"`
		Source   string `json:"source"`
		Uploaded bool   `json:"uploaded"`
	}
	var results []addResult
	var failed int

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
				results = append(results, addResult{
					Name:     m.Name,
					Registry: m.Registry,
					Source:   m.Source,
					Uploaded: m.Upload,
				})
			} else {
				fmt.Printf("  ✓ %s added to %s\n", m.Name, m.Registry)
			}
		case add.SkillAddErrorMsg:
			failed++
			if useJSON {
				results = append(results, addResult{Name: m.Name, Registry: targetRepo})
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", m.Name, m.Err)
			}
		}
	}

	if err := adder.Add(ctx, targetRepo, []add.Candidate{*found}); err != nil {
		return err
	}

	// Auto-sync.
	synced := autoSync(ctx, targetRepo, st, client, tgts, useJSON)

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"added":  results,
			"synced": synced,
		})
	}

	return nil
}

// runAddInteractive is the Bubble Tea browse mode — implemented in Task 7.
func runAddInteractive(
	ctx context.Context,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	tgts []targets.Target,
	useJSON bool,
) error {
	return fmt.Errorf("interactive mode not yet implemented — pass a skill name")
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
		if _, exists := targetManifest.Skills[c.Name]; exists {
			continue
		}
		seen[c.Name] = true
		filtered = append(filtered, c)
	}
	return filtered
}

// autoSync runs a sync for the target registry after adding skills.
func autoSync(ctx context.Context, targetRepo string, st *state.State, client *gh.Client, tgts []targets.Target, useJSON bool) bool {
	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
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
		if !useJSON {
			fmt.Fprintf(os.Stderr, "warning: sync failed: %v\nrun `scribe sync` to retry\n", err)
		}
		return false
	}
	return true
}
