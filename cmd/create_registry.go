package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/huh"
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

	isTTY := isatty.IsTerminal(os.Stdout.Fd())

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

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := gh.NewClient(cfg.Token)

	description := fmt.Sprintf("%s dev team skill stack", titleCase(team))
	_, err = client.CreateRepo(ctx, owner, repo, description, private)
	if err != nil {
		if !errors.Is(err, gh.ErrRepoExists) {
			return err
		}
		// Repo already exists.
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

	// Check if scribe.toml already exists in the repo.
	repoSlug := owner + "/" + repo
	hasManifest, err := client.FileExists(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		// If the repo is empty (no commits), FileExists will 404 on HEAD — treat as no manifest.
		hasManifest = false
	}

	if !hasManifest {
		fmt.Printf("Pushing initial scribe.toml and README.md...\n")
		files := map[string]string{
			"scribe.toml": scaffoldTOML(team),
			"README.md":   scaffoldREADME(team, repoSlug),
		}
		if err := client.CreateInitialCommit(ctx, owner, repo, files, "Initialize skill registry"); err != nil {
			return fmt.Errorf("push initial commit: %w", err)
		}
	}

	fmt.Printf("\nRegistry created: %s\n\n", repoSlug)
	return connectToRepo(repoSlug)
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

// scaffoldTOML generates the initial scribe.toml content for a new registry.
func scaffoldTOML(team string) string {
	return fmt.Sprintf(`[team]
name = %q
description = "%s dev team skill stack"

# Add skills here. Format:
# "skill-name" = { source = "github:owner/repo@version" }
# "my-skill"   = { source = "github:Owner/repo@main", path = "username/my-skill" }

[skills]
`, team, titleCase(team))
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
