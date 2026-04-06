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
)

func runHub(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	st, stErr := state.Load()
	if stErr != nil {
		st = &state.State{Installed: make(map[string]state.InstalledSkill)}
	}

	// JSON mode: --json flag, non-TTY stdout, or CI environment.
	if jsonFlag || !isatty.IsTerminal(os.Stdout.Fd()) || os.Getenv("CI") != "" {
		return writeHubJSON(os.Stdout, Version, cfg, st)
	}

	// TERM=dumb: plain text, no menu.
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(os.Stdout, "Scribe v%s\n", Version)
		writeStatusPlain(os.Stdout, cfg, st)
		return nil
	}

	// TTY mode: logo + styled status + action menu.
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

	return showActionMenu(cmd)
}

func writeHubJSON(w io.Writer, version string, cfg *config.Config, st *state.State) error {
	repos := cfg.TeamRepos()

	status := struct {
		Version        string   `json:"version"`
		Registries     []string `json:"registries"`
		InstalledCount int      `json:"installed_count"`
		LastSync       string   `json:"last_sync,omitempty"`
		PendingUpdates int      `json:"pending_updates"`
		StaleStatus    bool     `json:"stale_status"`
	}{
		Version:        version,
		Registries:     repos,
		InstalledCount: len(st.Installed),
		PendingUpdates: 0,
		StaleStatus:    true,
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
	fmt.Fprintf(w, "Last sync:  %s\n", formatRelativeTime(st.LastSync))
}

func writeStatusStyled(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()

	labelStyle := lipgloss.NewStyle().Faint(true)
	valueStyle := lipgloss.NewStyle().Bold(true)

	lines := []struct{ label, value string }{
		{"Registries", fmt.Sprintf("%d connected", len(repos))},
		{"Skills", fmt.Sprintf("%d installed", len(st.Installed))},
		{"Last sync", formatRelativeTime(st.LastSync)},
	}

	if len(repos) > 0 {
		var out []struct{ label, value string }
		out = append(out, lines[0]) // "Registries" header
		for _, r := range repos {
			out = append(out, struct{ label, value string }{"", r})
		}
		out = append(out, lines[1:]...) // Skills, Last sync
		lines = out
	}

	for _, l := range lines {
		if l.label == "" {
			fmt.Fprintf(w, "  %s  %s\n", labelStyle.Render("         "), valueStyle.Render(l.value))
		} else {
			fmt.Fprintf(w, "  %s  %s\n", labelStyle.Render(fmt.Sprintf("%9s", l.label)), valueStyle.Render(l.value))
		}
	}
	fmt.Fprintln(w)
}

func showActionMenu(cmd *cobra.Command) error {
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

	// Look up and execute the subcommand directly to avoid recursive rootCmd.Execute().
	sub, _, err := rootCmd.Find([]string{action})
	if err != nil {
		return fmt.Errorf("unknown action: %s", action)
	}
	sub.SetArgs(nil)
	return sub.Execute()
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
