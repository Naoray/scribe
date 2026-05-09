package logo

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
)

// minWidth is the smallest terminal column count that fits the full lockup.
// Card (12) + gap (1) + "cribe" art (28) = 41 cols. minWidth = 43 leaves a
// 2-col safety margin; below that we fall back to plain text.
const minWidth = 43

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

// FIGlet "Slant" rows split across the chip card (the S) and the wordmark
// continuation ("cribe") so the lockup reads as one word: [S]cribe.
//
// Generated with `figlet -f slant -k` (kerning, no smushing). Default
// smushing merges the `c` bottom-right with the `r` top, making the `c`
// look like an `e` with a horizontal bar — kerning keeps each letter
// visually distinct.
var (
	sArt = [5]string{
		"    _____ ",
		"   / ___/ ",
		`   \__ \  `,
		"  ___/ /  ",
		" /____/   ",
	}
	cribeArt = [5]string{
		"               _  __        ",
		"  _____ _____ (_)/ /_   ___ ",
		` / ___// ___// // __ \ / _ \`,
		"/ /__ / /   / // /_/ //  __/",
		`\___//_/   /_//_.___/ \___/ `,
	}
)

// Render writes the Scribe brand mark + wordmark lockup to w.
//
// The mark mirrors public/scribe-mark.svg: a thin-frame square card with
// an orange "chip" filling the NW interior corner and a calligraphic
// italic S filling the body of the card. The wordmark "cribe" continues
// from the card in the same FIGlet "Slant" font, so the whole reads as
// "Scribe" with the S styled as a contained brand mark.
//
// Layout:
//
//	┌──────────┐
//	│██  _____ │               _  __
//	│   / ___/ │  _____ _____ (_)/ /_   ___
//	│   \__ \  │ / ___// ___// // __ \ / _ \
//	│  ___/ /  │/ /__ / /   / // /_/ //  __/
//	│ /____/   │\___//_/   /_//_.___/ \___/
//	└──────────┘
//
//	v<version>   ·   one skill. every agent.
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
		wordStyle = lipgloss.NewStyle().Foreground(ink).Bold(true)
		taglStyle = lipgloss.NewStyle().Foreground(ink).Italic(true)
	)

	const gap = " "

	// Row 1: top frame.
	fmt.Fprintln(w, inkStyle.Render("┌──────────┐"))

	// Row 2: chip overlaid on S row 1 (the chip occupies the NW interior
	// corner where sArt[0] otherwise has leading whitespace), beside
	// cribe row 1.
	row1 := inkStyle.Render("│") +
		chipStyle.Render("██") +
		wordStyle.Render(sArt[0][2:]) +
		inkStyle.Render("│") +
		gap +
		wordStyle.Render(cribeArt[0])
	fmt.Fprintln(w, row1)

	// Rows 3–6: remaining S art rows + matching "cribe" rows.
	for i := 1; i < 5; i++ {
		row := inkStyle.Render("│") +
			wordStyle.Render(sArt[i]) +
			inkStyle.Render("│") +
			gap +
			wordStyle.Render(cribeArt[i])
		fmt.Fprintln(w, row)
	}

	// Row 7: bottom frame.
	fmt.Fprintln(w, inkStyle.Render("└──────────┘"))

	// Metadata line below the lockup.
	fmt.Fprintln(w)
	fmt.Fprintln(w, dimStyle.Render(versionTail)+"   "+inkStyle.Render("·")+"   "+taglStyle.Render("one skill. every agent."))
	fmt.Fprintln(w)
}

// renderPlain emits the same glyph layout as the styled path but without
// any ANSI escape sequences — for NO_COLOR consumers. Bold/italic styling
// drops; the structural lockup (chip, S, "cribe" wordmark, tagline) survives.
func renderPlain(w io.Writer, versionTail string) {
	const gap = " "

	fmt.Fprintln(w, "┌──────────┐")
	fmt.Fprintln(w, "│██"+sArt[0][2:]+"│"+gap+cribeArt[0])
	for i := 1; i < 5; i++ {
		fmt.Fprintln(w, "│"+sArt[i]+"│"+gap+cribeArt[i])
	}
	fmt.Fprintln(w, "└──────────┘")
	fmt.Fprintln(w)
	fmt.Fprintln(w, versionTail+"   ·   one skill. every agent.")
	fmt.Fprintln(w)
}
