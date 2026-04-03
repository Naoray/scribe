package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/discovery"
)

type listModel struct {
	skills   []discovery.Skill
	cursor   int
	offset   int
	quitting bool
	width    int
	height   int
}

func newListModel(skills []discovery.Skill) listModel {
	return listModel{skills: skills}
}

func (m listModel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "escape":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}
		case "down", "j":
			if m.cursor < len(m.skills)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}
		case "home", "g":
			m.cursor = 0
			m.ensureCursorVisible()
		case "end", "G":
			m.cursor = len(m.skills) - 1
			m.ensureCursorVisible()
		}
	}
	return m, nil
}

func (m listModel) maxContentLines() int {
	if m.height == 0 {
		return 30
	}
	// 2 for header + divider, 2 for footer (blank + help)
	overhead := 4
	avail := m.height - overhead
	if avail < 5 {
		avail = 5
	}
	return avail
}

func (m *listModel) ensureCursorVisible() {
	// Each item takes 2 lines (name + description), so visible items = maxLines / 2
	visible := m.maxContentLines() / 2
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

var (
	listTUINameStyle    = lipgloss.NewStyle().Bold(true)
	listTUIDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	listTUIDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	listTUIHeaderStyle  = lipgloss.NewStyle().Bold(true)
	listTUICountStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	listTUICursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
)

func (m listModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	// Header.
	total := listTUICountStyle.Render(fmt.Sprintf("%d skills", len(m.skills)))
	b.WriteString(listTUIHeaderStyle.Render("Installed Skills") + "  " + total + "\n")
	b.WriteString(listTUIDimStyle.Render(strings.Repeat("─", 40)) + "\n")

	maxLines := m.maxContentLines()
	linesUsed := 0

	// Scroll indicator top.
	if m.offset > 0 {
		b.WriteString(listTUIDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
		linesUsed++
	}

	lastGroup := ""
	if m.offset > 0 && m.offset < len(m.skills) {
		lastGroup = m.skills[m.offset].Package
		b.WriteString(listTUIGroupHeader(lastGroup) + "\n")
		linesUsed++
	}

	end := m.offset
	for i := m.offset; i < len(m.skills); i++ {
		sk := m.skills[i]
		group := sk.Package

		linesNeeded := 2 // name + description
		if sk.Description == "" {
			linesNeeded = 1
		}
		if group != lastGroup {
			linesNeeded += 2
		}
		if linesUsed+linesNeeded > maxLines {
			break
		}

		if group != lastGroup {
			lastGroup = group
			b.WriteString("\n" + listTUIGroupHeader(group) + "\n")
			linesUsed += 2
		}

		isCursor := i == m.cursor
		if isCursor {
			b.WriteString(listTUICursorStyle.Render("▸") + " " + listTUICursorStyle.Render(sk.Name) + "\n")
		} else {
			b.WriteString("  " + listTUINameStyle.Render(sk.Name) + "\n")
		}
		linesUsed++

		if sk.Description != "" {
			b.WriteString("  " + listTUIDescStyle.Render(sk.Description) + "\n")
			linesUsed++
		}

		end = i + 1
	}

	remaining := len(m.skills) - end
	if remaining > 0 {
		b.WriteString(listTUIDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(listTUIDimStyle.Render("↑↓ navigate · q quit") + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func listTUIGroupHeader(name string) string {
	label := name
	if label == "" {
		label = "standalone"
	}
	return listTUIHeaderStyle.Render(label)
}
