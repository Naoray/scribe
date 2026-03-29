package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/prereq"
	"github.com/Naoray/scribe/internal/state"
	syncsvc "github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
	"github.com/Naoray/scribe/internal/ui"
)

var guideJSON bool

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Interactive setup guide for Scribe",
	Long: `Walk through Scribe setup step by step.

Run with --json or pipe to get machine-readable steps for agents.

Examples:
  scribe guide          # interactive setup
  scribe guide --json   # agent-friendly step list`,
	Args: cobra.NoArgs,
	RunE: runGuide,
}

func init() {
	guideCmd.Flags().BoolVar(&guideJSON, "json", false, "Output machine-readable JSON (for CI/agents)")
}

func runGuide(cmd *cobra.Command, args []string) error {
	useJSON := guideJSON || !isatty.IsTerminal(os.Stdout.Fd())
	if useJSON {
		return runGuideJSON(os.Stdout)
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("scribe guide requires an interactive terminal — use --json for agent-friendly output")
	}

	return runGuideInteractive()
}

// runGuideJSON writes the guide steps as JSON to w.
func runGuideJSON(w io.Writer) error {
	result := prereq.Check()

	status := "not_connected"
	if len(result.Connections.Repos) > 0 {
		status = "connected"
	}

	type step struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}

	var steps []step

	if !result.GitHubAuth.OK {
		steps = append(steps, step{
			Command:     "gh auth login",
			Description: "Authenticate with GitHub",
		})
	}

	if len(result.Connections.Repos) == 0 {
		steps = append(steps, step{
			Command:     "scribe connect <owner/repo>",
			Description: "Connect to your team's skill registry",
		})
	}

	steps = append(steps, step{
		Command:     "scribe sync",
		Description: "Sync skills to your local machine",
	})

	steps = append(steps, step{
		Command:     "scribe list",
		Description: "Verify installed skills",
	})

	return json.NewEncoder(w).Encode(map[string]any{
		"status":        status,
		"prerequisites": result,
		"steps":         steps,
	})
}

// displayPrereqs shows prereq status with styled icons.
func displayPrereqs(result prereq.Result) {
	fmt.Println()
	fmt.Println(ui.Title.Render("Scribe Guide"))
	fmt.Println()

	if result.GitHubAuth.OK {
		fmt.Printf("  %s GitHub authenticated (%s)\n", ui.CheckOK.Render("✓"), result.GitHubAuth.Method)
	} else {
		fmt.Printf("  %s GitHub not authenticated\n", ui.CheckFail.Render("✗"))
	}

	if result.ScribeDir.OK {
		fmt.Printf("  %s Scribe directory exists\n", ui.CheckOK.Render("✓"))
	} else {
		fmt.Printf("  %s Scribe directory will be created\n", ui.CheckPending.Render("○"))
	}

	if len(result.Connections.Repos) > 0 {
		fmt.Printf("  %s Connected to %d registry\n", ui.CheckOK.Render("✓"), len(result.Connections.Repos))
	} else {
		fmt.Printf("  %s No team registries connected\n", ui.CheckPending.Render("○"))
	}

	fmt.Println()
}

// waitForAuth loops until the user authenticates with GitHub.
func waitForAuth() error {
	for {
		fmt.Println(ui.Subtle.Render("  To authenticate, run one of:"))
		fmt.Println(ui.Subtle.Render("    • gh auth login"))
		fmt.Println(ui.Subtle.Render("    • export GITHUB_TOKEN=<your-token>"))
		fmt.Println()

		var retry bool
		if err := huh.NewConfirm().Title("Ready to re-check?").Value(&retry).Run(); err != nil {
			return err
		}
		if !retry {
			return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
		}

		result := prereq.Check()
		if result.GitHubAuth.OK {
			fmt.Printf("  %s GitHub authenticated (%s)\n\n", ui.CheckOK.Render("✓"), result.GitHubAuth.Method)
			return nil
		}
		fmt.Printf("  %s Still not authenticated\n\n", ui.CheckFail.Render("✗"))
	}
}

// connectOnly connects to a repo without syncing (sync is handled separately by the Bubble Tea model).
func connectOnly(repo string, cfg *config.Config, client *gh.Client) error {
	owner, name, err := parseOwnerRepo(repo)
	if err != nil {
		return err
	}

	for _, existing := range cfg.TeamRepos {
		if strings.EqualFold(existing, repo) {
			fmt.Printf("  Already connected to %s\n", existing)
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
	fmt.Printf("  Connected to %s\n\n", repo)
	return nil
}

// runSyncWithProgress runs sync with a Bubble Tea progress display.
func runSyncWithProgress(repo string, cfg *config.Config, client *gh.Client) (syncsvc.SyncCompleteMsg, error) {
	st, err := state.Load()
	if err != nil {
		return syncsvc.SyncCompleteMsg{}, err
	}

	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	model := ui.NewSyncProgress(repo)
	p := tea.NewProgram(model)

	syncer := &syncsvc.Syncer{
		Client:  client,
		Targets: tgts,
		Emit:    func(msg any) { p.Send(msg) },
	}

	// Run sync in background, sending events to the Bubble Tea program.
	go func() {
		if err := syncer.Run(context.Background(), repo, st); err != nil {
			p.Send(syncsvc.SkillErrorMsg{Name: "sync", Err: err})
			p.Send(syncsvc.SyncCompleteMsg{Failed: 1})
		}
	}()

	finalModel, err := p.Run()
	if err != nil {
		return syncsvc.SyncCompleteMsg{}, fmt.Errorf("TUI error: %w", err)
	}

	if fm, ok := finalModel.(ui.SyncProgress); ok {
		return fm.Summary, nil
	}
	return syncsvc.SyncCompleteMsg{}, nil
}

// displaySummary renders the final summary box with next steps.
func displaySummary(repo string, summary syncsvc.SyncCompleteMsg, path string) {
	total := summary.Installed + summary.Updated + summary.Skipped
	var content strings.Builder

	content.WriteString(ui.Bold.Render("All set!"))
	content.WriteString("\n\n")
	content.WriteString(fmt.Sprintf("  Registry    %s\n", repo))
	content.WriteString(fmt.Sprintf("  Skills      %d installed, %d current, %d failed\n", summary.Installed+summary.Updated, summary.Skipped, summary.Failed))
	content.WriteString(fmt.Sprintf("  Targets     claude, cursor\n"))
	content.WriteString("\n")
	content.WriteString(ui.Bold.Render("  What's next:"))
	content.WriteString("\n")

	switch path {
	case "join":
		content.WriteString("  • scribe sync       Keep skills up to date\n")
		content.WriteString("  • scribe list       See installed skills and status\n")
	case "create":
		content.WriteString("  • scribe add        Add skills to your registry\n")
		content.WriteString("  • scribe list       See installed skills and status\n")
	}

	_ = total // suppress unused warning
	content.WriteString("  • scribe guide      Run this guide again anytime\n")

	fmt.Println(ui.Summary.Render(content.String()))
}

// runGuideInteractive runs the full interactive guide flow.
func runGuideInteractive() error {
	result := prereq.Check()
	displayPrereqs(result)

	// Auth gate — loop until authenticated.
	if !result.GitHubAuth.OK {
		if err := waitForAuth(); err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := gh.NewClient(cfg.Token)

	// Build path options based on current state.
	options := []huh.Option[string]{
		huh.NewOption("Join an existing team", "join"),
		huh.NewOption("Create a new skill registry", "create"),
	}
	if len(result.Connections.Repos) > 0 {
		options = append(options, huh.NewOption("View my current setup", "view"))
	}

	var chosen string
	if err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(options...).
		Value(&chosen).
		Run(); err != nil {
		return err
	}

	switch chosen {
	case "join":
		repo, err := resolveRepo(nil)
		if err != nil {
			return err
		}
		if err := connectOnly(repo, cfg, client); err != nil {
			return err
		}
		summary, err := runSyncWithProgress(repo, cfg, client)
		if err != nil {
			return err
		}
		displaySummary(repo, summary, "join")

	case "create":
		if err := runCreateRegistry(createRegistryCmd, nil); err != nil {
			return err
		}
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		if len(cfg.TeamRepos) > 0 {
			repo := cfg.TeamRepos[len(cfg.TeamRepos)-1]
			displaySummary(repo, syncsvc.SyncCompleteMsg{}, "create")
		}

	case "view":
		return runList(listCmd, nil)
	}

	return nil
}
