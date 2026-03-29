package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	RunE:  runList,
}

func init() {
	listCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	listCmd.Flags().String("registry", "", "Show only this registry (owner/repo or repo name)")
	listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
	listCmd.Flags().MarkHidden("all")
}

func runList(cmd *cobra.Command, args []string) error {
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

	// Migrate legacy state (no Registries field) for users who haven't synced yet.
	st.MigrateRegistries(cfg.TeamRepos[0])

	registry, _ := cmd.Flags().GetString("registry")
	repos, err := filterRegistries(registry, cfg.TeamRepos)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	client := gh.NewClient(ctx, cfg.Token)
	syncer := &sync.Syncer{Client: sync.WrapGitHubClient(client), Targets: []targets.Target{}}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(repos) > 1

	if useJSON {
		return printMultiListJSON(ctx, repos, syncer, st)
	}
	return printMultiListTable(ctx, repos, syncer, st, multiRegistry)
}

func printMultiListTable(ctx context.Context, repos []string, syncer *sync.Syncer, st *state.State, grouped bool) error {
	var footerParts []string

	for i, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		if grouped {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("── %s ──\n", teamRepo)
		} else {
			fmt.Printf("team: %s\n\n", teamRepo)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SKILL\tVERSION\tSTATUS\tAGENTS")

		for _, sk := range statuses {
			ver := sk.LoadoutRef
			if ver == "" && sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
			}

			agents := ""
			if sk.Installed != nil {
				agents = strings.Join(sk.Installed.Targets, ", ")
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sk.Name, ver, sk.Status.String(), agents)
		}

		w.Flush()

		counts := countStatuses(statuses)
		if grouped {
			parts := formatCounts(counts)
			if parts != "" {
				footerParts = append(footerParts, fmt.Sprintf("%s: %s", teamRepo, parts))
			}
		} else {
			fmt.Printf("\n%d current · %d outdated · %d missing · %d extra\n",
				counts[sync.StatusCurrent], counts[sync.StatusOutdated],
				counts[sync.StatusMissing], counts[sync.StatusExtra])
		}
	}

	if grouped && len(footerParts) > 0 {
		fmt.Printf("\n%s\n", strings.Join(footerParts, "  ·  "))
	}

	if !st.Team.LastSync.IsZero() {
		fmt.Printf("Last sync: %s\n", st.Team.LastSync.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

// formatCounts builds a compact count string like "2 current · 1 outdated".
func formatCounts(counts map[sync.Status]int) string {
	var parts []string
	if n := counts[sync.StatusCurrent]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d current", n))
	}
	if n := counts[sync.StatusOutdated]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d outdated", n))
	}
	if n := counts[sync.StatusMissing]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", n))
	}
	if n := counts[sync.StatusExtra]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d extra", n))
	}
	return strings.Join(parts, " · ")
}

func printMultiListJSON(ctx context.Context, repos []string, syncer *sync.Syncer, st *state.State) error {
	type skillJSON struct {
		Name       string   `json:"name"`
		Status     string   `json:"status"`
		Version    string   `json:"version,omitempty"`
		LoadoutRef string   `json:"loadout_ref,omitempty"`
		Maintainer string   `json:"maintainer,omitempty"`
		Agents     []string `json:"agents,omitempty"`
	}

	type registryJSON struct {
		Registry string      `json:"registry"`
		Skills   []skillJSON `json:"skills"`
	}

	var registries []registryJSON

	for _, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		skills := make([]skillJSON, 0, len(statuses))
		for _, sk := range statuses {
			ver := ""
			var agents []string
			if sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
				agents = sk.Installed.Targets
			}
			skills = append(skills, skillJSON{
				Name:       sk.Name,
				Status:     sk.Status.String(),
				Version:    ver,
				LoadoutRef: sk.LoadoutRef,
				Maintainer: sk.Maintainer,
				Agents:     agents,
			})
		}

		registries = append(registries, registryJSON{
			Registry: teamRepo,
			Skills:   skills,
		})
	}

	return writeJSON(os.Stdout, map[string]any{
		"registries": registries,
	})
}

func countStatuses(statuses []sync.SkillStatus) map[sync.Status]int {
	m := map[sync.Status]int{}
	for _, sk := range statuses {
		m[sk.Status]++
	}
	return m
}
