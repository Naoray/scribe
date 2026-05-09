package logo

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
)

// minWidth is the smallest terminal column count that fits the full lockup.
// Frame card (12) + gap (3) + tagline "one skill. every agent." (23) ≈ 38.
// 36 leaves a sliver of slack while still falling back gracefully on
// genuinely narrow terminals (<36 cols).
const minWidth = 36

// Brand palette — pulled directly from the Scribe website (scribe-mark.svg):
//
//	#15212a — ink (dark text/frame)
//	#f3ede1 — cream (page background / inverted ink for dark terminals)
//	#b9540f — burnt orange (chip accent square in the top-left of the mark)
const (
	colorInk    = "#15212a"
	colorCream  = "#f3ede1"
	colorOrange = "#b9540f"
)

// Render writes the Scribe brand mark + wordmark lockup to w.
//
// The mark mirrors public/scribe-mark.svg: a thin-frame square card with
// an orange "chip" filling the NW interior corner, inset L-shaped
// registration ticks in the other three interior corners (NE, SE, SW),
// and a centered italic S — placed beside the wordmark, version, and
// the website tagline ("one skill. every agent.").
//
// Layout:
//
//	┌──────────┐
//	│██     ┐  │   scribe   v<version>
//	│    S     │   one skill. every agent.
//	│  └    ┘  │
//	└──────────┘
//
// Colors invert by terminal background so ink stays legible. Honors
// SCRIBE_NO_BANNER (suppress), TERM=dumb (plain "Scribe v<version>"),
// width < minWidth (plain text fallback), and NO_COLOR (no ANSI escapes).
// width <= 0 is treated as unknown (assume wide enough).
func Render(w io.Writer, version string, width int) {
	if os.Getenv("SCRIBE_NO_BANNER") != "" {
		return
	}
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}
	if width > 0 && width < minWidth {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	versionTail := fmt.Sprintf("v%s", version)

	if os.Getenv("NO_COLOR") != "" {
		renderPlain(w, versionTail)
		return
	}

	ink := lipgloss.Color(colorInk)
	if lipgloss.HasDarkBackground(os.Stdin, os.Stderr) {
		ink = lipgloss.Color(colorCream)
	}
	orange := lipgloss.Color(colorOrange)

	var (
		inkStyle  = lipgloss.NewStyle().Foreground(ink)
		dimStyle  = lipgloss.NewStyle().Foreground(ink).Faint(true)
		chipStyle = lipgloss.NewStyle().Foreground(orange).Bold(true)
		tickStyle = lipgloss.NewStyle().Foreground(ink).Faint(true)
		nameStyle = lipgloss.NewStyle().Foreground(ink).Bold(true).Italic(true)
		sStyle    = lipgloss.NewStyle().Foreground(ink).Bold(true).Italic(true)
		taglStyle = lipgloss.NewStyle().Foreground(ink).Italic(true)
	)

	// Frame: 12 cells wide (2 borders + 10 interior).
	// Interior layout (10 cols × 3 rows):
	//   row 1: chip (NW) at cols 1-2, reg-tick `┐` (NE) at col 8
	//   row 2: italic S centered at col 5
	//   row 3: reg-tick `└` (SW) at col 3, reg-tick `┘` (SE) at col 8
	// The two-cell-wide chip approximates a square given the ~2:1 cell
	// aspect ratio of typical terminal fonts.
	top := inkStyle.Render("┌──────────┐")
	bot := inkStyle.Render("└──────────┘")

	row1 := inkStyle.Render("│") +
		chipStyle.Render("██") +
		inkStyle.Render("     ") +
		tickStyle.Render("┐") +
		inkStyle.Render("  │")

	row2 := inkStyle.Render("│    ") +
		sStyle.Render("S") +
		inkStyle.Render("     │") +
		"   " + nameStyle.Render("scribe") +
		"   " + dimStyle.Render(versionTail)

	row3 := inkStyle.Render("│  ") +
		tickStyle.Render("└") +
		inkStyle.Render("    ") +
		tickStyle.Render("┘") +
		inkStyle.Render("  │") +
		"   " + taglStyle.Render("one skill. every agent.")

	fmt.Fprintln(w, top)
	fmt.Fprintln(w, row1)
	fmt.Fprintln(w, row2)
	fmt.Fprintln(w, row3)
	fmt.Fprintln(w, bot)
	fmt.Fprintln(w)
}

// renderPlain emits the same glyph layout as the styled path but without
// any ANSI escape sequences — for NO_COLOR consumers. Bold/italic styling
// drops; the structural mark (chip, ticks, S, wordmark, tagline) survives.
func renderPlain(w io.Writer, versionTail string) {
	fmt.Fprintln(w, "┌──────────┐")
	fmt.Fprintln(w, "│██     ┐  │")
	fmt.Fprintln(w, "│    S     │   scribe   "+versionTail)
	fmt.Fprintln(w, "│  └    ┘  │   one skill. every agent.")
	fmt.Fprintln(w, "└──────────┘")
	fmt.Fprintln(w)
}
