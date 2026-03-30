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
	"github.com/Naoray/scribe/internal/targets"
	"github.com/Naoray/scribe/internal/workflow"
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

	return connectToRepo(repo, cfg, gh.NewClient(cfg.Token))
}

// connectToRepo performs the connect-and-sync workflow for a given "owner/repo" string.
func connectToRepo(repo string, cfg *config.Config, client *gh.Client) error {
	owner, name, err := parseOwnerRepo(repo)
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

	ctx := context.Background()
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

	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}
	fmtr := workflow.NewFormatter(false, false) // connect always uses text, single registry

	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				fmtr.OnSkillResolved(m.Name, m.SkillStatus)
			case sync.SkillSkippedMsg:
				fmtr.OnSkillSkipped(m.Name, sync.SkillStatus{})
			case sync.SkillDownloadingMsg:
				fmtr.OnSkillDownloading(m.Name)
			case sync.SkillInstalledMsg:
				fmtr.OnSkillInstalled(m.Name, m.Version, m.Updated)
			case sync.SkillErrorMsg:
				fmtr.OnSkillError(m.Name, m.Err)
			case sync.SyncCompleteMsg:
				fmtr.OnSyncComplete(m)
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
		return nil
	}

	return fmtr.Flush()
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
			_, _, err := parseOwnerRepo(s)
			return err
		}).
		Value(&repo).
		Run()
	if err != nil {
		return "", err
	}
	return repo, nil
}

// parseOwnerRepo validates and splits an "owner/repo" string.
func parseOwnerRepo(s string) (owner, repo string, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo (e.g. ArtistfyHQ/team-skills)", s)
	}
	return parts[0], parts[1], nil
}
