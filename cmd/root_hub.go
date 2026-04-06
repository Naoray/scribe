package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/logo"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

const labelWidth = 9

func runHub(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	st, err := state.Load()
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

	// Stdin must be a TTY for the interactive menu.
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stdout, "Run 'scribe --help' to see available commands.")
		return nil
	}

	return showActionMenu()
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

func writeStatusPlain(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()
	fmt.Fprintf(w, "Registries: %d connected\n", len(repos))
	fmt.Fprintf(w, "Skills:     %d installed\n", len(st.Installed))
	fmt.Fprintf(w, "Last sync:  %s\n", workflow.TimeAgo(st.LastSync))
}

func writeStatusStyled(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()

	labelStyle := lipgloss.NewStyle().Faint(true)
	valueStyle := lipgloss.NewStyle().Bold(true)

	lines := []struct{ label, value string }{
		{"Registries", fmt.Sprintf("%d connected", len(repos))},
		{"Skills", fmt.Sprintf("%d installed", len(st.Installed))},
		{"Last sync", workflow.TimeAgo(st.LastSync)},
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

	for _, l := range lines {
		fmt.Fprintf(w, "  %s  %s\n",
			labelStyle.Render(fmt.Sprintf("%*s", labelWidth, l.label)),
			valueStyle.Render(l.value),
		)
	}
	fmt.Fprintln(w)
}

func showActionMenu() error {
	var action string
	err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(
			huh.NewOption("Sync skills from registries", "sync"),
			huh.NewOption("List installed skills", "list"),
			huh.NewOption("Connect a registry", "connect"),
			huh.NewOption("Interactive setup guide", "guide"),
			huh.NewOption("Show help", "help"),
		).
		Value(&action).
		Run()

	if err != nil {
		// Ctrl+C or other interrupt — exit cleanly.
		if err == huh.ErrUserAborted {
			os.Exit(130)
		}
		return err
	}

	// Dispatch to the selected subcommand via SetArgs so Cobra parses the
	// action name instead of falling back to os.Args (which would re-enter runHub).
	rootCmd.SetArgs([]string{action})
	return rootCmd.Execute()
}
