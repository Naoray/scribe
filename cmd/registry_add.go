package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func newRegistryAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [name|owner/repo]",
		Short: "Push a local skill or package to a team registry",
		Long: `Push a local skill or a package reference to a team registry on GitHub.

If the argument is a bare name, it's treated as a skill: either a source
reference is added (if the skill was synced from another registry) or the
files are uploaded directly to the target registry.

If the argument looks like "owner/repo", it's treated as a package
reference. When the upstream repo exposes a scribe.yaml package manifest,
its declared install commands are used automatically. Otherwise Scribe
prompts for an install command per detected tool (claude, cursor). Use
--install tool=command (repeatable) to supply commands non-interactively.

With no arguments in a terminal, shows an interactive browser to select
skills. In non-TTY mode, an argument is required.

Examples:
  scribe registry add cleanup
  scribe registry add gstack --registry ArtistfyHQ/team-skills
  scribe registry add obra/superpowers --install "claude=/plugin install superpowers"`,
		Args: cobra.MaximumNArgs(1),
		RunE: runRegistryAdd,
	}
	cmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().String("registry", "", "Target registry (owner/repo)")
	cmd.Flags().StringArray("install", nil, "Per-tool install command for package refs (tool=command, repeatable)")
	return cmd
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	addYes, _ := cmd.Flags().GetBool("yes")
	addJSON, _ := cmd.Flags().GetBool("json")
	addRegistry, _ := cmd.Flags().GetString("registry")
	installFlags, _ := cmd.Flags().GetStringArray("install")

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

	// Non-TTY without arg is an error.
	if len(args) == 0 && !isTTY {
		return fmt.Errorf("skill name or owner/repo required when not running interactively")
	}

	ctx := cmd.Context()

	// Package ref fast path — doesn't need local/remote discovery.
	if len(args) == 1 && strings.Contains(args[0], "/") {
		return runRegistryAddPackageRef(ctx, args[0], installFlags, adder, targetRepo, st, client, targets, useJSON, isTTY)
	}

	// Discover candidates.
	localCandidates, err := adder.DiscoverLocal(st)
	if err != nil {
		return fmt.Errorf("discover local skills: %w", err)
	}

	targetOwner, targetRepoName, err := manifest.ParseOwnerRepo(targetRepo)
	if err != nil {
		return fmt.Errorf("invalid target registry: %w", err)
	}
	targetManifest, err := fetchRegistryManifest(ctx, client, targetOwner, targetRepoName)
	if err != nil {
		return fmt.Errorf("fetch target registry: %w", err)
	}

	otherManifests := map[string]*manifest.Manifest{}
	for _, repo := range cfg.TeamRepos() {
		if repo == targetRepo {
			continue
		}
		o, r, err := manifest.ParseOwnerRepo(repo)
		if err != nil {
			continue
		}
		m, err := fetchRegistryManifest(ctx, client, o, r)
		if err != nil || !m.IsRegistry() {
			continue
		}
		otherManifests[repo] = m
	}

	remoteCandidates := adder.DiscoverRemote(targetManifest, otherManifests)
	allCandidates := filterAlreadyInTarget(
		append(localCandidates, remoteCandidates...),
		targetManifest,
	)

	if len(args) == 1 {
		return runAddByName(ctx, args[0], allCandidates, adder, targetRepo, cfg, st, client, targets, useJSON, isTTY, addYes)
	}

	return runAddInteractive(ctx, allCandidates, adder, targetRepo, cfg, st, client, targets, useJSON, addYes)
}

// runRegistryAddPackageRef adds an owner/repo reference to the target registry.
// If the upstream repo has a scribe.yaml package manifest, its installs are
// used. Otherwise, install commands are gathered per tool via --install flags
// (non-TTY) or interactive prompts (TTY).
func runRegistryAddPackageRef(
	ctx context.Context,
	packageRepo string,
	installFlags []string,
	adder *add.Adder,
	targetRepo string,
	st *state.State,
	client *gh.Client,
	targets []tools.Tool,
	useJSON bool,
	isTTY bool,
) error {
	results := wireAddEmit(adder, targetRepo, useJSON)

	// Try the package-manifest path first.
	err := adder.AddPackageRef(ctx, targetRepo, packageRepo)
	if err == nil {
		return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
	}

	// Fallback if the upstream repo has no package manifest (or isn't a package).
	if !isPackageManifestMissingErr(err) {
		return err
	}

	// Gather install commands.
	installs, perr := collectInstallCommands(packageRepo, installFlags, targets, isTTY)
	if perr != nil {
		return perr
	}
	if len(installs) == 0 {
		return fmt.Errorf("no install commands provided for %s — pass --install tool=command", packageRepo)
	}

	if err := pushBarePackageRef(ctx, adder, targetRepo, packageRepo, installs); err != nil {
		return err
	}

	return finishAdd(ctx, *results, targetRepo, st, client, targets, useJSON)
}

// isPackageManifestMissingErr returns true when AddPackageRef failed because
// the upstream repo doesn't expose a scribe.yaml package manifest.
func isPackageManifestMissingErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// "fetch package manifest for X: ..." — repo has no manifest at all.
	// "X is not a package (kind: Y)" — manifest exists but isn't a package.
	// "X declares no install commands ..." — package with empty installs.
	return strings.Contains(s, "fetch package manifest for ") ||
		strings.Contains(s, "is not a package") ||
		strings.Contains(s, "declares no install commands")
}

// collectInstallCommands returns a map of tool name → install command,
// either from --install flags or by prompting the user.
func collectInstallCommands(packageRepo string, flags []string, targets []tools.Tool, isTTY bool) (map[string]string, error) {
	// Parse --install flags first — they always win.
	installs := map[string]string{}
	validTools := map[string]bool{}
	for _, t := range targets {
		validTools[t.Name()] = true
	}

	for _, raw := range flags {
		tool, cmd, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --install value %q: expected tool=command", raw)
		}
		tool = strings.TrimSpace(tool)
		cmd = strings.TrimSpace(cmd)
		if tool == "" || cmd == "" {
			return nil, fmt.Errorf("invalid --install value %q: tool and command must be non-empty", raw)
		}
		if !validTools[tool] {
			return nil, fmt.Errorf("unknown tool %q in --install — expected one of: %s", tool, strings.Join(toolNames(targets), ", "))
		}
		installs[tool] = cmd
	}

	if len(installs) > 0 {
		return installs, nil
	}

	// Non-TTY without flags is a hard error.
	if !isTTY {
		return nil, fmt.Errorf("%s has no scribe.yaml package manifest — provide install commands via --install tool=command (e.g. --install \"claude=/plugin install foo\")", packageRepo)
	}

	// Interactive: prompt for each tool; empty = skip.
	fmt.Printf("\n%s has no scribe.yaml package manifest.\n", packageRepo)
	fmt.Println("Enter the install command for each tool (leave blank to skip):")
	fmt.Println()

	for _, t := range targets {
		var value string
		prompt := huh.NewInput().
			Title(fmt.Sprintf("Install command for %s", t.Name())).
			Placeholder("e.g. /plugin install superpowers").
			Value(&value)
		if err := prompt.Run(); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value != "" {
			installs[t.Name()] = value
		}
	}
	return installs, nil
}

func toolNames(targets []tools.Tool) []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.Name())
	}
	return names
}

// pushBarePackageRef appends a package-type catalog entry for packageRepo to
// the target registry's scribe.yaml using the given per-tool install commands.
// Used when the upstream repo has no scribe.yaml package manifest.
func pushBarePackageRef(ctx context.Context, adder *add.Adder, targetRepo, packageRepo string, installs map[string]string) error {
	targetOwner, targetRepoName, err := manifest.ParseOwnerRepo(targetRepo)
	if err != nil {
		return fmt.Errorf("parse target registry %q: %w", targetRepo, err)
	}
	pkgOwner, pkgRepoName, err := manifest.ParseOwnerRepo(packageRepo)
	if err != nil {
		return fmt.Errorf("parse package repo %q: %w", packageRepo, err)
	}

	m, err := fetchRegistryManifest(ctx, adder.Client, targetOwner, targetRepoName)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}
	if m.FindByName(pkgRepoName) != nil {
		return fmt.Errorf("%q is already in %s", pkgRepoName, targetRepo)
	}

	entry := manifest.Entry{
		Name:     pkgRepoName,
		Source:   fmt.Sprintf("github:%s/%s@main", pkgOwner, pkgRepoName),
		Type:     manifest.EntryTypePackage,
		Installs: installs,
		Author:   pkgOwner,
	}

	adder.Emit(add.SkillAddingMsg{Name: entry.Name, Upload: false})

	m.Catalog = append(m.Catalog, entry)
	encoded, err := m.Encode()
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	msg := fmt.Sprintf("add package: %s", entry.Name)
	if err := adder.Client.PushFiles(ctx, targetOwner, targetRepoName, map[string]string{
		manifest.ManifestFilename: string(encoded),
	}, msg); err != nil {
		return err
	}

	adder.Emit(add.SkillAddedMsg{
		Name:     entry.Name,
		Registry: targetRepo,
		Source:   entry.Source,
		Upload:   false,
	})
	return nil
}
