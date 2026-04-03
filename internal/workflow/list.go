package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/discovery"
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
	skills, err := discovery.OnDisk(st)
	if err != nil {
		return err
	}

	if useJSON {
		return printLocalJSON(w, skills)
	}
	return printLocalTable(w, skills)
}

// list styles — scoped to avoid polluting the package namespace.
var (
	listHeaderStyle = lipgloss.NewStyle().Bold(true)
	listCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listNameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	listDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	listTotalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

func printLocalTable(w io.Writer, skills []discovery.Skill) error {
	if len(skills) == 0 {
		fmt.Fprintln(w, "No skills installed.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Install skills from a registry:  scribe connect <owner/repo>")
		return nil
	}

	// Bucket skills by group for clean rendering.
	type group struct {
		name   string
		skills []discovery.Skill
	}
	var groups []group
	groupIdx := map[string]int{}

	for _, sk := range skills {
		pkg := sk.Package
		if idx, ok := groupIdx[pkg]; ok {
			groups[idx].skills = append(groups[idx].skills, sk)
		} else {
			groupIdx[pkg] = len(groups)
			groups = append(groups, group{name: pkg, skills: []discovery.Skill{sk}})
		}
	}

	// Check if any skill has version info — if so, use detailed table.
	hasVersions := false
	for _, sk := range skills {
		if sk.Version != "" {
			hasVersions = true
			break
		}
	}

	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}

		// Group header with divider line.
		label := g.name
		if label == "" {
			label = "standalone"
		}
		count := listCountStyle.Render(fmt.Sprintf("(%d)", len(g.skills)))
		header := fmt.Sprintf("%s %s", listHeaderStyle.Render(label), count)
		fmt.Fprintln(w, header)
		fmt.Fprintln(w, listDivStyle.Render(strings.Repeat("─", len(label)+5)))

		if hasVersions {
			// Detailed table when managed skills exist.
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, sk := range g.skills {
				ver := sk.Version
				if ver == "" {
					ver = "-"
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\n",
					sk.Name, ver, strings.Join(sk.Targets, ", "))
			}
			tw.Flush()
		} else {
			// Compact name list when everything is unmanaged.
			printCompactNames(w, g.skills)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, listTotalStyle.Render(fmt.Sprintf("%d skills total", len(skills))))
	return nil
}

// printCompactNames renders skill names in a single column with dot leaders for scanability.
func printCompactNames(w io.Writer, skills []discovery.Skill) {
	for i, sk := range skills {
		num := listCountStyle.Render(fmt.Sprintf("%3d", i+1))
		name := listNameStyle.Render(sk.Name)
		fmt.Fprintf(w, "  %s  %s\n", num, name)
	}
}

func printLocalJSON(w io.Writer, skills []discovery.Skill) error {
	type localSkillJSON struct {
		Name    string   `json:"name"`
		Package string   `json:"package,omitempty"`
		Version string   `json:"version"`
		Source  string   `json:"source"`
		Targets []string `json:"targets"`
		Managed bool     `json:"managed"`
		Path    string   `json:"path,omitempty"`
	}

	out := make([]localSkillJSON, 0, len(skills))
	for _, sk := range skills {
		tgts := sk.Targets
		if tgts == nil {
			tgts = []string{}
		}
		out = append(out, localSkillJSON{
			Name:    sk.Name,
			Package: sk.Package,
			Version: sk.Version,
			Source:  sk.Source,
			Targets: tgts,
			Managed: sk.Managed,
			Path:    sk.LocalPath,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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

