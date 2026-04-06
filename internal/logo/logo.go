package logo

import (
	"fmt"
	"io"
	"os"
	"strings"

	"image/color"

	"charm.land/lipgloss/v2"
)

// logoFull is the ANSI Shadow style logo (~48 chars wide x 6 lines).
const logoFull = `███████╗ ██████╗██████╗ ██╗██████╗ ███████╗
██╔════╝██╔════╝██╔══██╗██║██╔══██╗██╔════╝
███████╗██║     ██████╔╝██║██████╔╝█████╗
╚════██║██║     ██╔══██╗██║██╔══██╗██╔══╝
███████║╚██████╗██║  ██║██║██████╔╝███████╗
╚══════╝ ╚═════╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝`

// logoCompact is the small FIGlet logo (~28 chars wide x 4 lines).
const logoCompact = ` ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|`

// Render writes the Scribe logo and version to w.
// width is the terminal width in columns — used to select logo size.
// Respects SCRIBE_NO_BANNER, TERM=dumb, and NO_COLOR environment variables.
func Render(w io.Writer, version string, width int) {
	// SCRIBE_NO_BANNER: suppress entirely.
	if os.Getenv("SCRIBE_NO_BANNER") != "" {
		return
	}

	// TERM=dumb: plain text only, no block characters.
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	// Select logo size based on terminal width.
	var art string
	switch {
	case width >= 60:
		art = logoFull
	case width >= 40:
		art = logoCompact
	default:
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	noColor := os.Getenv("NO_COLOR") != ""
	lines := strings.Split(art, "\n")

	if noColor {
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	} else {
		colors := gradient(len(lines))
		for i, line := range lines {
			style := lipgloss.NewStyle().Foreground(colors[i]).Bold(true)
			fmt.Fprintln(w, style.Render(line))
		}
	}

	// Version below the logo, dimmed.
	if noColor {
		fmt.Fprintf(w, "v%s\n", version)
	} else {
		dim := lipgloss.NewStyle().Faint(true)
		fmt.Fprintln(w, dim.Render(fmt.Sprintf("v%s", version)))
	}
	fmt.Fprintln(w)
}

// gradient returns a slice of colors for per-line logo rendering.
// Uses dark or light palette based on terminal background detection.
func gradient(n int) []color.Color {
	var start, end string

	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	if isDark {
		start, end = "#00B4D8", "#60E890"
	} else {
		start, end = "#0077B6", "#2D6A4F"
	}

	return lipgloss.Blend1D(n, lipgloss.Color(start), lipgloss.Color(end))
}
