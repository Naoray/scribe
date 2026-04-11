package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/logo"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

const labelWidth = 10 // "Registries" is the longest label at 10 chars

func runHub(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		cfg = &config.Config{}
	}
	st, err := factory.State()
	if err != nil {
		st = &state.State{Installed: make(map[string]state.InstalledSkill)}
	}

	if jsonFlag || !isatty.IsTerminal(os.Stdout.Fd()) || os.Getenv("CI") != "" {
		return writeHubJSON(os.Stdout, Version, cfg, st)
	}

	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(os.Stdout, "Scribe v%s\n", Version)
		writeStatusPlain(os.Stdout, cfg, st)
		return nil
	}

	width, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if width <= 0 {
		width = 80
	}

	logo.Render(os.Stdout, Version, width)
	writeStatusStyled(os.Stdout, cfg, st)

	// Show the standard cobra help below the logo + status so users see every
	// available command at a glance.
	cmd.SetOut(os.Stdout)
	return cmd.Help()
}

func writeHubJSON(w io.Writer, version string, cfg *config.Config, st *state.State) error {
	repos := cfg.TeamRepos()

	status := struct {
		Version        string   `json:"version"`
		Registries     []string `json:"registries"`
		InstalledCount int      `json:"installed_count"`
		LastSync       string   `json:"last_sync,omitempty"`
	}{
		Version:        version,
		Registries:     repos,
		InstalledCount: len(st.Installed),
	}

	if repos == nil {
		status.Registries = []string{}
	}

	if !st.LastSync.IsZero() {
		status.LastSync = st.LastSync.UTC().Format(time.RFC3339)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

// syncTime returns a human-readable last-sync string. Uses "never" for the
// zero value rather than workflow.TimeAgo's "never synced" to avoid
// redundancy when shown beside the "Last sync" label.
func syncTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return workflow.TimeAgo(t)
}

func writeStatusPlain(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()
	fmt.Fprintf(w, "Registries: %d connected\n", len(repos))
	fmt.Fprintf(w, "Skills:     %d installed\n", len(st.Installed))
	fmt.Fprintf(w, "Last sync:  %s\n", syncTime(st.LastSync))
}

func writeStatusStyled(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()

	labelStyle := lipgloss.NewStyle().Faint(true)
	valueStyle := lipgloss.NewStyle().Bold(true)

	lines := []struct{ label, value string }{
		{"Registries", fmt.Sprintf("%d connected", len(repos))},
		{"Skills", fmt.Sprintf("%d installed", len(st.Installed))},
		{"Last sync", syncTime(st.LastSync)},
	}

	if len(repos) > 0 {
		var out []struct{ label, value string }
		out = append(out, lines[0])
		for _, r := range repos {
			out = append(out, struct{ label, value string }{"", r})
		}
		out = append(out, lines[1:]...)
		lines = out
	}

	for _, line := range lines {
		fmt.Fprintf(w, "  %s  %s\n",
			labelStyle.Render(fmt.Sprintf("%*s", labelWidth, line.label)),
			valueStyle.Render(line.value),
		)
	}
	fmt.Fprintln(w)
}
