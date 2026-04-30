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

func runDefault(cmd *cobra.Command, args []string) error {
	return runList(cmd, args)
}

func runStatus(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)
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
		r := jsonRendererForCommand(cmd, jsonFlag)
		if err := r.Result(buildStatusOutput(Version, cfg, st)); err != nil {
			return err
		}
		return r.Flush()
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
	return nil
}

type statusOutput struct {
	Version        string   `json:"version"`
	Registries     []string `json:"registries"`
	InstalledCount int      `json:"installed_count"`
	LastSync       string   `json:"last_sync,omitempty"`
}

func buildStatusOutput(version string, cfg *config.Config, st *state.State) statusOutput {
	repos := cfg.TeamRepos()
	out := statusOutput{
		Version:        version,
		Registries:     repos,
		InstalledCount: len(st.Installed),
	}
	if repos == nil {
		out.Registries = []string{}
	}
	if !st.LastSync.IsZero() {
		out.LastSync = st.LastSync.UTC().Format(time.RFC3339)
	}
	return out
}

func writeStatusJSON(w io.Writer, version string, cfg *config.Config, st *state.State) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildStatusOutput(version, cfg, st))
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
	if shouldShowNoToolHint(st) {
		fmt.Fprintln(w, "Hint: scribe installed but no AI tool detected. Run `scribe tools` to see what's supported.")
	}
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
	if shouldShowNoToolHint(st) {
		fmt.Fprintf(w, "  %s  %s\n",
			labelStyle.Render(fmt.Sprintf("%*s", labelWidth, "Hint")),
			valueStyle.Render("scribe installed but no AI tool detected. Run `scribe tools` to see what's supported."),
		)
	}
	fmt.Fprintln(w)
}

func shouldShowNoToolHint(st *state.State) bool {
	if len(st.Installed) != 1 {
		return false
	}
	installed, ok := st.Installed["scribe"]
	if !ok {
		return false
	}
	return installed.Origin == state.OriginBootstrap && len(installed.Tools) == 0
}
