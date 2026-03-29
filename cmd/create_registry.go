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
)

var createRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Scaffold a new team skills registry on GitHub",
	Long: `Create a new GitHub repository with a scribe.toml manifest and connect to it.

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
	if team == "" {
		if !isTTY {
			return fmt.Errorf("--team is required in non-interactive mode")
		}
		if err := huh.NewInput().Title("What's your team name?").Validate(notEmpty("team name")).Value(&team).Run(); err != nil {
			return err
		}
	}

	if owner == "" {
		if !isTTY {
			return fmt.Errorf("--owner is required in non-interactive mode")
		}
		if err := huh.NewInput().Title("GitHub org or username?").Validate(notEmpty("owner")).Value(&owner).Run(); err != nil {
			return err
		}
	}

	if !cmd.Flags().Changed("repo") && isTTY {
		if err := huh.NewInput().Title("Repository name?").Value(&repo).Validate(notEmpty("repo name")).Run(); err != nil {
			return err
		}
	}

	if !cmd.Flags().Changed("private") && isTTY {
		if err := huh.NewConfirm().Title("Private repository?").Value(&private).Run(); err != nil {
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

	hasManifest, err := client.FileExists(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		// Surface real errors; FileExists already converts 404 → (false, nil).
		fmt.Fprintf(os.Stderr, "warning: could not check for scribe.toml: %v\n", err)
		hasManifest = false
	}

	if !hasManifest {
		fmt.Fprintf(os.Stderr, "Pushing initial scribe.toml and README.md...\n")
		files := map[string]string{
			"scribe.toml": scaffoldTOML(team),
			"README.md":   scaffoldREADME(team, repoSlug),
		}
		if err := client.PushFiles(ctx, owner, repo, files, "Initialize skill registry"); err != nil {
			return fmt.Errorf("push initial commit: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "\nRegistry created: %s\n\n", repoSlug)
	return connectToRepo(repoSlug, cfg, client)
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

// scaffoldTOML generates the initial scribe.toml content for a new registry.
func scaffoldTOML(team string) string {
	return fmt.Sprintf(`[team]
name = %q
description = %q

# Add skills here. Format:
# "skill-name" = { source = "github:owner/repo@version" }
# "my-skill"   = { source = "github:Owner/repo@main", path = "username/my-skill" }

[skills]
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

Edit `+"`scribe.toml`"+` to add or update skills, then push to this repo.
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
