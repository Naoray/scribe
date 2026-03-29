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

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their status vs team loadout",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
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

	// For now: show first registry's skills (multi-registry list is a TODO).
	teamRepo := cfg.TeamRepos[0]

	client := gh.NewClient(cfg.Token)
	syncer := &sync.Syncer{Client: client, Targets: []targets.Target{}}

	statuses, err := syncer.Diff(context.Background(), teamRepo, st)
	if err != nil {
		return err
	}

	useJSON := listJSON || !isatty.IsTerminal(os.Stdout.Fd())

	if useJSON {
		return printListJSON(cfg.TeamRepos, statuses)
	}
	return printListTable(teamRepo, st, statuses)
}

func printListTable(teamRepo string, st *state.State, statuses []sync.SkillStatus) error {
	fmt.Printf("team: %s\n\n", teamRepo)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tVERSION\tSTATUS\tTARGETS")

	for _, sk := range statuses {
		ver := sk.LoadoutRef
		if ver == "" && sk.Installed != nil {
			ver = sk.Installed.DisplayVersion()
		}

		tgts := ""
		if sk.Installed != nil {
			tgts = strings.Join(sk.Installed.Targets, ", ")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sk.Name, ver, sk.Status.String(), tgts)
	}

	w.Flush()

	counts := countStatuses(statuses)
	fmt.Printf("\n%d current · %d outdated · %d missing · %d extra\n",
		counts[sync.StatusCurrent], counts[sync.StatusOutdated],
		counts[sync.StatusMissing], counts[sync.StatusExtra])

	if !st.Team.LastSync.IsZero() {
		fmt.Printf("Last sync: %s\n", st.Team.LastSync.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func printListJSON(teamRepos []string, statuses []sync.SkillStatus) error {
	type skillJSON struct {
		Name       string   `json:"name"`
		Status     string   `json:"status"`
		Version    string   `json:"version,omitempty"`
		LoadoutRef string   `json:"loadout_ref,omitempty"`
		Maintainer string   `json:"maintainer,omitempty"`
		Targets    []string `json:"targets,omitempty"`
	}

	skills := make([]skillJSON, 0, len(statuses))
	for _, sk := range statuses {
		ver := ""
		var tgts []string
		if sk.Installed != nil {
			ver = sk.Installed.DisplayVersion()
			tgts = sk.Installed.Targets
		}
		skills = append(skills, skillJSON{
			Name:       sk.Name,
			Status:     sk.Status.String(),
			Version:    ver,
			LoadoutRef: sk.LoadoutRef,
			Maintainer: sk.Maintainer,
			Targets:    tgts,
		})
	}

	counts := countStatuses(statuses)
	return writeJSON(os.Stdout, map[string]any{
		"team_repos": teamRepos,
		"skills":     skills,
		"summary": map[string]int{
			"current":  counts[sync.StatusCurrent],
			"outdated": counts[sync.StatusOutdated],
			"missing":  counts[sync.StatusMissing],
			"extra":    counts[sync.StatusExtra],
		},
	})
}

func countStatuses(statuses []sync.SkillStatus) map[sync.Status]int {
	m := map[sync.Status]int{}
	for _, sk := range statuses {
		m[sk.Status]++
	}
	return m
}
