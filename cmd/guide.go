package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	clienv "github.com/Naoray/scribe/internal/cli/env"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/prereq"
	"github.com/Naoray/scribe/internal/workflow"
)

func newGuideCommand() *cobra.Command {
	cmd := &cobra.Command{
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
	return markJSONSupported(cmd)
}

// Styles for guide output — kept local to cmd/ per architecture.
var (
	guideTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Padding(0, 1)
	guideCheckOKStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	guideCheckFailStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	guideCheckPendStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3"))
	guideSubtleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3"))
	guideBoldStyle       = lipgloss.NewStyle().Bold(true)
	guideSummaryBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).BorderForeground(lipgloss.Color("#7C3AED"))
)

func runGuide(cmd *cobra.Command, _ []string) error {
	jsonFlag := jsonFlagPassed(cmd)
	mode := clienv.Detect(os.Stdout, os.Stdin, jsonFlag)
	if mode.Format == clienv.FormatJSON {
		r := jsonRendererForCommand(cmd, jsonFlag)
		if err := r.Result(buildGuideOutput()); err != nil {
			return err
		}
		return r.Flush()
	}

	if !mode.Interactive {
		err := fmt.Errorf("scribe guide requires an interactive terminal")
		return clierrors.Wrap(err, "INTERACTIVE_TERMINAL_REQUIRED", clierrors.ExitUsage,
			clierrors.WithRemediation("Use `scribe guide --json` for agent-friendly output."),
		)
	}

	return runGuideInteractive(cmd)
}

// guideStep is a single actionable step in the JSON output.
type guideStep struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// runGuideJSON writes the guide steps as JSON to w.
func runGuideJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildGuideOutput())
}

type guideOutput struct {
	Status        string        `json:"status"`
	Prerequisites prereq.Result `json:"prerequisites"`
	Steps         []guideStep   `json:"steps"`
}

func buildGuideOutput() guideOutput {
	result := prereq.Check()

	status := "not_connected"
	if len(result.Connections.Repos) > 0 {
		status = "connected"
	}

	var steps []guideStep

	if !result.GitHubAuth.OK {
		steps = append(steps, guideStep{
			Command:     "gh auth login",
			Description: "Authenticate with GitHub",
		})
	}

	if len(result.Connections.Repos) == 0 {
		steps = append(steps, guideStep{
			Command:     "scribe connect <owner/repo>",
			Description: "Connect to your team's skill registry",
		})
	}

	steps = append(steps, guideStep{
		Command:     "scribe sync",
		Description: "Sync skills to your local machine",
	})

	steps = append(steps, guideStep{
		Command:     "scribe list",
		Description: "Verify installed skills",
	})

	steps = append(steps, guideStep{
		Command:     "scribe adopt",
		Description: "Import hand-rolled skills from ~/.claude/skills into the store",
	})

	return guideOutput{
		Status:        status,
		Prerequisites: result,
		Steps:         steps,
	}
}

// displayPrereqs shows prereq status with styled icons.
func displayPrereqs(result prereq.Result) {
	fmt.Println()
	fmt.Println(guideTitleStyle.Render("Scribe Guide"))
	fmt.Println()

	if result.GitHubAuth.OK {
		fmt.Printf("  %s GitHub authenticated (%s)\n", guideCheckOKStyle.Render("✓"), result.GitHubAuth.Method)
	} else {
		fmt.Printf("  %s GitHub not authenticated\n", guideCheckFailStyle.Render("✗"))
	}

	if result.ScribeDir.OK {
		fmt.Printf("  %s Scribe directory exists\n", guideCheckOKStyle.Render("✓"))
	} else {
		fmt.Printf("  %s Scribe directory will be created\n", guideCheckPendStyle.Render("○"))
	}

	if n := len(result.Connections.Repos); n > 0 {
		suffix := "y"
		if n != 1 {
			suffix = "ies"
		}
		fmt.Printf("  %s Connected to %d registr%s\n", guideCheckOKStyle.Render("✓"), n, suffix)
	} else {
		fmt.Printf("  %s No team registries connected\n", guideCheckPendStyle.Render("○"))
	}

	fmt.Println()
}

// waitForAuth loops until the user authenticates with GitHub.
func waitForAuth() error {
	for {
		fmt.Println(guideSubtleStyle.Render("  To authenticate, run one of:"))
		fmt.Println(guideSubtleStyle.Render("    • gh auth login"))
		fmt.Println(guideSubtleStyle.Render("    • export GITHUB_TOKEN=<your-token>"))
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
			fmt.Printf("  %s GitHub authenticated (%s)\n\n", guideCheckOKStyle.Render("✓"), result.GitHubAuth.Method)
			return nil
		}
		fmt.Printf("  %s Still not authenticated\n\n", guideCheckFailStyle.Render("✗"))
	}
}

// displayGuideSummary renders the final summary box with next steps.
func displayGuideSummary(repo, chosenFlow string) {
	var content string

	content += guideBoldStyle.Render("All set!") + "\n\n"
	content += fmt.Sprintf("  Registry    %s\n", repo)
	content += "\n"
	content += guideBoldStyle.Render("  What's next:") + "\n"

	switch chosenFlow {
	case "join":
		content += "  • scribe sync       Keep skills up to date\n"
		content += "  • scribe list       See installed skills and status\n"
		content += "  • scribe adopt      Import hand-rolled skills into the store\n"
	case "create":
		content += "  • scribe add        Add skills to your registry\n"
		content += "  • scribe list       See installed skills and status\n"
		content += "  • scribe adopt      Import hand-rolled skills into the store\n"
	}

	content += "  • scribe guide      Run this guide again anytime\n"

	fmt.Println(guideSummaryBoxStyle.Render(content))
}

// runGuideInteractive runs the full interactive guide flow.
func runGuideInteractive(cmd *cobra.Command) error {
	result := prereq.Check()
	displayPrereqs(result)

	// Auth gate — loop until authenticated.
	if !result.GitHubAuth.OK {
		if err := waitForAuth(); err != nil {
			return err
		}
	}

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

		// Use the connect workflow which handles dedup, manifest validation,
		// config save, and auto-sync.
		bag := &workflow.Bag{
			RepoArg: repo,
		}
		if err := workflow.Run(cmd.Context(), workflow.ConnectSteps(), bag); err != nil {
			return err
		}
		if err := saveWorkflowState(bag); err != nil {
			return err
		}

		displayGuideSummary(repo, "join")

	case "create":
		createCmd := newCreateRegistryCommand()
		createCmd.SetContext(cmd.Context())
		if err := runCreateRegistry(createCmd, nil); err != nil {
			return err
		}
		// Show summary with the last connected repo.
		result := prereq.Check()
		if len(result.Connections.Repos) > 0 {
			repo := result.Connections.Repos[len(result.Connections.Repos)-1]
			displayGuideSummary(repo, "create")
		}

	case "view":
		listCmd := newListCommand()
		listCmd.SetContext(cmd.Context())
		return runList(listCmd, nil)
	}

	return nil
}
