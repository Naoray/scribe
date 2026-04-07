package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/sync"
)

// ── Styles ──────────────────────────────────────────────────────────────────

var (
	installCursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	installCheckStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	installDimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	installHeaderStyle    = lipgloss.NewStyle().Bold(true)
	installDivStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	installNameStyle      = lipgloss.NewStyle().Bold(true)
	installDescStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	installCountStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	installInstalledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	installOutdatedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
)

// ── Model ───────────────────────────────────────────────────────────────────

type installItem struct {
	entry    browseEntry
	selected bool
}

type installModel struct {
	items     []installItem
	cursor    int
	offset    int
	search    string
	confirmed bool
	quitting  bool
	width     int
	height    int
}

func newInstallModel(entries []browseEntry, initialQuery string) installModel {
	items := make([]installItem, len(entries))
	for i, e := range entries {
		items[i] = installItem{entry: e}
	}
	return installModel{
		items:  items,
		search: initialQuery,
	}
}

func (m installModel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

// ── Update ──────────────────────────────────────────────────────────────────

func (m installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.ensureCursorVisible()

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
				m = m.ensureCursorVisible()
			}
		case "down", "j":
			filtered := m.filteredItems()
			if m.cursor < len(filtered)-1 {
				m.cursor++
				m = m.ensureCursorVisible()
			}
		case "space":
			filtered := m.filteredItems()
			if m.cursor < len(filtered) {
				key := installItemKey(filtered[m.cursor].entry)
				for i := range m.items {
					if installItemKey(m.items[i].entry) == key {
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

func (m installModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	filtered := m.filteredItems()

	title := installHeaderStyle.Render("Install skills")
	total := installCountStyle.Render(fmt.Sprintf("%d skills", len(filtered)))
	b.WriteString(title + "  " + total + "\n")
	b.WriteString(installDivStyle.Render(strings.Repeat("─", 40)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	if len(filtered) == 0 {
		if m.search != "" {
			b.WriteString("\n  No skills matching \"" + m.search + "\"\n")
		} else {
			b.WriteString("\n  No skills available.\n")
		}
	} else {
		maxLines := m.maxContentLines()
		if m.search != "" {
			maxLines--
		}
		linesUsed := 0

		if m.offset > 0 {
			b.WriteString(installDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
			linesUsed++
		}

		lastGroup := ""
		if m.offset > 0 && m.offset < len(filtered) {
			lastGroup = filtered[m.offset].entry.Registry
			b.WriteString("\n" + installHeaderStyle.Render(lastGroup) + "\n")
			linesUsed += 2
		}

		end := m.offset
		for i := m.offset; i < len(filtered); i++ {
			item := filtered[i]
			group := item.entry.Registry

			desc := ""
			if item.entry.Status.Entry != nil {
				desc = item.entry.Status.Entry.Description
			}

			linesNeeded := 1
			if desc != "" {
				linesNeeded = 2
			}
			if group != lastGroup {
				linesNeeded += 2
			}
			if linesUsed+linesNeeded > maxLines {
				break
			}

			if group != lastGroup {
				lastGroup = group
				b.WriteString("\n" + installHeaderStyle.Render(group) + "\n")
				linesUsed += 2
			}

			isCursor := i == m.cursor

			check := "[ ]"
			if item.selected {
				check = installCheckStyle.Render("[x]")
			}

			name := item.entry.Status.Name
			statusBadge := renderStatusBadge(item.entry.Status.Status)

			line := name
			if statusBadge != "" {
				line = name + "  " + statusBadge
			}

			if isCursor {
				b.WriteString(installCursorStyle.Render("▸") + " " + check + " " + installCursorStyle.Render(line) + "\n")
			} else {
				b.WriteString("  " + check + " " + installNameStyle.Render(name))
				if statusBadge != "" {
					b.WriteString("  " + statusBadge)
				}
				b.WriteString("\n")
			}
			linesUsed++

			if desc != "" {
				b.WriteString("       " + installDescStyle.Render(desc) + "\n")
				linesUsed++
			}

			end = i + 1
		}

		remaining := len(filtered) - end
		if remaining > 0 {
			b.WriteString(installDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
		}
	}

	b.WriteString("\n")
	help := "↑↓ navigate · space select · enter install · q quit"
	if n := m.selectedCount(); n > 0 {
		help += fmt.Sprintf("  (%d selected)", n)
	}
	b.WriteString(installDimStyle.Render(help) + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func renderStatusBadge(s sync.Status) string {
	switch s {
	case sync.StatusCurrent:
		return installInstalledStyle.Render("(installed)")
	case sync.StatusOutdated:
		return installOutdatedStyle.Render("(update available)")
	}
	return ""
}

// ── Viewport helpers ────────────────────────────────────────────────────────

func (m installModel) maxContentLines() int {
	if m.height == 0 {
		return 30
	}
	overhead := 5
	avail := m.height - overhead
	if avail < 5 {
		avail = 5
	}
	return avail
}

func (m installModel) ensureCursorVisible() installModel {
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
	return m
}

// ── Data helpers ────────────────────────────────────────────────────────────

func (m installModel) filteredItems() []installItem {
	if m.search == "" {
		return m.items
	}
	var result []installItem
	lower := strings.ToLower(m.search)
	for _, item := range m.items {
		name := strings.ToLower(item.entry.Status.Name)
		desc := ""
		if item.entry.Status.Entry != nil {
			desc = strings.ToLower(item.entry.Status.Entry.Description)
		}
		if strings.Contains(name, lower) || strings.Contains(desc, lower) {
			result = append(result, item)
		}
	}
	return result
}

func (m installModel) selectedCount() int {
	n := 0
	for _, item := range m.items {
		if item.selected {
			n++
		}
	}
	return n
}

func (m installModel) selectedEntries() []browseEntry {
	var selected []browseEntry
	for _, item := range m.items {
		if item.selected {
			selected = append(selected, item.entry)
		}
	}
	return selected
}

// installItemKey produces a unique key per (registry, skill) pair.
func installItemKey(e browseEntry) string {
	return e.Registry + "::" + e.Status.Name
}
