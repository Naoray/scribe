package logo

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
)

// minWidth is the smallest terminal column count that fits the full lockup.
// Card (14) + gap (3) + tagline "one skill. every agent." (23) ≈ 40.
// 38 leaves a sliver of slack while still falling back gracefully on
// genuinely narrow terminals (<38 cols).
const minWidth = 38

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
// an orange "chip" filling the NW interior corner and a calligraphic
// italic S filling the body of the card. The S is rendered in FIGlet
// "Slant" style so it actually slants in any terminal — independent of
// whether the terminal supports italic ANSI styling.
//
// Layout:
//
//	┌────────────┐
//	│██          │
//	│    _____   │   scribe   v<version>
//	│   / ___/   │
//	│   \__ \    │   one skill. every agent.
//	│  ___/ /    │
//	│ /____/     │
//	└────────────┘
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
		sStyle    = lipgloss.NewStyle().Foreground(ink).Bold(true)
		nameStyle = lipgloss.NewStyle().Foreground(ink).Bold(true).Italic(true)
		taglStyle = lipgloss.NewStyle().Foreground(ink).Italic(true)
	)

	// Frame: 14 cells wide (2 borders + 12 interior).
	// Body of the card holds a 5-row FIGlet "Slant" S, drawn in ink, with
	// the orange chip occupying the NW interior corner on row 1.
	top := inkStyle.Render("┌────────────┐")
	bot := inkStyle.Render("└────────────┘")

	row1 := inkStyle.Render("│") + chipStyle.Render("██") + inkStyle.Render("          │")
	row2 := inkStyle.Render("│    ") + sStyle.Render("_____") + inkStyle.Render("   │") +
		"   " + nameStyle.Render("scribe") + "   " + dimStyle.Render(versionTail)
	row3 := inkStyle.Render("│   ") + sStyle.Render("/ ___/") + inkStyle.Render("   │")
	row4 := inkStyle.Render("│   ") + sStyle.Render(`\__ \ `) + inkStyle.Render("  │") +
		"   " + taglStyle.Render("one skill. every agent.")
	row5 := inkStyle.Render("│  ") + sStyle.Render(`___/ / `) + inkStyle.Render("  │")
	row6 := inkStyle.Render("│ ") + sStyle.Render(`/____/  `) + inkStyle.Render(" │")

	fmt.Fprintln(w, top)
	fmt.Fprintln(w, row1)
	fmt.Fprintln(w, row2)
	fmt.Fprintln(w, row3)
	fmt.Fprintln(w, row4)
	fmt.Fprintln(w, row5)
	fmt.Fprintln(w, row6)
	fmt.Fprintln(w, bot)
	fmt.Fprintln(w)
}

// renderPlain emits the same glyph layout as the styled path but without
// any ANSI escape sequences — for NO_COLOR consumers. Bold/italic styling
// drops; the structural mark (chip, S, wordmark, tagline) survives.
func renderPlain(w io.Writer, versionTail string) {
	fmt.Fprintln(w, "┌────────────┐")
	fmt.Fprintln(w, "│██          │")
	fmt.Fprintln(w, "│    _____   │   scribe   "+versionTail)
	fmt.Fprintln(w, "│   / ___/   │")
	fmt.Fprintln(w, `│   \__ \    │   one skill. every agent.`)
	fmt.Fprintln(w, "│  ___/ /    │")
	fmt.Fprintln(w, "│ /____/     │")
	fmt.Fprintln(w, "└────────────┘")
	fmt.Fprintln(w)
}
