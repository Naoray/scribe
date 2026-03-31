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

func (m addModel) Init() tea.Cmd { return nil }

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			// Skip group headers.
			for m.cursor > 0 && m.filteredItems()[m.cursor].candidate.Name == "" {
				m.cursor--
			}
		case "down", "j":
			filtered := m.filteredItems()
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}
			for m.cursor < len(filtered)-1 && filtered[m.cursor].candidate.Name == "" {
				m.cursor++
			}
		case "space":
			filtered := m.filteredItems()
			if m.cursor < len(filtered) {
				// Toggle in the original items list.
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
			}
		default:
			if len(msg.String()) == 1 {
				m.search += msg.String()
				m.cursor = 0
			}
		}
	}
	return m, nil
}

func (m addModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Add skills to %s", m.targetRepo),
	)
	b.WriteString(title + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> Search: %s\n\n", m.search))
	} else {
		b.WriteString("> Search: \n\n")
	}

	filtered := m.filteredItems()
	if len(filtered) == 0 {
		if m.search != "" {
			b.WriteString("  No skills matching \"" + m.search + "\"\n")
		} else {
			b.WriteString("  All available skills are already in " + m.targetRepo + ".\n")
		}
	}

	// Group by origin.
	currentGroup := ""
	for i, item := range filtered {
		group := itemGroup(item.candidate)
		if group != currentGroup {
			currentGroup = group
			header := lipgloss.NewStyle().Bold(true).Render(group)
			b.WriteString("\n" + header + "\n")
		}

		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if item.selected {
			check = "[x]"
		}

		origin := shortOrigin(item.candidate)
		b.WriteString(fmt.Sprintf("%s%s %-20s %s\n", cursor, check, item.candidate.Name, origin))
	}

	b.WriteString("\n↑↓ navigate · space select · enter add · q quit")

	if n := m.selectedCount(); n > 0 {
		b.WriteString(fmt.Sprintf("  (%d selected)", n))
	}
	b.WriteString("\n")

	return tea.NewView(b.String())
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

func itemGroup(c add.Candidate) string {
	if c.Origin == "local" {
		return "LOCAL"
	}
	if strings.HasPrefix(c.Origin, "registry:") {
		return "FROM " + strings.TrimPrefix(c.Origin, "registry:")
	}
	return "OTHER"
}

func shortOrigin(c add.Candidate) string {
	if c.LocalPath != "" {
		// Show abbreviated path.
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
