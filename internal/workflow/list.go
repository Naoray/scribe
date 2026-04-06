package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
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
		return listLocal(w, b.State, useJSON, b.ListTUI)
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

func listLocal(w io.Writer, st *state.State, useJSON bool, tuiFn func([]discovery.Skill) error) error {
	skills, err := discovery.OnDisk(st)
	if err != nil {
		return err
	}

	if useJSON {
		return printLocalJSON(w, skills, st)
	}
	if tuiFn != nil {
		return tuiFn(skills)
	}
	return printLocalTable(w, skills)
}

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
		maxName, maxVer := 4, 7
		for _, sk := range g.skills {
			maxName = max(maxName, runewidth.StringWidth(sk.Name))
			maxVer = max(maxVer, runewidth.StringWidth(sk.Version))
		}

		for _, sk := range g.skills {
			ver := sk.Version
			if ver == "" {
				ver = "—"
			}
			agents := listDimStyle.Render(strings.Join(sk.Targets, ", "))

			name := listNameStyle.Render(runewidth.FillRight(sk.Name, maxName))
			verStr := listDimStyle.Render(runewidth.FillRight(ver, maxVer))

			fmt.Fprintf(w, "  %s  %s  %s\n", name, verStr, agents)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, listTotalStyle.Render(fmt.Sprintf("%d skills total", len(skills))))
	return nil
}

func printLocalJSON(w io.Writer, skills []discovery.Skill, st *state.State) error {
	type localSkillJSON struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Version     string   `json:"version"`
		ContentHash string   `json:"content_hash,omitempty"`
		Source      string   `json:"source"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	out := make([]localSkillJSON, 0, len(skills))
	for _, sk := range skills {
		tgts := sk.Targets
		if tgts == nil {
			tgts = []string{}
		}

		_, managed := st.Installed[sk.Name]

		out = append(out, localSkillJSON{
			Name:        sk.Name,
			Description: sk.Description,
			Package:     sk.Package,
			Version:     sk.Version,
			ContentHash: sk.ContentHash,
			Source:      sk.Source,
			Targets:     tgts,
			Managed:     managed,
			Path:        sk.LocalPath,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func printMultiListTable(ctx context.Context, w io.Writer, repos []string, syncer *sync.Syncer, st *state.State, grouped bool) error {
	var allCounts []map[sync.Status]int

	for i, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		if i > 0 {
			fmt.Fprintln(w)
		}

		renderSkillTable(w, teamRepo, statuses)
		allCounts = append(allCounts, countStatuses(statuses))
	}

	// Summary footer.
	fmt.Fprintln(w)
	printStatusSummary(w, allCounts, repos)

	if !st.Team.LastSync.IsZero() {
		fmt.Fprintln(w, listDimStyle.Render("Last sync: "+st.Team.LastSync.Local().Format("2006-01-02 15:04")))
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
	maxName, maxVer, maxAuthor := 4, 7, 6
	for _, sk := range statuses {
		maxName = max(maxName, runewidth.StringWidth(sk.Name))
		maxVer = max(maxVer, runewidth.StringWidth(sk.DisplayVersion()))
		maxAuthor = max(maxAuthor, runewidth.StringWidth(sk.DisplayAuthor()))
	}

	// Rows.
	for _, sk := range statuses {
		name := listNameStyle.Render(runewidth.FillRight(sk.Name, maxName))
		ver := listDimStyle.Render(runewidth.FillRight(sk.DisplayVersion(), maxVer))
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


