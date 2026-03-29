package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

var connectCmd = &cobra.Command{
	Use:   "connect [owner/repo]",
	Short: "Connect to a team skills repo",
	Long: `Connect to a team skills repo so Scribe can sync your local skills.

The repo must contain a scribe.toml with a [team] section.

Examples:
  scribe connect ArtistfyHQ/team-skills
  scribe connect                          # interactive prompt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConnect,
}

func runConnect(cmd *cobra.Command, args []string) error {
	repo, err := resolveRepo(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	return connectToRepo(cmd.Context(), repo, cfg, gh.NewClient(cmd.Context(), cfg.Token))
}

// connectToRepo performs the connect-and-sync workflow for a given "owner/repo" string.
func connectToRepo(ctx context.Context, repo string, cfg *config.Config, client *gh.Client) error {
	owner, name, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return err
	}

	// Dedup check before any network calls (case-insensitive — GitHub repos are case-insensitive).
	for _, existing := range cfg.TeamRepos {
		if strings.EqualFold(existing, repo) {
			fmt.Printf("Already connected to %s\n", existing)
			return nil
		}
	}

	raw, err := client.FetchFile(ctx, owner, name, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("could not access %s: %w", repo, err)
	}

	m, err := manifest.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid scribe.toml in %s: %w", repo, err)
	}
	if !m.IsLoadout() {
		return fmt.Errorf("%s/scribe.toml has no [team] section — is this a skill package?", repo)
	}

	cfg.TeamRepos = append(cfg.TeamRepos, repo)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("Connected to %s\n", repo)

	// Auto-sync the newly connected repo only.
	st, err := state.Load()
	if err != nil {
		return err
	}

	tgts := resolveTargets(m.Targets)
	syncer := &sync.Syncer{
		Client:  sync.WrapGitHubClient(client),
		Targets: tgts,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				verb := "installed"
				if m.Updated {
					verb = "updated to"
				}
				fmt.Printf("  %-20s %s %s\n", m.Name, verb, m.Version)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
			case sync.SyncCompleteMsg:
				fmt.Printf("\ndone: %d installed, %d updated, %d current, %d failed\n",
					m.Installed, m.Updated, m.Skipped, m.Failed)
			}
		},
	}

	fmt.Printf("\nsyncing skills...\n\n")
	if err := syncer.Run(ctx, repo, st); err != nil {
		fmt.Fprintf(os.Stderr, "warning: sync failed for %s: %v\n", repo, err)
		fmt.Fprintf(os.Stderr, "run `scribe sync` to retry\n")
		if !isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("sync failed: %w", err)
		}
	}

	return nil
}

// resolveRepo returns the owner/repo string from args or an interactive prompt.
func resolveRepo(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("no repo specified — usage: scribe connect <owner/repo>")
	}

	var repo string
	err := huh.NewInput().
		Title("Team skills repo").
		Placeholder("owner/repo").
		Validate(func(s string) error {
			_, _, err := manifest.ParseOwnerRepo(s)
			return err
		}).
		Value(&repo).
		Run()
	if err != nil {
		return "", err
	}
	return repo, nil
}

