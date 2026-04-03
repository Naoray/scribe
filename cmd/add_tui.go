package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/add"
)

type addItem struct {
	candidate add.Candidate
	selected  bool
}

type addModel struct {
	items      []addItem
	cursor     int
	search     string
	targetRepo string
	confirmed  bool
	quitting   bool
	width      int
	height     int
	offset     int // viewport scroll offset
}

func newAddModel(candidates []add.Candidate, targetRepo string) addModel {
	items := make([]addItem, len(candidates))
	for i, c := range candidates {
		items[i] = addItem{candidate: c}
	}
	return addModel{
		items:      items,
		targetRepo: targetRepo,
	}
}

func (m addModel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.search == "" {
				m.quitting = true
				return m, tea.Quit
			}
			// If searching, clear search instead of quitting.
			m.search = ""
			m.cursor = 0
			m.offset = 0
		case "escape":
			if m.search != "" {
				m.search = ""
				m.cursor = 0
				m.offset = 0
			} else {
				m.quitting = true
				return m, tea.Quit
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}
		case "down", "j":
			filtered := m.filteredItems()
			if m.cursor < len(filtered)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}
		case "space":
			filtered := m.filteredItems()
			if m.cursor < len(filtered) {
				name := filtered[m.cursor].candidate.Name
				for i := range m.items {
					if m.items[i].candidate.Name == name {
						m.items[i].selected = !m.items[i].selected
						break
					}
				}
			}
		case "enter":
			if m.selectedCount() > 0 {
				m.confirmed = true
				return m, tea.Quit
			}
		case "backspace":
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.cursor = 0
				m.offset = 0
			}
		default:
			if len(msg.String()) == 1 {
				m.search += msg.String()
				m.cursor = 0
				m.offset = 0
			}
		}
	}
	return m, nil
}

// maxContentLines returns how many lines are available for items + group headers,
// accounting for the header (title + search + blank) and footer lines.
func (m addModel) maxContentLines() int {
	if m.height == 0 {
		return 30 // sensible default before first WindowSizeMsg
	}
	// 3 lines for header (title, search, blank line) + 3 for footer (blank, help, scroll hint)
	overhead := 6
	avail := m.height - overhead
	if avail < 5 {
		avail = 5
	}
	return avail
}

// visibleItemCount estimates how many items fit, used by ensureCursorVisible.
// Slightly conservative to leave room for group headers.
func (m addModel) visibleItemCount() int {
	return m.maxContentLines() - 3 // reserve space for possible group headers
}

// ensureCursorVisible adjusts offset so the cursor stays in the viewport.
func (m *addModel) ensureCursorVisible() {
	visible := m.visibleItemCount()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

var (
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

func (m addModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	// Header.
	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Add skills to %s", m.targetRepo),
	)
	b.WriteString(title + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n\n", m.search))
	} else {
		b.WriteString(dimStyle.Render("> type to filter...") + "\n\n")
	}

	filtered := m.filteredItems()
	if len(filtered) == 0 {
		if m.search != "" {
			b.WriteString("  No skills matching \"" + m.search + "\"\n")
		} else {
			b.WriteString("  All available skills are already in " + m.targetRepo + ".\n")
		}
	} else {
		maxLines := m.maxContentLines()
		linesUsed := 0

		// Scroll indicator top.
		if m.offset > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
			linesUsed++
		}

		lastGroup := ""
		// If we're scrolled past the first item, show the group of the first visible item.
		if m.offset > 0 {
			lastGroup = skillGroup(filtered[m.offset].candidate)
			b.WriteString(lipgloss.NewStyle().Bold(true).Render(lastGroup) + "\n")
			linesUsed++
		}

		end := m.offset
		for i := m.offset; i < len(filtered); i++ {
			item := filtered[i]
			group := skillGroup(item.candidate)

			// Check if a group header would fit.
			linesNeeded := 1 // the item itself
			if group != lastGroup {
				linesNeeded += 2 // blank line + header
			}
			if linesUsed+linesNeeded > maxLines {
				break
			}

			if group != lastGroup {
				lastGroup = group
				b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render(group) + "\n")
				linesUsed += 2
			}

			isCursor := i == m.cursor

			check := "[ ]"
			if item.selected {
				check = selectedStyle.Render("[x]")
			}

			name := item.candidate.Name
			origin := shortOrigin(item.candidate)

			if isCursor {
				line := fmt.Sprintf("> %s %-24s %s", check, name, dimStyle.Render(origin))
				b.WriteString(cursorStyle.Render(">") + line[1:] + "\n")
			} else {
				b.WriteString(fmt.Sprintf("  %s %-24s %s\n", check, name, dimStyle.Render(origin)))
			}
			linesUsed++
			end = i + 1
		}

		// Scroll indicator bottom.
		remaining := len(filtered) - end
		if remaining > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
		}
	}

	// Footer.
	b.WriteString("\n")
	help := "↑↓ navigate · space select · enter add · q quit"
	if n := m.selectedCount(); n > 0 {
		help += fmt.Sprintf("  (%d selected)", n)
	}
	b.WriteString(dimStyle.Render(help) + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m addModel) filteredItems() []addItem {
	if m.search == "" {
		return m.items
	}
	var filtered []addItem
	lower := strings.ToLower(m.search)
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.candidate.Name), lower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m addModel) selectedCount() int {
	n := 0
	for _, item := range m.items {
		if item.selected {
			n++
		}
	}
	return n
}

func (m addModel) selectedCandidates() []add.Candidate {
	var selected []add.Candidate
	for _, item := range m.items {
		if item.selected {
			selected = append(selected, item.candidate)
		}
	}
	return selected
}

func skillGroup(c add.Candidate) string {
	if c.Origin != "local" {
		if strings.HasPrefix(c.Origin, "registry:") {
			return strings.TrimPrefix(c.Origin, "registry:")
		}
		return c.Origin
	}
	if c.Package != "" {
		return c.Package
	}
	return "standalone"
}

func shortOrigin(c add.Candidate) string {
	if c.Package != "" {
		return c.Package
	}
	if c.LocalPath != "" {
		if strings.Contains(c.LocalPath, ".claude") {
			return "~/.claude/skills"
		}
		return "~/.scribe/skills"
	}
	if c.Source != "" {
		if len(c.Source) > 30 {
			return c.Source[:27] + "..."
		}
		return c.Source
	}
	return ""
}
