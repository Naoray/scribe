package cmd

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/sync"
)

// list styles — shared between the TUI views.
var (
	// statusStyles maps each sync status to its lipgloss style.
	statusStyles = map[sync.Status]lipgloss.Style{
		sync.StatusCurrent:  lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")),
		sync.StatusOutdated: lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")),
		sync.StatusMissing:  lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")),
		sync.StatusExtra:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3")),
	}
)

// renderStatusCount returns a styled "N label" string, or "" if count is zero.
func renderStatusCount(s sync.Status, n int) string {
	if n == 0 {
		return ""
	}
	return statusStyles[s].Render(fmt.Sprintf("%d %s", n, s.Display().Label))
}
