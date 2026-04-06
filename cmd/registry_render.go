package cmd

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

// registry list styles
var (
	regNameStyle  = lipgloss.NewStyle().Bold(true)
	regCountStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	regFootStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

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
		footer += " · last sync " + workflow.TimeAgo(st.LastSync)
	}

	fmt.Fprintln(w, regFootStyle.Render(footer))
	return nil
}

func printRegistryEmpty(w io.Writer) {
	fmt.Fprintln(w, "No registries connected.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Connect a registry:  scribe connect <owner/repo>")
}
