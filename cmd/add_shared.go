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

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

var githubURLInDescriptionRE = regexp.MustCompile(`https?://github\.com/[^\s)]+`)

// addResult is the per-skill result emitted by registry-push flows.
type addResult struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Source   string `json:"source,omitempty"`
	Uploaded bool   `json:"uploaded"`
	Error    string `json:"error,omitempty"`
}

// runAddByName performs a name-based registry push (used by `scribe registry add <name>`).
func runAddByName(
	ctx context.Context,
	name string,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
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

	warnOnMissingSourceAttribution([]add.Candidate{*found})

	results := wireAddEmit(adder, targetRepo, useJSON)

	if err := adder.Add(ctx, targetRepo, []add.Candidate{*found}); err != nil {
		return err
	}

	return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
}

// runAddInteractive launches the legacy registry-push browser TUI.
func runAddInteractive(
	ctx context.Context,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
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

	warnOnMissingSourceAttribution(selected)

	results := wireAddEmit(adder, targetRepo, useJSON)

	if err := adder.Add(ctx, targetRepo, selected); err != nil {
		return err
	}

	return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
}

func warnOnMissingSourceAttribution(candidates []add.Candidate) {
	for _, candidate := range candidates {
		if warning := missingSourceWarning(candidate); warning != "" {
			fmt.Fprintln(os.Stderr, warning)
		}
	}
}

func missingSourceWarning(candidate add.Candidate) string {
	if candidate.Attribution != (discovery.Source{}) {
		return ""
	}
	description := candidate.RawDescription
	if description == "" {
		description = candidate.Description
	}
	if strings.Contains(description, "⛔️") {
		return ""
	}
	if !githubURLInDescriptionRE.MatchString(description) {
		return ""
	}
	return fmt.Sprintf("warning: %s mentions a GitHub URL in its description but has no source frontmatter", candidate.Name)
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
	if !isTTY {
		return "", fmt.Errorf("multiple registries connected — pass --registry owner/repo")
	}
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

// wireAddEmit sets up the Adder's Emit callback to collect results.
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
	projectRoot := resolveCurrentProjectRoot()
	var kitFilter []string
	var kitFilterEnabled bool
	if projectRoot != "" && st != nil {
		kitFilter, kitFilterEnabled = workflow.ResolveKitFilter(st)
	}

	syncer := &sync.Syncer{
		Client:           sync.WrapGitHubClient(client),
		Tools:            targets,
		ProjectRoot:      projectRoot,
		KitFilter:        kitFilter,
		KitFilterEnabled: kitFilterEnabled,
		Emit: func(msg any) {
			if useJSON {
				return
			}
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				verb := "installed"
				if m.Updated {
					verb = "updated"
				}
				fmt.Printf("  %-20s %s\n", m.Name, verb)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
			case sync.BudgetWarningMsg:
				fmt.Fprintf(os.Stderr, "warning: %s\n", m.Message)
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
