// internal/ui/styles.go
package ui

import "charm.land/lipgloss/v2"

var (
	// Title is used for section headers like "Scribe Guide".
	Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	// CheckOK renders a passing prereq check.
	CheckOK = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	// CheckFail renders a failing prereq check.
	CheckFail = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4672"))

	// CheckPending renders a neutral/pending check.
	CheckPending = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))

	// Subtle is for secondary information.
	Subtle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))

	// Bold is for emphasis in summaries.
	Bold = lipgloss.NewStyle().Bold(true)

	// Summary wraps the final summary box.
	Summary = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#04B575")).
		Padding(1, 2).
		MarginTop(1)
)
