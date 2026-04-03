package cmd

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

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

// ── Phase ───────────────────────────────────────────────────────────────────

type addPhase int

const (
	addPhaseGroups addPhase = iota
	addPhaseSkills
)

// ── Group item ──────────────────────────────────────────────────────────────

type addGroupItem struct {
	name  string
	key   string
	count int
}

// ── Model ───────────────────────────────────────────────────────────────────

type addItem struct {
	candidate add.Candidate
	selected  bool
}

type addModel struct {
	phase      addPhase
	groups     []addGroupItem
	groupKey   string
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

func newAddModel(candidates []add.Candidate, targetRepo string, groupFlag string) addModel {
	items := make([]addItem, len(candidates))
	for i, c := range candidates {
		items[i] = addItem{candidate: c}
	}

	// Build group list.
	counts := map[string]int{}
	for _, c := range candidates {
		g := skillGroup(c)
		counts[g]++
	}

	groups := []addGroupItem{
		{name: "all", key: "", count: len(candidates)},
	}
	// "standalone" first (like uncategorized), then others alphabetically.
	if n, ok := counts["standalone"]; ok {
		groups = append(groups, addGroupItem{name: "standalone", key: "standalone", count: n})
	}
	var pkgs []string
	for k := range counts {
		if k != "standalone" {
			pkgs = append(pkgs, k)
		}
	}
	sort.Strings(pkgs)
	for _, k := range pkgs {
		groups = append(groups, addGroupItem{name: k, key: k, count: counts[k]})
	}

	m := addModel{
		phase:      addPhaseGroups,
		groups:     groups,
		items:      items,
		targetRepo: targetRepo,
	}

	// If --group flag is set, skip to skills phase.
	if groupFlag != "" {
		m.groupKey = groupFlag
		m.phase = addPhaseSkills
	}

	return m
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
		if m.phase == addPhaseGroups {
			return m.updateGroups(msg)
		}
		return m.updateSkills(msg)
	}
	return m, nil
}

func (m addModel) updateGroups(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "escape":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.groups)-1 {
			m.cursor++
		}
	case "enter":
		m.groupKey = m.groups[m.cursor].key
		m.phase = addPhaseSkills
		m.cursor = 0
		m.offset = 0
	}
	return m, nil
}

func (m addModel) updateSkills(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			// Back to groups.
			m.phase = addPhaseGroups
			m.cursor = 0
			m.offset = 0
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
	return m, nil
}

// ── View ────────────────────────────────────────────────────────────────────

func (m addModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var s string
	if m.phase == addPhaseGroups {
		s = m.viewGroups()
	} else {
		s = m.viewSkills()
	}

	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m addModel) viewGroups() string {
	var b strings.Builder

	total := addCountStyle.Render(fmt.Sprintf("%d skills", len(m.items)))
	b.WriteString(addHeaderStyle.Render(fmt.Sprintf("Add skills to %s", m.targetRepo)) + "  " + total + "\n")
	b.WriteString(addDivStyle.Render(strings.Repeat("─", 40)) + "\n\n")

	for i, g := range m.groups {
		isCursor := i == m.cursor
		count := addCountStyle.Render(fmt.Sprintf("(%d)", g.count))

		if isCursor {
			b.WriteString(addCursorStyle.Render("▸") + " " + addCursorStyle.Render(g.name) + " " + count + "\n")
		} else {
			b.WriteString("  " + g.name + " " + count + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(addDimStyle.Render("↑↓ navigate · enter browse · q quit") + "\n")

	return b.String()
}

func (m addModel) viewSkills() string {
	var b strings.Builder
	filtered := m.filteredItems()

	// Header.
	label := m.groupKey
	if label == "" {
		label = "all"
	}
	title := addHeaderStyle.Render(fmt.Sprintf("Add skills to %s", m.targetRepo))
	group := addCountStyle.Render(fmt.Sprintf("%s · %d skills", label, len(filtered)))
	b.WriteString(title + "  " + group + "\n")
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
		width := m.width
		if width == 0 {
			width = 80
		}

		maxLines := m.maxContentLines()
		if m.search != "" {
			maxLines-- // search bar takes a line
		}
		linesUsed := 0

		if m.offset > 0 {
			b.WriteString(addDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
			linesUsed++
		}

		end := m.offset
		for i := m.offset; i < len(filtered); i++ {
			item := filtered[i]

			linesNeeded := 1
			if linesUsed+linesNeeded > maxLines {
				break
			}

			isCursor := i == m.cursor

			check := "[ ]"
			if item.selected {
				check = addCheckStyle.Render("[x]")
			}

			name := item.candidate.Name
			desc := item.candidate.Description

			// Build inline description.
			var descPart string
			if desc != "" {
				prefixLen := 6 // "▸ [x] " or "  [ ] "
				avail := width - prefixLen - runewidth.StringWidth(name) - 4 // 4 for " — "
				if avail > 3 {
					descPart = " — " + addDescStyle.Render(runewidth.Truncate(desc, avail, "…"))
				}
			}

			if isCursor {
				b.WriteString(addCursorStyle.Render("▸") + " " + check + " " + addCursorStyle.Render(name) + descPart + "\n")
			} else {
				b.WriteString("  " + check + " " + addNameStyle.Render(name) + descPart + "\n")
			}
			linesUsed++

			end = i + 1
		}

		remaining := len(filtered) - end
		if remaining > 0 {
			b.WriteString(addDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
		}
	}

	// Footer.
	b.WriteString("\n")
	help := "↑↓ navigate · space select · enter add · esc back · q quit"
	if n := m.selectedCount(); n > 0 {
		help += fmt.Sprintf("  (%d selected)", n)
	}
	b.WriteString(addDimStyle.Render(help) + "\n")

	return b.String()
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
	visible := m.maxContentLines()
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
	if m.groupKey == "" && m.search == "" {
		return m.items
	}
	var result []addItem
	lower := strings.ToLower(m.search)
	for _, item := range m.items {
		// Group filter.
		if m.groupKey != "" {
			g := skillGroup(item.candidate)
			if g != m.groupKey {
				continue
			}
		}
		// Search filter.
		if m.search != "" && !strings.Contains(strings.ToLower(item.candidate.Name), lower) {
			continue
		}
		result = append(result, item)
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
