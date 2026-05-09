package logo

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
)

// minWidth is the smallest terminal column count that fits the full lockup.
// Mark (8) + gap (3) + "scribe" (6) + gap (3) + version tail (~7) ≈ 27.
const minWidth = 28

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

// Render writes the Scribe brand lockup and version to w.
//
// Layout (mark + wordmark, matching the website's scribe-lockup.svg):
//
//	┌──────┐
//	│▇     │
//	│      │   scribe   v<version>
//	│   S  │
//	└──────┘
//
// Colors invert by terminal background so the ink stays legible.
// Honors SCRIBE_NO_BANNER (suppresses output), TERM=dumb (plain text fallback),
// NO_COLOR (strips ANSI), and width below minWidth (plain text fallback).
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
		chipStyle = lipgloss.NewStyle().Foreground(orange)
		nameStyle = lipgloss.NewStyle().Foreground(ink).Bold(true).Italic(true)
		versStyle = lipgloss.NewStyle().Foreground(ink).Faint(true)
	)

	row1 := inkStyle.Render("┌──────┐")
	row2 := inkStyle.Render("│") + chipStyle.Render("▇") + inkStyle.Render("     │")
	row3 := inkStyle.Render("│      │") + "   " + nameStyle.Render("scribe") + "   " + versStyle.Render(versionTail)
	row4 := inkStyle.Render("│   ") + nameStyle.Render("S") + inkStyle.Render("  │")
	row5 := inkStyle.Render("└──────┘")

	fmt.Fprintln(w, row1)
	fmt.Fprintln(w, row2)
	fmt.Fprintln(w, row3)
	fmt.Fprintln(w, row4)
	fmt.Fprintln(w, row5)
	fmt.Fprintln(w)
}

// renderPlain emits the lockup with no ANSI escapes — used for NO_COLOR mode.
// Bold/italic styling is also dropped since NO_COLOR consumers want plain text.
func renderPlain(w io.Writer, versionTail string) {
	fmt.Fprintln(w, "┌──────┐")
	fmt.Fprintln(w, "│▇     │")
	fmt.Fprintln(w, "│      │   scribe   "+versionTail)
	fmt.Fprintln(w, "│   S  │")
	fmt.Fprintln(w, "└──────┘")
	fmt.Fprintln(w)
}
