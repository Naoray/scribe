package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/prereq"
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

// runGuideInteractive runs the full interactive guide flow.
// Implemented in Task 6.
func runGuideInteractive() error {
	return fmt.Errorf("interactive guide not yet implemented")
}
