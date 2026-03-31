package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

// ListSteps returns the step list for the list command.
func ListSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"BranchLocalOrRemote", StepBranchLocalOrRemote},
	}
}

// StepBranchLocalOrRemote handles both the local-only view and the
// remote diff view, keeping the workflow linear.
func StepBranchLocalOrRemote(ctx context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	// Local view: explicit --local flag or no registries connected.
	if b.LocalFlag || len(b.Config.TeamRepos) == 0 {
		return listLocal(w, b.State, useJSON)
	}

	// Reuse shared steps for migration and filtering.
	if err := StepMigrateRegistries(ctx, b); err != nil {
		return err
	}
	if err := StepFilterRegistries(ctx, b); err != nil {
		return err
	}

	syncer := &sync.Syncer{Client: sync.WrapGitHubClient(b.Client), Targets: []targets.Target{}}
	multiRegistry := len(b.Repos) > 1

	if useJSON {
		return printMultiListJSON(ctx, w, b.Repos, syncer, b.State)
	}
	return printMultiListTable(ctx, w, b.Repos, syncer, b.State, multiRegistry)
}

func listLocal(w io.Writer, st *state.State, useJSON bool) error {
	if useJSON {
		return printLocalJSON(w, st)
	}
	return printLocalTable(w, st)
}

func printLocalTable(w io.Writer, st *state.State) error {
	if len(st.Installed) == 0 {
		fmt.Fprintln(w, "No skills installed.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Install skills from a registry:  scribe connect <owner/repo>")
		return nil
	}

	names := sortedSkillNames(st.Installed)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SKILL\tVERSION\tTARGETS\tSOURCE")

	for _, name := range names {
		skill := st.Installed[name]
		source, _, _ := strings.Cut(skill.Source, "@")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			name,
			skill.DisplayVersion(),
			strings.Join(skill.Targets, ", "),
			source,
		)
	}

	return tw.Flush()
}

func printLocalJSON(w io.Writer, st *state.State) error {
	type localSkillJSON struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Source      string   `json:"source"`
		Targets     []string `json:"targets"`
		Registries  []string `json:"registries,omitempty"`
	}

	names := sortedSkillNames(st.Installed)
	skills := make([]localSkillJSON, 0, len(names))
	for _, name := range names {
		sk := st.Installed[name]
		tgts := sk.Targets
		if tgts == nil {
			tgts = []string{}
		}
		skills = append(skills, localSkillJSON{
			Name:       name,
			Version:    sk.DisplayVersion(),
			Source:     sk.Source,
			Targets:    tgts,
			Registries: sk.Registries,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(skills)
}

func printMultiListTable(ctx context.Context, w io.Writer, repos []string, syncer *sync.Syncer, st *state.State, grouped bool) error {
	var footerParts []string

	for i, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		if grouped {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "── %s ──\n", teamRepo)
		} else {
			fmt.Fprintf(w, "team: %s\n\n", teamRepo)
		}

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SKILL\tVERSION\tSTATUS\tAGENTS")

		for _, sk := range statuses {
			ver := sk.LoadoutRef
			if ver == "" && sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
			}

			agents := ""
			if sk.Installed != nil {
				agents = strings.Join(sk.Installed.Targets, ", ")
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sk.Name, ver, sk.Status.String(), agents)
		}

		tw.Flush()

		counts := countStatuses(statuses)
		if grouped {
			parts := formatCounts(counts)
			if parts != "" {
				footerParts = append(footerParts, fmt.Sprintf("%s: %s", teamRepo, parts))
			}
		} else {
			fmt.Fprintf(w, "\n%d current · %d outdated · %d missing · %d extra\n",
				counts[sync.StatusCurrent], counts[sync.StatusOutdated],
				counts[sync.StatusMissing], counts[sync.StatusExtra])
		}
	}

	if grouped && len(footerParts) > 0 {
		fmt.Fprintf(w, "\n%s\n", strings.Join(footerParts, "  ·  "))
	}

	if !st.Team.LastSync.IsZero() {
		fmt.Fprintf(w, "Last sync: %s\n", st.Team.LastSync.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func printMultiListJSON(ctx context.Context, w io.Writer, repos []string, syncer *sync.Syncer, st *state.State) error {
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

	return json.NewEncoder(w).Encode(map[string]any{
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

func sortedSkillNames(installed map[string]state.InstalledSkill) []string {
	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
