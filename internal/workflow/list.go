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
	listHeaderStyle  = lipgloss.NewStyle().Bold(true)
	listCountStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listNameStyle    = lipgloss.NewStyle().Bold(true)
	listDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	listDivStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	listTotalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listCurrentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	listOutdatedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
	listMissingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	listExtraStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3"))
	listAuthorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF"))
	listRegStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
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
			printSkillList(w, g.skills)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, listTotalStyle.Render(fmt.Sprintf("%d skills total", len(skills))))
	return nil
}

// printSkillList renders skills with name and description on two lines.
func printSkillList(w io.Writer, skills []discovery.Skill) {
	for _, sk := range skills {
		name := listNameStyle.Render(sk.Name)
		fmt.Fprintf(w, "  %s\n", name)
		if sk.Description != "" {
			fmt.Fprintf(w, "  %s\n", listDescStyle.Render(sk.Description))
		}
	}
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
	var footerParts []string

	for i, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		if i > 0 {
			fmt.Fprintln(w)
		}

		// Registry header.
		count := listCountStyle.Render(fmt.Sprintf("(%d skills)", len(statuses)))
		fmt.Fprintf(w, "%s %s\n", listRegStyle.Render(teamRepo), count)
		fmt.Fprintln(w, listDivStyle.Render(strings.Repeat("─", len(teamRepo)+15)))

		// Calculate column widths.
		maxName, maxVer, maxAuthor := 0, 0, 0
		for _, sk := range statuses {
			if w := len(sk.Name); w > maxName {
				maxName = w
			}
			ver := sk.LoadoutRef
			if ver == "" && sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
			}
			if w := len(ver); w > maxVer {
				maxVer = w
			}
			if w := len(sk.Maintainer); w > maxAuthor {
				maxAuthor = w
			}
		}
		if maxName < 4 {
			maxName = 4
		}
		if maxVer < 7 {
			maxVer = 7
		}
		if maxAuthor < 6 {
			maxAuthor = 6
		}

		for _, sk := range statuses {
			ver := sk.LoadoutRef
			if ver == "" && sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
			}

			// Status indicator with color.
			var statusStr string
			switch sk.Status {
			case sync.StatusCurrent:
				statusStr = listCurrentStyle.Render("✓ current")
			case sync.StatusOutdated:
				statusStr = listOutdatedStyle.Render("↑ update")
			case sync.StatusMissing:
				statusStr = listMissingStyle.Render("○ missing")
			case sync.StatusExtra:
				statusStr = listExtraStyle.Render("? extra")
			}

			// Author.
			author := sk.Maintainer
			if author == "" {
				author = "—"
			}

			// Agents.
			agents := ""
			if sk.Installed != nil && len(sk.Installed.Targets) > 0 {
				agents = listDimStyle.Render(strings.Join(sk.Installed.Targets, ", "))
			}

			name := listNameStyle.Render(fmt.Sprintf("%-*s", maxName, sk.Name))
			verStr := listDimStyle.Render(fmt.Sprintf("%-*s", maxVer, ver))
			authorStr := listAuthorStyle.Render(fmt.Sprintf("%-*s", maxAuthor, author))

			fmt.Fprintf(w, "  %s  %s  %s  %-12s  %s\n", name, verStr, authorStr, statusStr, agents)
		}

		counts := countStatuses(statuses)
		if grouped {
			parts := formatCounts(counts)
			if parts != "" {
				footerParts = append(footerParts, fmt.Sprintf("%s: %s", teamRepo, parts))
			}
		}
	}

	// Summary footer.
	fmt.Fprintln(w)
	if len(repos) == 1 {
		statuses, _, err := syncer.Diff(ctx, repos[0], st)
		if err == nil {
			counts := countStatuses(statuses)
			var parts []string
			if n := counts[sync.StatusCurrent]; n > 0 {
				parts = append(parts, listCurrentStyle.Render(fmt.Sprintf("%d current", n)))
			}
			if n := counts[sync.StatusOutdated]; n > 0 {
				parts = append(parts, listOutdatedStyle.Render(fmt.Sprintf("%d outdated", n)))
			}
			if n := counts[sync.StatusMissing]; n > 0 {
				parts = append(parts, listMissingStyle.Render(fmt.Sprintf("%d missing", n)))
			}
			if n := counts[sync.StatusExtra]; n > 0 {
				parts = append(parts, listExtraStyle.Render(fmt.Sprintf("%d extra", n)))
			}
			fmt.Fprintln(w, strings.Join(parts, listDimStyle.Render(" · ")))
		}
	} else if len(footerParts) > 0 {
		fmt.Fprintln(w, strings.Join(footerParts, listDimStyle.Render("  ·  ")))
	}

	if !st.Team.LastSync.IsZero() {
		fmt.Fprintln(w, listDimStyle.Render("Last sync: "+st.Team.LastSync.Local().Format("2006-01-02 15:04")))
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

