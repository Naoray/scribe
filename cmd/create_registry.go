package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
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
		if err := confirmOrRequire(&private, "private", "Private repository?", isTTY); err != nil {
			return err
		}
	}

	// Validate inputs (#1, #2).
	if err := validateGitHubName(team, "team name"); err != nil {
		return err
	}
	if err := validateGitHubName(owner, "owner"); err != nil {
		return err
	}
	if err := validateGitHubName(repo, "repo name"); err != nil {
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

	desc := teamDescription(team)
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
			manifest.ManifestFilename: scaffoldYAML(team),
			"README.md":              scaffoldREADME(team, repoSlug),
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
func confirmOrRequire(value *bool, flag, title string, isTTY bool) error {
	if !isTTY {
		return nil
	}
	return huh.NewConfirm().Title(title).Value(value).Run()
}

// ghNameRe matches valid GitHub owner and repo names (alphanumeric, hyphens, dots, underscores).
var ghNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// validateGitHubName checks that s is a valid GitHub owner/repo/team name.
func validateGitHubName(s, label string) error {
	if !ghNameRe.MatchString(s) {
		return fmt.Errorf("%s %q is invalid: use only letters, numbers, hyphens, dots, or underscores", label, s)
	}
	return nil
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

func teamDescription(team string) string {
	return fmt.Sprintf("%s dev team skill stack", titleCase(team))
}

// scaffoldYAML generates the initial scribe.yaml content for a new registry.
func scaffoldYAML(team string) string {
	return fmt.Sprintf(`apiVersion: scribe/v1
kind: Registry
team:
  name: %q
  description: %q

# Add skills here. Example:
# catalog:
#   - name: my-skill
#     source: "github:owner/repo@version"
#     author: username

catalog: []
`, team, teamDescription(team))
}

// scaffoldREADME generates the initial README.md content for a new registry.
func scaffoldREADME(team, repo string) string {
	title := titleCase(team)
	return fmt.Sprintf(`# %s — Skill Registry

Shared skill registry managed by [Scribe](https://github.com/Naoray/scribe).

## Setup

Install scribe, then connect:

    scribe connect %s

## Sync

Pull the latest skills to your machine:

    scribe sync

## Adding skills

Edit `+"`scribe.yaml`"+` to add or update skills, then push to this repo.
Teammates run `+"`scribe sync`"+` to pick up changes.
`, title, repo)
}

// titleCase capitalises the first letter of each segment separated by hyphens.
func titleCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, "-")
}
