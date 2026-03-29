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
	syncCmd.Flags().StringVar(&registryFlag, "registry", "", "Sync only this registry (owner/repo or repo name)")
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

	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}

	// Migrate legacy state (no Registries field) on first multi-registry run.
	if len(cfg.TeamRepos) > 0 {
		st.MigrateRegistries(cfg.TeamRepos[0])
	}

	repos, err := filterRegistries(registryFlag, cfg.TeamRepos)
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	useJSON := syncJSON || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(cfg.TeamRepos) > 1

	resolved := map[string]sync.SkillStatus{}

	type skillResult struct {
		Name    string `json:"name"`
		Action  string `json:"action"`
		Status  string `json:"status,omitempty"`
		Version string `json:"version,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	// For JSON: collect per-registry results.
	type registryResult struct {
		Registry string        `json:"registry"`
		Skills   []skillResult `json:"skills"`
	}
	var jsonRegistries []registryResult
	totalSummary := sync.SyncCompleteMsg{}

	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
	}

	for _, teamRepo := range repos {
		clear(resolved)
		var jsonResults []skillResult

		syncer.Emit = func(msg any) {
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
				totalSummary.Installed += m.Installed
				totalSummary.Updated += m.Updated
				totalSummary.Skipped += m.Skipped
				totalSummary.Failed += m.Failed
			}
		}

		if !useJSON && multiRegistry {
			fmt.Fprintf(os.Stderr, "── %s ──\n", teamRepo)
		} else if !useJSON {
			fmt.Fprintf(os.Stderr, "syncing %s...\n\n", teamRepo)
		}

		if err := syncer.Run(context.Background(), teamRepo, st); err != nil {
			return err
		}

		// Track which registry each synced skill belongs to.
		for name := range resolved {
			st.AddRegistry(name, teamRepo)
		}
		_ = st.Save()

		if useJSON {
			jsonRegistries = append(jsonRegistries, registryResult{
				Registry: teamRepo,
				Skills:   jsonResults,
			})
		}

		if !useJSON && multiRegistry {
			fmt.Println()
		}
	}

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"registries": jsonRegistries,
			"summary": map[string]int{
				"installed": totalSummary.Installed,
				"updated":   totalSummary.Updated,
				"skipped":   totalSummary.Skipped,
				"failed":    totalSummary.Failed,
			},
		})
	}

	fmt.Printf("\ndone: %d installed, %d updated, %d current, %d failed\n",
		totalSummary.Installed, totalSummary.Updated, totalSummary.Skipped, totalSummary.Failed)

	return nil
}
