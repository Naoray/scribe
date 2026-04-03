package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/discovery"
)

// ── Phase ───────────────────────────────────────────────────────────────────

type listPhase int

const (
	listPhaseGroups listPhase = iota
	listPhaseSkills
)

// ── Styles ──────────────────────────────────────────────────────────────────

var (
	ltNameStyle   = lipgloss.NewStyle().Bold(true)
	ltDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	ltDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ltHeaderStyle = lipgloss.NewStyle().Bold(true)
	ltCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	ltCursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	ltDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// ── Group item ──────────────────────────────────────────────────────────────

type listGroupItem struct {
	name  string
	key   string // "" for "all"
	count int
}

// ── Model ───────────────────────────────────────────────────────────────────

type listModel struct {
	phase    listPhase
	groups   []listGroupItem
	skills   []discovery.Skill
	filtered []discovery.Skill
	groupKey string // active group filter
	search   string
	cursor   int
	offset   int
	quitting bool
	width    int
	height   int
}

func newListModel(skills []discovery.Skill, groupFlag string) listModel {
	// Build group list.
	counts := map[string]int{}
	for _, sk := range skills {
		g := sk.Package
		if g == "" {
			g = "uncategorized"
		}
		counts[g]++
	}

	groups := []listGroupItem{
		{name: "all", key: "", count: len(skills)},
	}
	// Uncategorized first, then packages alphabetically.
	if n, ok := counts["uncategorized"]; ok {
		groups = append(groups, listGroupItem{name: "uncategorized", key: "uncategorized", count: n})
	}
	var pkgs []string
	for k := range counts {
		if k != "uncategorized" {
			pkgs = append(pkgs, k)
		}
	}
	sort.Strings(pkgs)
	for _, k := range pkgs {
		groups = append(groups, listGroupItem{name: k, key: k, count: counts[k]})
	}

	m := listModel{
		phase:  listPhaseGroups,
		groups: groups,
		skills: skills,
	}

	// If --group flag is set, skip to skills phase.
	if groupFlag != "" {
		m.groupKey = groupFlag
		m.filtered = m.filterSkills()
		m.phase = listPhaseSkills
	}

	return m
}

func (m listModel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

// ── Update ──────────────────────────────────────────────────────────────────

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		if m.phase == listPhaseGroups {
			return m.updateGroups(msg)
		}
		return m.updateSkills(msg)
	}
	return m, nil
}

func (m listModel) updateGroups(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.filtered = m.filterSkills()
		m.phase = listPhaseSkills
		m.cursor = 0
		m.offset = 0
	}
	return m, nil
}

func (m listModel) updateSkills(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if m.search != "" {
			m.search = ""
			m.filtered = m.filterSkills()
			m.cursor = 0
			m.offset = 0
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "escape":
		if m.search != "" {
			m.search = ""
			m.filtered = m.filterSkills()
			m.cursor = 0
			m.offset = 0
		} else {
			// Back to groups.
			m.phase = listPhaseGroups
			m.cursor = 0
			m.offset = 0
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
	case "home":
		m.cursor = 0
		m.ensureCursorVisible()
	case "end":
		m.cursor = len(m.filtered) - 1
		m.ensureCursorVisible()
	case "backspace":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.filtered = m.filterSkills()
			m.cursor = 0
			m.offset = 0
		}
	default:
		if len(msg.String()) == 1 {
			m.search += msg.String()
			m.filtered = m.filterSkills()
			m.cursor = 0
			m.offset = 0
		}
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────────────────

func (m listModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var s string
	if m.phase == listPhaseGroups {
		s = m.viewGroups()
	} else {
		s = m.viewSkills()
	}

	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m listModel) viewGroups() string {
	var b strings.Builder

	total := ltCountStyle.Render(fmt.Sprintf("%d skills", len(m.skills)))
	b.WriteString(ltHeaderStyle.Render("Installed Skills") + "  " + total + "\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", 40)) + "\n\n")

	for i, g := range m.groups {
		isCursor := i == m.cursor
		count := ltCountStyle.Render(fmt.Sprintf("(%d)", g.count))

		if isCursor {
			b.WriteString(ltCursorStyle.Render("▸") + " " + ltCursorStyle.Render(g.name) + " " + count + "\n")
		} else {
			b.WriteString("  " + g.name + " " + count + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · enter browse · q quit") + "\n")

	return b.String()
}

func (m listModel) viewSkills() string {
	if m.width < 80 {
		return m.viewSkillsSingleColumn()
	}
	return m.viewSkillsSplitPane()
}

func (m listModel) viewSkillsSingleColumn() string {
	var b strings.Builder

	label := m.groupKey
	if label == "" {
		label = "all"
	}
	title := ltHeaderStyle.Render("Installed Skills")
	group := ltCountStyle.Render(fmt.Sprintf("%s · %d skills", label, len(m.filtered)))
	b.WriteString(title + "  " + group + "\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", 40)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	contentHeight := m.contentHeight()
	linesUsed := 0

	if m.offset > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
		linesUsed++
	}

	end := m.offset
	for i := m.offset; i < len(m.filtered); i++ {
		if linesUsed >= contentHeight {
			break
		}
		sk := m.filtered[i]
		isCursor := i == m.cursor

		line := m.formatSkillLine(sk, isCursor, m.width-4)
		b.WriteString(line + "\n")
		linesUsed++
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · type to search · esc back · q quit") + "\n")
	return b.String()
}

func (m listModel) viewSkillsSplitPane() string {
	var b strings.Builder

	// Header.
	label := m.groupKey
	if label == "" {
		label = "all"
	}
	title := ltHeaderStyle.Render("Installed Skills")
	group := ltCountStyle.Render(fmt.Sprintf("%s · %d skills", label, len(m.filtered)))
	b.WriteString(title + "  " + group + "\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", m.width)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	contentHeight := m.contentHeight()
	leftWidth, rightWidth := m.paneWidths()

	// Left pane: skill list.
	var leftLines []string
	if m.offset > 0 {
		leftLines = append(leftLines, ltDimStyle.Render(fmt.Sprintf("  ↑ %d more", m.offset)))
	}

	end := m.offset
	maxItems := contentHeight
	if m.offset > 0 {
		maxItems-- // scroll indicator takes a line
	}

	for i := m.offset; i < len(m.filtered) && len(leftLines) < maxItems; i++ {
		sk := m.filtered[i]
		isCursor := i == m.cursor
		leftLines = append(leftLines, m.formatSkillLine(sk, isCursor, leftWidth-2))
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		leftLines = append(leftLines, ltDimStyle.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	}

	// Pad left pane to contentHeight.
	for len(leftLines) < contentHeight {
		leftLines = append(leftLines, "")
	}
	leftContent := strings.Join(leftLines[:contentHeight], "\n")

	// Right pane: detail for cursor skill.
	rightContent := ""
	if m.cursor < len(m.filtered) {
		rightContent = m.renderDetail(m.filtered[m.cursor], rightWidth)
	}

	// Pad right pane to contentHeight.
	rightLines := strings.Split(rightContent, "\n")
	for len(rightLines) < contentHeight {
		rightLines = append(rightLines, "")
	}
	rightContent = strings.Join(rightLines[:contentHeight], "\n")

	// Join panes.
	leftRendered := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight).Render(leftContent)
	divider := strings.TrimRight(strings.Repeat("│\n", contentHeight), "\n")
	divRendered := lipgloss.NewStyle().Height(contentHeight).Foreground(lipgloss.Color("#555555")).Render(divider)
	rightRendered := lipgloss.NewStyle().Width(rightWidth).Height(contentHeight).Render(rightContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, divRendered, rightRendered)
	b.WriteString(body)

	// Footer.
	b.WriteString("\n\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · enter actions · type to search · esc back · q quit") + "\n")
	return b.String()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func (m listModel) contentHeight() int {
	if m.height == 0 {
		return 20
	}
	headerHeight := 2 // title + divider
	searchHeight := 0
	if m.search != "" {
		searchHeight = 1
	}
	footerHeight := 2 // blank + help
	h := m.height - headerHeight - searchHeight - footerHeight
	if h < 5 {
		h = 5
	}
	return h
}

func (m *listModel) ensureCursorVisible() {
	visible := m.contentHeight()
	if visible < 5 {
		visible = 5
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m listModel) paneWidths() (int, int) {
	left := m.width * 45 / 100
	if maxDynamic := m.width - 40; left > maxDynamic {
		left = maxDynamic
	}
	if left > 60 {
		left = 60
	}
	if left < 20 {
		left = 20
	}
	right := m.width - left - 3 // 3 for divider + padding
	if right < 20 {
		right = 20
	}
	return left, right
}

func (m listModel) formatSkillLine(sk discovery.Skill, isCursor bool, maxWidth int) string {
	prefix := "  "
	nameStyle := ltNameStyle
	descStyle := ltDescStyle
	if isCursor {
		prefix = ltCursorStyle.Render("▸") + " "
		nameStyle = ltCursorStyle
		descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0088aa"))
	}

	name := sk.Name
	// Calculate remaining space for description.
	// prefix (2) + name + " — " (3) = overhead
	descSpace := maxWidth - runewidth.StringWidth(name) - 5
	if sk.Description != "" && descSpace > 10 {
		desc := runewidth.Truncate(sk.Description, descSpace, "...")
		return prefix + nameStyle.Render(name) + " " + descStyle.Render("— "+desc)
	}
	return prefix + nameStyle.Render(name)
}

func (m listModel) renderDetail(sk discovery.Skill, width int) string {
	var b strings.Builder

	b.WriteString(ltCursorStyle.Render(sk.Name) + "\n")

	if sk.Description != "" {
		descStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("#aaaaaa"))
		b.WriteString(descStyle.Render(sk.Description) + "\n")
	}

	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	type kv struct{ key, val string }
	var pairs []kv

	if sk.Version != "" {
		pairs = append(pairs, kv{"Version", sk.Version})
	}
	if sk.ContentHash != "" {
		pairs = append(pairs, kv{"Hash", sk.ContentHash})
	}
	if sk.Package != "" {
		pairs = append(pairs, kv{"Package", sk.Package})
	}
	if sk.Source != "" {
		pairs = append(pairs, kv{"Source", sk.Source})
	}
	if len(sk.Targets) > 0 {
		pairs = append(pairs, kv{"Targets", strings.Join(sk.Targets, ", ")})
	}
	if sk.LocalPath != "" {
		path := sk.LocalPath
		if home, err := os.UserHomeDir(); err == nil {
			path = strings.Replace(path, home, "~", 1)
		}
		pairs = append(pairs, kv{"Path", path})
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(10)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))

	for _, p := range pairs {
		b.WriteString(keyStyle.Render(p.key) + valStyle.Render(p.val) + "\n")
	}

	return b.String()
}

func (m listModel) filterSkills() []discovery.Skill {
	var result []discovery.Skill
	lower := strings.ToLower(m.search)

	for _, sk := range m.skills {
		// Group filter.
		if m.groupKey != "" {
			g := sk.Package
			if g == "" {
				g = "uncategorized"
			}
			if g != m.groupKey {
				continue
			}
		}
		// Search filter.
		if m.search != "" {
			if !strings.Contains(strings.ToLower(sk.Name), lower) &&
				!strings.Contains(strings.ToLower(sk.Description), lower) {
				continue
			}
		}
		result = append(result, sk)
	}

	if m.groupKey == "" && m.search == "" {
		return m.skills
	}
	return result
}
