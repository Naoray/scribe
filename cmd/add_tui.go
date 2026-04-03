package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/add"
)

// ── Styles ──────────────────────────────────────────────────────────────────

var (
	addCursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	addCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	addDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	addHeaderStyle = lipgloss.NewStyle().Bold(true)
	addDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	addNameStyle   = lipgloss.NewStyle().Bold(true)
	addDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	addCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// ── Model ───────────────────────────────────────────────────────────────────

type addItem struct {
	candidate add.Candidate
	selected  bool
}

type addModel struct {
	items      []addItem
	cursor     int
	offset     int
	search     string
	targetRepo string
	confirmed  bool
	quitting   bool
	width      int
	height     int
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

// ── Update ──────────────────────────────────────────────────────────────────

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.search != "" {
				m.search = ""
				m.cursor = 0
				m.offset = 0
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
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

// ── View ────────────────────────────────────────────────────────────────────

func (m addModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	filtered := m.filteredItems()

	// Header.
	title := addHeaderStyle.Render(fmt.Sprintf("Add skills to %s", m.targetRepo))
	total := addCountStyle.Render(fmt.Sprintf("%d skills", len(filtered)))
	b.WriteString(title + "  " + total + "\n")
	b.WriteString(addDivStyle.Render(strings.Repeat("─", 40)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	if len(filtered) == 0 {
		if m.search != "" {
			b.WriteString("\n  No skills matching \"" + m.search + "\"\n")
		} else {
			b.WriteString("\n  All skills are already in " + m.targetRepo + ".\n")
		}
	} else {
		maxLines := m.maxContentLines()
		if m.search != "" {
			maxLines-- // search bar takes a line
		}
		linesUsed := 0

		if m.offset > 0 {
			b.WriteString(addDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
			linesUsed++
		}

		// Track groups for headers.
		lastGroup := ""
		if m.offset > 0 && m.offset < len(filtered) {
			lastGroup = skillGroup(filtered[m.offset].candidate)
			b.WriteString("\n" + addHeaderStyle.Render(lastGroup) + "\n")
			linesUsed += 2
		}

		end := m.offset
		for i := m.offset; i < len(filtered); i++ {
			item := filtered[i]
			group := skillGroup(item.candidate)

			// Group header.
			linesNeeded := 2 // name + description
			if item.candidate.Description == "" {
				linesNeeded = 1
			}
			if group != lastGroup {
				linesNeeded += 2 // blank + header
			}
			if linesUsed+linesNeeded > maxLines {
				break
			}

			if group != lastGroup {
				lastGroup = group
				b.WriteString("\n" + addHeaderStyle.Render(group) + "\n")
				linesUsed += 2
			}

			isCursor := i == m.cursor

			check := "[ ]"
			if item.selected {
				check = addCheckStyle.Render("[x]")
			}

			name := item.candidate.Name
			desc := item.candidate.Description

			if isCursor {
				b.WriteString(addCursorStyle.Render("▸") + " " + check + " " + addCursorStyle.Render(name) + "\n")
			} else {
				b.WriteString("  " + check + " " + addNameStyle.Render(name) + "\n")
			}
			linesUsed++

			if desc != "" {
				b.WriteString("       " + addDescStyle.Render(desc) + "\n")
				linesUsed++
			}

			end = i + 1
		}

		remaining := len(filtered) - end
		if remaining > 0 {
			b.WriteString(addDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
		}
	}

	// Footer.
	b.WriteString("\n")
	help := "↑↓ navigate · space select · enter add · q quit"
	if n := m.selectedCount(); n > 0 {
		help += fmt.Sprintf("  (%d selected)", n)
	}
	b.WriteString(addDimStyle.Render(help) + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// ── Viewport helpers ────────────────────────────────────────────────────────

func (m addModel) maxContentLines() int {
	if m.height == 0 {
		return 30
	}
	// header (2) + footer (2) + padding
	overhead := 5
	avail := m.height - overhead
	if avail < 5 {
		avail = 5
	}
	return avail
}

func (m *addModel) ensureCursorVisible() {
	// Each item is ~2 lines (name + desc), be conservative.
	visible := m.maxContentLines() / 2
	if visible < 3 {
		visible = 3
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

// ── Data helpers ────────────────────────────────────────────────────────────

func (m addModel) filteredItems() []addItem {
	if m.search == "" {
		return m.items
	}
	var result []addItem
	lower := strings.ToLower(m.search)
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.candidate.Name), lower) {
			result = append(result, item)
		}
	}
	return result
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
