package logo

import (
	"fmt"
	"io"
	"os"
	"strings"

	"image/color"

	"charm.land/lipgloss/v2"
)

// bannerWidth is the minimum terminal width below which we fall back to plain
// text. The banner itself is ~30 chars; pad with version + buffer.
const bannerWidth = 30

// Render writes the Scribe banner and version to w on a single line.
//
// Layout: `█████ scribe ─────  v<version>`
//   - blocks + "scribe": cyan→green gradient (left to right)
//   - "─────" divider: continues the gradient
//   - " v<version>": dim
//
// width is the terminal width in columns. Below bannerWidth, falls back to
// plain `Scribe v<version>`. Honors SCRIBE_NO_BANNER, TERM=dumb, NO_COLOR.
// width <= 0 is treated as unknown (assume wide enough for the banner).
func Render(w io.Writer, version string, width int) {
	if os.Getenv("SCRIBE_NO_BANNER") != "" {
		return
	}
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}
	if width > 0 && width < bannerWidth {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	const (
		blocks  = "█████"
		name    = " scribe "
		divider = "─────"
	)
	bannerCore := blocks + name + divider // gradient applied across this run
	versionPart := fmt.Sprintf("  v%s", version)

	noColor := os.Getenv("NO_COLOR") != ""
	if noColor {
		fmt.Fprintln(w, bannerCore+versionPart)
		fmt.Fprintln(w)
		return
	}

	colors := gradient(len([]rune(bannerCore)))
	var sb strings.Builder
	for i, r := range []rune(bannerCore) {
		style := lipgloss.NewStyle().Foreground(colors[i]).Bold(true)
		sb.WriteString(style.Render(string(r)))
	}
	dim := lipgloss.NewStyle().Faint(true)
	sb.WriteString(dim.Render(versionPart))

	fmt.Fprintln(w, sb.String())
	fmt.Fprintln(w)
}

// gradient returns n colors blended cyan→green, choosing palette by background.
func gradient(n int) []color.Color {
	var start, end string
	if lipgloss.HasDarkBackground(os.Stdin, os.Stderr) {
		start, end = "#00B4D8", "#60E890"
	} else {
		start, end = "#0077B6", "#2D6A4F"
	}
	return lipgloss.Blend1D(n, lipgloss.Color(start), lipgloss.Color(end))
}
