package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// RegistryListSteps returns the step list for the registry list command.
func RegistryListSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"PrintRegistryList", StepPrintRegistryList},
	}
}

// CountSkillsPerRegistry counts installed skills per registry by matching
// the slugified registry prefix in namespaced skill keys (e.g. "ArtistfyHQ-team-skills/deploy" matches "ArtistfyHQ/team-skills").
func CountSkillsPerRegistry(repos []string, st *state.State) map[string]int {
	counts := make(map[string]int, len(repos))
	for _, repo := range repos {
		counts[repo] = 0
		slug := tools.SlugifyRegistry(repo)
		for name := range st.Installed {
			if strings.HasPrefix(name, slug+"/") {
				counts[repo]++
			}
		}
	}
	return counts
}

// list styles
var (
	regNameStyle  = lipgloss.NewStyle().Bold(true)
	regCountStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	regFootStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// StepPrintRegistryList renders connected registries as styled text or JSON.
func StepPrintRegistryList(_ context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	repos := b.Config.TeamRepos()

	if len(repos) == 0 {
		if useJSON {
			return PrintRegistryJSON(w, nil, b.State)
		}
		fmt.Fprintln(w, "No registries connected.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Connect a registry:  scribe connect <owner/repo>")
		return nil
	}

	counts := CountSkillsPerRegistry(repos, b.State)

	if useJSON {
		return PrintRegistryJSON(w, repos, b.State)
	}
	return printRegistryTable(w, repos, counts, b.State)
}

func printRegistryTable(w io.Writer, repos []string, counts map[string]int, st *state.State) error {
	for _, repo := range repos {
		count := regCountStyle.Render(fmt.Sprintf("(%d)", counts[repo]))
		fmt.Fprintf(w, "%s %s\n", regNameStyle.Render(repo), count)
	}

	fmt.Fprintln(w)

	footer := fmt.Sprintf("%d registries connected", len(repos))
	if len(repos) == 1 {
		footer = "1 registry connected"
	}
	if st.LastSync.IsZero() {
		footer += " · never synced"
	} else {
		footer += " · last sync " + timeAgo(st.LastSync)
	}

	fmt.Fprintln(w, regFootStyle.Render(footer))
	return nil
}

type regJSON struct {
	Registry   string `json:"registry"`
	SkillCount int    `json:"skill_count"`
}

type regListJSON struct {
	Registries []regJSON `json:"registries"`
	LastSync   *string   `json:"last_sync"`
}

// PrintRegistryJSON writes registry list as JSON to w.
func PrintRegistryJSON(w io.Writer, repos []string, st *state.State) error {
	counts := CountSkillsPerRegistry(repos, st)

	entries := make([]regJSON, 0, len(repos))
	for _, repo := range repos {
		entries = append(entries, regJSON{
			Registry:   repo,
			SkillCount: counts[repo],
		})
	}

	var lastSync *string
	if !st.LastSync.IsZero() {
		s := st.LastSync.UTC().Format("2006-01-02T15:04:05Z")
		lastSync = &s
	}

	out := regListJSON{
		Registries: entries,
		LastSync:   lastSync,
	}

	// Ensure empty slice renders as [] not null.
	if out.Registries == nil {
		out.Registries = []regJSON{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
