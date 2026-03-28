package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

var syncJSON bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local skills to match team loadout",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output machine-readable JSON (for CI/agents)")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	teamRepo := cfg.TeamRepo
	if teamRepo == "" {
		teamRepo = st.Team.Repo
	}
	if teamRepo == "" {
		return fmt.Errorf("not initialized — run `scribe init <owner/repo>` first")
	}

	client := gh.NewClient(cfg.Token)
	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	useJSON := syncJSON || !isatty.IsTerminal(os.Stdout.Fd())

	// resolved holds the initial diff result per skill so we can look up
	// version info when a SkillSkippedMsg arrives (which only carries the name).
	resolved := map[string]sync.SkillStatus{}

	// For JSON output we collect a result per skill as events arrive,
	// then emit one document at the end.
	type skillResult struct {
		Name    string `json:"name"`
		Action  string `json:"action"` // skipped | installed | updated | error
		Status  string `json:"status,omitempty"`
		Version string `json:"version,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	var jsonResults []skillResult
	var jsonSummary sync.SyncCompleteMsg

	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus

			case sync.SkillSkippedMsg:
				sk := resolved[m.Name]
				ver := ""
				if sk.Installed != nil {
					ver = sk.Installed.DisplayVersion()
				}
				if useJSON {
					jsonResults = append(jsonResults, skillResult{
						Name:    m.Name,
						Action:  "skipped",
						Status:  sk.Status.String(),
						Version: ver,
					})
				} else {
					fmt.Printf("  %-20s ok (%s)\n", m.Name, ver)
				}

			case sync.SkillDownloadingMsg:
				if !useJSON {
					fmt.Printf("  %-20s downloading...\n", m.Name)
				}

			case sync.SkillInstalledMsg:
				if useJSON {
					action := "installed"
					if m.Updated {
						action = "updated"
					}
					jsonResults = append(jsonResults, skillResult{
						Name:    m.Name,
						Action:  action,
						Version: m.Version,
					})
				} else {
					verb := "installed"
					if m.Updated {
						verb = "updated to"
					}
					fmt.Printf("  %-20s %s %s\n", m.Name, verb, m.Version)
				}

			case sync.SkillErrorMsg:
				if useJSON {
					jsonResults = append(jsonResults, skillResult{
						Name:   m.Name,
						Action: "error",
						Error:  m.Err.Error(),
					})
				} else {
					fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
				}

			case sync.SyncCompleteMsg:
				jsonSummary = m
				if !useJSON {
					fmt.Printf("\ndone: %d installed, %d updated, %d current, %d failed\n",
						m.Installed, m.Updated, m.Skipped, m.Failed)
				}
			}
		},
	}

	if !useJSON {
		fmt.Fprintf(os.Stderr, "syncing %s...\n\n", teamRepo)
	}

	if err := syncer.Run(context.Background(), teamRepo, st); err != nil {
		return err
	}

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"team_repo": teamRepo,
			"skills":    jsonResults,
			"summary": map[string]int{
				"installed": jsonSummary.Installed,
				"updated":   jsonSummary.Updated,
				"skipped":   jsonSummary.Skipped,
				"failed":    jsonSummary.Failed,
			},
		})
	}
	return nil
}
