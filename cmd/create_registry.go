package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/scaffold"
	"github.com/Naoray/scribe/internal/workflow"
)

var createRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Scaffold a new team skills registry on GitHub",
	Long: `Create a new GitHub repository with a scribe.yaml manifest and connect to it.

Examples:
  scribe create registry                                    # interactive
  scribe create registry -t myteam -o MyOrg                 # flags
  scribe create registry -t myteam -o MyOrg -r skills-repo  # custom repo name`,
	RunE: runCreateRegistry,
}

func init() {
	createRegistryCmd.Flags().StringP("team", "t", "", "Team name")
	createRegistryCmd.Flags().StringP("owner", "o", "", "GitHub org or username")
	createRegistryCmd.Flags().StringP("repo", "r", "team-registry", "Repository name")
	createRegistryCmd.Flags().Bool("private", true, "Create a private repository")
}

func runCreateRegistry(cmd *cobra.Command, args []string) error {
	team, _ := cmd.Flags().GetString("team")
	owner, _ := cmd.Flags().GetString("owner")
	repo, _ := cmd.Flags().GetString("repo")
	private, _ := cmd.Flags().GetBool("private")

	isTTY := isatty.IsTerminal(os.Stdin.Fd())

	// Prompt for missing values if TTY; error if non-TTY.
	if err := promptOrRequire(&team, "team", "What's your team name?", isTTY); err != nil {
		return err
	}
	if err := promptOrRequire(&owner, "owner", "GitHub org or username?", isTTY); err != nil {
		return err
	}
	if !cmd.Flags().Changed("repo") {
		if err := promptOrRequire(&repo, "repo", "Repository name?", isTTY); err != nil {
			return err
		}
	}
	if !cmd.Flags().Changed("private") {
		if err := confirmOrRequire(&private, "Private repository?", isTTY); err != nil {
			return err
		}
	}

	// Validate inputs (#1, #2).
	if err := scaffold.ValidateGitHubName(team, "team name"); err != nil {
		return err
	}
	if err := scaffold.ValidateGitHubName(owner, "owner"); err != nil {
		return err
	}
	if err := scaffold.ValidateGitHubName(repo, "repo name"); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := gh.NewClient(cmd.Context(), cfg.Token)

	// Auth check (#3) — creating repos requires authentication.
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required to create repositories — run `gh auth login` or set GITHUB_TOKEN")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	desc := scaffold.TeamDescription(team)
	created, err := client.CreateRepo(ctx, owner, repo, desc, private)
	if err != nil {
		if !errors.Is(err, gh.ErrRepoExists) {
			return err
		}
		if !isTTY {
			return fmt.Errorf("repository %s/%s already exists", owner, repo)
		}
		var useExisting bool
		if err := huh.NewConfirm().Title(fmt.Sprintf("Repo %s/%s already exists. Use it?", owner, repo)).Value(&useExisting).Run(); err != nil {
			return err
		}
		if !useExisting {
			return fmt.Errorf("aborted")
		}
	}

	// Use canonical owner/repo from GitHub when available (#11).
	if created != nil {
		owner = created.GetOwner().GetLogin()
		repo = created.GetName()
	}
	repoSlug := owner + "/" + repo

	hasManifest, err := client.FileExists(ctx, owner, repo, manifest.ManifestFilename, "HEAD")
	if err != nil {
		return fmt.Errorf("check for existing manifest: %w", err)
	}
	if !hasManifest {
		// Also check for legacy format.
		hasManifest, err = client.FileExists(ctx, owner, repo, manifest.LegacyManifestFilename, "HEAD")
		if err != nil {
			return fmt.Errorf("check for legacy manifest: %w", err)
		}
	}

	if !hasManifest {
		fmt.Fprintf(os.Stderr, "Pushing initial %s and README.md...\n", manifest.ManifestFilename)
		files := map[string]string{
			manifest.ManifestFilename: scaffold.ScaffoldYAML(team),
			"README.md":              scaffold.ScaffoldREADME(team, repoSlug),
		}
		if err := client.PushFiles(ctx, owner, repo, files, "Initialize skill registry"); err != nil {
			fmt.Fprintf(os.Stderr, "\nRepo %s was created but the initial commit failed: %v\n", repoSlug, err)
			fmt.Fprintf(os.Stderr, "Run the command again to retry, or delete the repo at https://github.com/%s\n", repoSlug)
			return fmt.Errorf("push initial commit: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "\nRegistry created: %s\n\n", repoSlug)

	// Connect to the newly created registry via the connect workflow tail.
	// Config and Client are already loaded — skip LoadConfig.
	bag := &workflow.Bag{
		RepoArg:  repoSlug,
		Config:   cfg,
		Client:   client,
		Provider: provider.NewGitHubProvider(provider.WrapGitHubClient(client)),
	}
	if err := workflow.Run(ctx, workflow.ConnectTail(), bag); err != nil {
		fmt.Fprintf(os.Stderr, "\nRepo %s was created but connecting failed: %v\n", repoSlug, err)
		fmt.Fprintf(os.Stderr, "Run `scribe connect %s` to retry.\n", repoSlug)
		return err
	}
	return nil
}

// promptOrRequire prompts for a missing string value in TTY mode, or returns an
// error in non-TTY mode. If the value is already non-empty, it's a no-op.
func promptOrRequire(value *string, flag, title string, isTTY bool) error {
	if *value != "" {
		return nil
	}
	if !isTTY {
		return fmt.Errorf("--%s is required in non-interactive mode", flag)
	}
	return huh.NewInput().Title(title).Validate(notEmpty(flag)).Value(value).Run()
}

// confirmOrRequire prompts for a bool value in TTY mode. In non-TTY mode it's
// a no-op (the flag default is used).
func confirmOrRequire(value *bool, title string, isTTY bool) error {
	if !isTTY {
		return nil
	}
	return huh.NewConfirm().Title(title).Value(value).Run()
}

// notEmpty returns a huh validation function that rejects empty strings.
func notEmpty(field string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s cannot be empty", field)
		}
		return nil
	}
}
