package cmd

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/workflow"
)

// list styles — scoped to avoid polluting the package namespace.
var (
	listHeaderStyle = lipgloss.NewStyle().Bold(true)
	listCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listNameStyle   = lipgloss.NewStyle().Bold(true)
	listDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	listDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	listTotalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listAuthorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF"))
	listRegStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))

	// statusStyles maps each sync status to its lipgloss style.
	statusStyles = map[sync.Status]lipgloss.Style{
		sync.StatusCurrent:  lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")),
		sync.StatusOutdated: lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")),
		sync.StatusMissing:  lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")),
		sync.StatusExtra:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3")),
	}
)

// renderStatus returns a styled "icon label" string for a sync status, padded to consistent width.
func renderStatus(s sync.Status) string {
	d := s.Display()
	raw := d.Icon + " " + d.Label
	return statusStyles[s].Render(runewidth.FillRight(raw, 10))
}

// renderStatusCount returns a styled "N label" string, or "" if count is zero.
func renderStatusCount(s sync.Status, n int) string {
	if n == 0 {
		return ""
	}
	return statusStyles[s].Render(fmt.Sprintf("%d %s", n, s.Display().Label))
}

func printLocalTable(w io.Writer, skills []discovery.Skill) error {
	if len(skills) == 0 {
		fmt.Fprintln(w, "No skills installed.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Install skills from a registry:  scribe connect <owner/repo>")
		return nil
	}

	// Bucket skills by group.
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

	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}

		label := g.name
		if label == "" {
			label = "standalone"
		}
		count := listCountStyle.Render(fmt.Sprintf("(%d skills)", len(g.skills)))
		fmt.Fprintf(w, "%s %s\n", listRegStyle.Render(label), count)
		fmt.Fprintln(w, listDivStyle.Render(strings.Repeat("─", len(label)+15)))

		// Compute column widths.
		maxName, maxVersionWidth := 4, 7
		for _, sk := range g.skills {
			maxName = max(maxName, runewidth.StringWidth(sk.Name))
			maxVersionWidth = max(maxVersionWidth, runewidth.StringWidth(sk.Version))
		}

		for _, sk := range g.skills {
			ver := sk.Version
			if ver == "" {
				ver = "—"
			}
			agents := listDimStyle.Render(strings.Join(sk.Targets, ", "))

			name := listNameStyle.Render(runewidth.FillRight(sk.Name, maxName))
			verStr := listDimStyle.Render(runewidth.FillRight(ver, maxVersionWidth))

			fmt.Fprintf(w, "  %s  %s  %s\n", name, verStr, agents)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, listTotalStyle.Render(fmt.Sprintf("%d skills total", len(skills))))
	return nil
}

func printMultiListTable(w io.Writer, repos []string, diffs map[string][]sync.SkillStatus, st *state.State, grouped bool) error {
	var allCounts []map[sync.Status]int

	for i, teamRepo := range repos {
		statuses := diffs[teamRepo]

		if i > 0 {
			fmt.Fprintln(w)
		}

		renderSkillTable(w, teamRepo, statuses)
		allCounts = append(allCounts, workflow.CountStatuses(statuses))
	}

	// Summary footer.
	fmt.Fprintln(w)
	printStatusSummary(w, allCounts, repos)

	if !st.LastSync.IsZero() {
		fmt.Fprintln(w, listDimStyle.Render("Last sync: "+st.LastSync.Local().Format("2006-01-02 15:04")))
	}
	return nil
}

// renderSkillTable renders a single registry's skills as a styled table.
func renderSkillTable(w io.Writer, repo string, statuses []sync.SkillStatus) {
	// Header.
	count := listCountStyle.Render(fmt.Sprintf("(%d skills)", len(statuses)))
	fmt.Fprintf(w, "%s %s\n", listRegStyle.Render(repo), count)
	fmt.Fprintln(w, listDivStyle.Render(strings.Repeat("─", len(repo)+15)))

	// Compute column widths in one pass.
	maxName, maxVersionWidth, maxAuthor := 4, 7, 6
	for _, sk := range statuses {
		maxName = max(maxName, runewidth.StringWidth(sk.Name))
		maxVersionWidth = max(maxVersionWidth, runewidth.StringWidth(sk.DisplayVersion()))
		maxAuthor = max(maxAuthor, runewidth.StringWidth(sk.DisplayAuthor()))
	}

	// Rows.
	for _, sk := range statuses {
		name := listNameStyle.Render(runewidth.FillRight(sk.Name, maxName))
		ver := listDimStyle.Render(runewidth.FillRight(sk.DisplayVersion(), maxVersionWidth))
		author := listAuthorStyle.Render(runewidth.FillRight(sk.DisplayAuthor(), maxAuthor))
		status := renderStatus(sk.Status)
		agents := listDimStyle.Render(sk.DisplayAgents())

		fmt.Fprintf(w, "  %s  %s  %s  %s  %s\n", name, ver, author, status, agents)
	}
}

// printStatusSummary renders the colored status count footer.
func printStatusSummary(w io.Writer, allCounts []map[sync.Status]int, repos []string) {
	// Merge all counts.
	merged := map[sync.Status]int{}
	for _, c := range allCounts {
		for s, n := range c {
			merged[s] += n
		}
	}

	order := []sync.Status{sync.StatusCurrent, sync.StatusOutdated, sync.StatusMissing, sync.StatusExtra}
	var parts []string
	for _, s := range order {
		if part := renderStatusCount(s, merged[s]); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 {
		fmt.Fprintln(w, strings.Join(parts, listDimStyle.Render(" · ")))
	}
}
