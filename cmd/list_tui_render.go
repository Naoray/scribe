package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/workflow"
)

var (
	ltNameStyle    = lipgloss.NewStyle().Bold(true)
	ltDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ltHeaderStyle  = lipgloss.NewStyle().Bold(true)
	ltCountStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	ltCursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	ltDivStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ltGroupStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	ltSpinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF"))
	ltUpdateStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	ltRemoveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06060"))
	ltNeutralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
	ltDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa"))
	ltMetaKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(10)
	ltMetaValStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
	ltReasonStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Italic(true)
	ltExcerptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	ltExcerptH1    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F4B942"))
	ltExcerptH2    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DD3FC"))
	ltExcerptCode  = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#F9A8D4"))
	ltExcerptList  = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8C1EC"))
	ltSkeleton     = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	ltPaneStyle    = lipgloss.NewStyle()
	ltErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func tickSpinnerCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickSpinnerMsg{} })
}

func (m listModel) viewLoading() string {
	frame := spinnerFrames[m.spinnerFrame]
	msg := "Loading skills..."
	if m.isBrowseMode() {
		msg = "Loading registry skills..."
	}
	return "\n  " + ltSpinnerStyle.Render(frame) + "  " + ltDimStyle.Render(msg) + "\n"
}

func (m listModel) viewError() string {
	width := m.width
	if width < 40 {
		width = 40
	}
	return "\n  " + ltErrorStyle.Render(wrapText("Error: "+m.err.Error(), width-4)) + "\n"
}

func (m listModel) viewListFull() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString(m.renderQueryLine() + "\n")

	contentHeight := m.contentHeight()
	var rowsBuf strings.Builder
	m.renderRows(&rowsBuf, contentHeight, m.width-4, false)
	content := rowsBuf.String()
	b.WriteString(content)
	for i := strings.Count(content, "\n"); i < contentHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.renderSummary() + "\n")
	if m.backgroundLoad {
		b.WriteString(ltDimStyle.Render(spinnerFrames[m.spinnerFrame]+" checking registry updates in background...") + "\n")
	}
	if m.commandMode {
		b.WriteString(ltDimStyle.Render("Command mode · enter run · esc cancel · backspace delete") + "\n")
	}
	if m.isBrowseMode() {
		b.WriteString(ltDimStyle.Render("↑↓ navigate · /search · enter detail · q quit") + "\n")
	} else {
		b.WriteString(ltDimStyle.Render("↑↓ navigate · /search · :commands · enter detail · q quit") + "\n")
		b.WriteString(ltDimStyle.Render("Commands: :add <query> · :sync · :remove <name> · :help") + "\n")
	}
	return b.String()
}

func (m listModel) viewSplit() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString(m.renderQueryLine() + "\n")

	contentHeight := m.contentHeight()
	leftWidth, rightWidth := m.paneWidths()

	var leftBuf strings.Builder
	m.renderRows(&leftBuf, contentHeight, leftWidth-2, true)
	leftLines := strings.Split(strings.TrimRight(leftBuf.String(), "\n"), "\n")
	for len(leftLines) < contentHeight {
		leftLines = append(leftLines, "")
	}
	leftContent := strings.Join(leftLines[:contentHeight], "\n")

	var rightContent string
	if m.cursor < len(m.filtered) {
		rightContent = m.renderDetailPane(m.filtered[m.cursor], rightWidth)
	}
	rightLines := strings.Split(rightContent, "\n")
	for len(rightLines) < contentHeight {
		rightLines = append(rightLines, "")
	}
	rightContent = strings.Join(rightLines[:contentHeight], "\n")

	leftRendered := ltPaneStyle.Width(leftWidth).Height(contentHeight).Render(leftContent)
	divider := strings.TrimRight(strings.Repeat("│\n", contentHeight), "\n")
	divRendered := ltDivStyle.Height(contentHeight).Render(divider)
	rightRendered := ltPaneStyle.Width(rightWidth).Height(contentHeight).Render(rightContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, divRendered, rightRendered)
	b.WriteString(body)

	b.WriteString("\n\n")
	if m.backgroundLoad {
		b.WriteString(ltDimStyle.Render(spinnerFrames[m.spinnerFrame]+" checking registry updates in background...") + "\n")
	}
	if m.commandMode {
		b.WriteString(ltDimStyle.Render("Command mode · enter run · esc cancel · backspace delete") + "\n")
	}
	switch {
	case m.substate == listSubstateConfirm:
		b.WriteString(ltDimStyle.Render("y confirm · n cancel") + "\n")
	case m.substate == listSubstateUpdateChoice:
		if m.updateHasMods {
			b.WriteString(ltDimStyle.Render("r registry · l local · m merge · esc cancel") + "\n")
		} else {
			b.WriteString(ltDimStyle.Render("u update · esc cancel") + "\n")
		}
	case m.substate == listSubstateTools:
		b.WriteString(ltDimStyle.Render("↑↓ choose · enter toggle/save · esc cancel") + "\n")
	case m.focus == focusList:
		if m.isBrowseMode() {
			b.WriteString(ltDimStyle.Render("↑↓ browse skills · →/enter install · esc close · q quit") + "\n")
		} else {
			b.WriteString(ltDimStyle.Render("↑↓ browse skills · →/enter actions · esc close · q quit") + "\n")
		}
	case m.focus == focusPreview:
		b.WriteString(ltDimStyle.Render("↑↓ scroll preview · ←/tab actions · esc close") + "\n")
	default:
		if m.isBrowseMode() {
			b.WriteString(ltDimStyle.Render("↑↓ choose install · enter run · ←/tab back to list · esc close") + "\n")
		} else {
			b.WriteString(ltDimStyle.Render("↑↓ pick action · ↓ preview · enter run · ←/tab back · esc close") + "\n")
		}
	}
	return b.String()
}

func (m listModel) renderQueryLine() string {
	if m.commandMode {
		if m.commandInput == "" {
			return ltCursorStyle.Render(":") + " "
		}
		return ltCursorStyle.Render(":") + " " + m.commandInput
	}
	if m.searchMode || m.search != "" {
		return "/ " + m.search
	}
	if m.isBrowseMode() {
		return ltDimStyle.Render("/ search registries...")
	}
	return ltDimStyle.Render("/ search...")
}

func (m listModel) renderHeader() string {
	var b strings.Builder
	total := ltCountStyle.Render(fmt.Sprintf("%d skills", len(m.rows)))
	title := "Installed Skills"
	if m.isBrowseMode() {
		title = "Browse Skills"
	}
	b.WriteString(ltHeaderStyle.Render(title) + "  " + total + "\n")
	width := m.width
	if width < 40 {
		width = 40
	}
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width)) + "\n")
	for _, warn := range m.warnings {
		b.WriteString("  " + ltErrorStyle.Render("! "+wrapText(warn, width-4)) + "\n")
	}
	return b.String()
}

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		for runewidth.StringWidth(line) > width {
			cut := width
			for cut > 0 && runewidth.StringWidth(line[:cut]) > width {
				cut--
			}
			if cut <= 0 {
				cut = len(line)
			}
			out.WriteString(line[:cut])
			out.WriteString("\n")
			line = line[cut:]
		}
		out.WriteString(line)
	}
	return out.String()
}

func (m listModel) renderRows(b *strings.Builder, contentHeight, maxWidth int, compact bool) {
	if len(m.filtered) == 0 {
		b.WriteString(ltDimStyle.Render("  (no skills match)") + "\n")
		return
	}

	nameCol := 0
	for _, r := range m.filtered {
		w := runewidth.StringWidth(r.Name)
		if w > nameCol {
			nameCol = w
		}
	}
	statusReserve := 0
	if !compact {
		statusReserve = 42
	} else {
		statusReserve = 4
	}
	maxNameCol := maxWidth - statusReserve - 2
	if maxNameCol < 8 {
		maxNameCol = 8
	}
	if nameCol > maxNameCol {
		nameCol = maxNameCol
	}
	if nameCol < 8 {
		nameCol = 8
	}

	linesUsed := 0
	if m.offset > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
		linesUsed++
	}

	prevGroup := ""
	if m.offset > 0 && m.offset < len(m.filtered) {
		prevGroup = m.filtered[m.offset-1].Group
	}

	end := m.offset
	for i := m.offset; i < len(m.filtered); i++ {
		row := m.filtered[i]
		if linesUsed >= contentHeight {
			break
		}

		header := ""
		headerLines := 0
		if row.Group != prevGroup {
			header = m.formatGroupHeader(row.Group)
			if header != "" {
				headerLines = 1
			}
		}

		remainingAfter := len(m.filtered) - (i + 1)
		needBottomIndicator := remainingAfter > 0
		needed := headerLines + 1
		if needBottomIndicator {
			needed++
		}
		if linesUsed+needed > contentHeight {
			break
		}

		if header != "" {
			b.WriteString(header + "\n")
			linesUsed++
		}
		prevGroup = row.Group
		isCursor := i == m.cursor
		b.WriteString(m.formatRow(row, isCursor, nameCol, compact) + "\n")
		linesUsed++
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 && linesUsed < contentHeight {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}
}

func (m listModel) formatGroupHeader(group string) string {
	if group == "" {
		return ""
	}
	count := m.groupCounts[group]
	return ltGroupStyle.Render(group) + " " + ltCountStyle.Render(fmt.Sprintf("(%d)", count))
}

func (m listModel) formatRow(row listRow, isCursor bool, nameCol int, compact bool) string {
	prefix := "  "
	nameStyle := ltNameStyle
	if isCursor {
		prefix = ltCursorStyle.Render("▸") + " "
		nameStyle = ltCursorStyle
	}

	name := runewidth.Truncate(row.Name, nameCol, "…")
	name = runewidth.FillRight(name, nameCol)

	if compact {
		if !row.HasStatus {
			line := prefix + nameStyle.Render(name)
			if !row.Managed {
				line += " " + ltDimStyle.Render("[unmanaged]")
			}
			return line
		}
		icon := statusStyles[row.Status].Render(row.Status.Display().Icon)
		line := prefix + nameStyle.Render(name) + "  " + icon
		if !row.Managed {
			line += " " + ltDimStyle.Render("[unmanaged]")
		}
		return line
	}

	if !row.HasStatus {
		line := prefix + nameStyle.Render(name)
		if !row.Managed {
			line += " " + ltDimStyle.Render("[unmanaged]")
		} else if m.backgroundLoad && row.Origin == state.OriginRegistry && row.Group != "" {
			ver, author := m.renderSkeletonColumns(row)
			line += "  " + ver + "  " + author
		}
		return line
	}

	ver := row.Version
	if ver == "" {
		ver = "-"
	}
	ver = runewidth.Truncate(ver, 14, "…")
	ver = runewidth.FillRight(ver, 14)

	author := row.Author
	if author == "" {
		author = "-"
	}
	author = runewidth.Truncate(author, 12, "…")
	author = runewidth.FillRight(author, 12)

	line := prefix + nameStyle.Render(name) + "  " + ltDimStyle.Render(ver) + "  " + ltDimStyle.Render(author)

	if row.HasStatus {
		icon := statusStyles[row.Status].Render(row.Status.Display().Icon)
		label := statusStyles[row.Status].Render(row.Status.Display().Label)
		line += "  " + icon + " " + label
	}

	if !row.Managed {
		line += " " + ltDimStyle.Render("[unmanaged]")
	}

	return line
}

func (m listModel) renderSkeletonColumns(row listRow) (string, string) {
	phase := (m.spinnerFrame + skeletonSeed(row.Name)) % 3
	ver := renderSkeletonToken([]int{5, 3}, phase)
	author := renderSkeletonToken([]int{4, 2}, (phase+1)%3)
	return ver, author
}

func renderSkeletonToken(parts []int, phase int) string {
	shades := []string{"░", "▒", "▓"}
	segments := make([]string, 0, len(parts))
	width := 0
	for i, n := range parts {
		fill := shades[(phase+i)%len(shades)]
		segments = append(segments, ltSkeleton.Render(strings.Repeat(fill, n)))
		width += n
	}
	out := strings.Join(segments, " ")
	width += len(parts) - 1
	return runewidth.FillRight(out, width)
}

func skeletonSeed(name string) int {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	return sum % 3
}

func (m listModel) renderSummary() string {
	if len(m.rows) == 0 {
		return ""
	}
	hasStatus := false
	counts := map[sync.Status]int{}
	for _, r := range m.rows {
		if r.HasStatus {
			hasStatus = true
			counts[r.Status]++
		}
	}
	if !hasStatus {
		return ltDimStyle.Render(fmt.Sprintf("%d skills total", len(m.rows)))
	}
	order := []sync.Status{sync.StatusCurrent, sync.StatusModified, sync.StatusOutdated, sync.StatusConflicted, sync.StatusMissing, sync.StatusExtra}
	var parts []string
	for _, s := range order {
		if part := renderStatusCount(s, counts[s]); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ltDimStyle.Render(" · "))
}

func (m listModel) renderDetailPane(row listRow, width int) string {
	var b strings.Builder
	b.WriteString(ltCursorStyle.Render(row.Name) + "\n")

	desc := ""
	switch {
	case row.Local != nil && row.Local.Description != "":
		desc = row.Local.Description
	case row.Entry != nil && row.Entry.Description != "":
		desc = row.Entry.Description
	}
	if desc != "" {
		b.WriteString(ltDescStyle.Width(width-2).Render(desc) + "\n")
	}
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	for _, p := range m.metadataPairsForRow(row) {
		b.WriteString(ltMetaKeyStyle.Render(p.key) + ltMetaValStyle.Render(p.value) + "\n")
	}

	if row.Local != nil && !row.Managed {
		b.WriteString(ltDimStyle.Render("run: scribe adopt "+row.Name) + "\n")
	}

	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	if m.substate == listSubstateTools {
		b.WriteString(m.renderToolsEditor(width))
		return b.String()
	}

	if m.statusMsg != "" {
		b.WriteString(m.statusMsg + "\n")
		switch m.substate {
		case listSubstateUpdateChoice:
			b.WriteString("\n")
			if m.updateHasMods {
				b.WriteString(ltDimStyle.Render("[m] merge with upstream") + "\n")
				b.WriteString(ltDimStyle.Render("[r] replace with registry version") + "\n")
				b.WriteString(ltDimStyle.Render("[l] keep local version") + "\n")
			} else {
				b.WriteString(ltDimStyle.Render("[u] update now") + "\n")
			}
			b.WriteString(ltDimStyle.Render("[esc] cancel") + "\n")
		case listSubstateConfirm:
			b.WriteString("\n")
			b.WriteString(ltDimStyle.Render("[y] confirm remove") + "\n")
			b.WriteString(ltDimStyle.Render("[n] cancel") + "\n")
		}
		return b.String()
	}

	actions := m.actionsForRow(row)
	b.WriteString(m.renderActionGrid(actions, width-2))

	if row.Excerpt != "" {
		b.WriteString(m.renderPreviewSection(row, width-2) + "\n")
	}
	return b.String()
}

// renderActionGrid lays actions out in a 2-column grid (column-major, so the
// flat actionCursor still walks top-to-bottom naturally) when the pane is
// wide enough. Falls back to a single column on narrow panes. Collapsing
// the action list saves vertical room for the preview below.
func (m listModel) renderActionGrid(actions []actionItem, width int) string {
	const cols = 2
	const minColWidth = 18

	var b strings.Builder
	colWidth := width / cols
	if colWidth < minColWidth || len(actions) < 2 {
		for i, a := range actions {
			isCursor := i == m.actionCursor && m.focus == focusActions
			b.WriteString(m.formatActionCell(a, isCursor, 0) + "\n")
		}
		return b.String()
	}

	rowsCount := (len(actions) + cols - 1) / cols
	for r := 0; r < rowsCount; r++ {
		leftIdx := r
		rightIdx := r + rowsCount
		left := m.formatActionCell(actions[leftIdx], leftIdx == m.actionCursor && m.focus == focusActions, colWidth)
		var right string
		if rightIdx < len(actions) {
			right = m.formatActionCell(actions[rightIdx], rightIdx == m.actionCursor && m.focus == focusActions, 0)
		}
		b.WriteString(left + right + "\n")
	}
	if m.focus == focusActions && m.actionCursor >= 0 && m.actionCursor < len(actions) {
		if a := actions[m.actionCursor]; a.disabled && a.reason != "" {
			b.WriteString("  " + ltReasonStyle.Render(a.reason) + "\n")
		}
	}
	return b.String()
}

// formatActionCell renders a single action. When width > 0 the cell is
// right-padded to that display width so the next column aligns. Padding is
// based on the unstyled text length because lipgloss emits zero-width ANSI
// escapes that would inflate rune-width measurements.
func (m listModel) formatActionCell(a actionItem, isCursor bool, width int) string {
	rawPrefix := "  "
	styledPrefix := rawPrefix
	if isCursor {
		styledPrefix = ltCursorStyle.Render("▸") + " "
	}

	rawLabel := a.label
	var styledLabel string
	switch {
	case a.disabled:
		styledLabel = ltDimStyle.Render(rawLabel)
	case isCursor:
		styledLabel = ltCursorStyle.Render(rawLabel)
	default:
		styledLabel = a.style.Render(rawLabel)
	}

	cell := styledPrefix + styledLabel
	used := runewidth.StringWidth(rawPrefix + rawLabel)

	// Only append the reason when rendering single-column (width == 0);
	// grid cells drop it to keep columns narrow.
	if width == 0 && a.disabled && a.reason != "" {
		reason := " " + ltReasonStyle.Render(a.reason)
		cell += reason
		used += runewidth.StringWidth(" " + a.reason)
	}

	if width > 0 && used < width {
		cell += strings.Repeat(" ", width-used)
	}
	return cell
}

func (m listModel) renderPreviewSection(row listRow, width int) string {
	lines := buildExcerptLines(row.Excerpt, width)
	focused := m.focus == focusPreview

	heading := "preview"
	headingStyle := ltDimStyle
	if focused {
		heading = "▸ preview"
		headingStyle = ltCursorStyle
	}

	offset := m.clampedExcerptOffset(lines)

	// Build heading line with counter on the left and the "more above"
	// indicator right-aligned to the pane width. Widths computed on the
	// unstyled text since lipgloss adds zero-width ANSI escapes.
	leftRaw := heading
	styledLeft := headingStyle.Render(heading)
	if len(lines) > 0 && focused {
		counter := fmt.Sprintf("%d/%d", offset+1, len(lines))
		leftRaw += " " + counter
		styledLeft += " " + ltDimStyle.Render(counter)
	}

	var out strings.Builder
	out.WriteString(ltDivStyle.Render(strings.Repeat("─", width)) + "\n")

	if offset > 0 {
		rightRaw := fmt.Sprintf("↑ %d more above", offset)
		gap := width - runewidth.StringWidth(leftRaw) - runewidth.StringWidth(rightRaw)
		if gap < 1 {
			gap = 1
		}
		out.WriteString(styledLeft + strings.Repeat(" ", gap) + ltDimStyle.Render(rightRaw) + "\n")
	} else {
		out.WriteString(styledLeft + "\n")
	}

	// Blank spacer line directly under the preview heading.
	out.WriteString("\n")

	if len(lines) == 0 {
		out.WriteString(ltDimStyle.Render("(empty)"))
		return out.String()
	}

	out.WriteString(strings.Join(lines[offset:], "\n"))
	return out.String()
}

func (m listModel) clampedExcerptOffset(lines []string) int {
	if m.excerptOffset < 0 {
		return 0
	}
	if len(lines) == 0 {
		return 0
	}
	if m.excerptOffset > len(lines)-1 {
		return len(lines) - 1
	}
	return m.excerptOffset
}

type metaPair struct{ key, value string }

// metadataPairsForRow collects the Status/Version/Author/... key-value pairs
// shown in the detail pane header. Declarative table — each row is a single
// call; empty values are dropped by the closure so conditions live at the
// call site as expressions, not nested if-blocks.
func (m listModel) metadataPairsForRow(row listRow) []metaPair {
	pairs := make([]metaPair, 0, 8)
	add := func(key, value string) {
		if value != "" {
			pairs = append(pairs, metaPair{key, value})
		}
	}

	add("Status", statusLabel(row))
	add("Managed", managedLabel(row))
	add("Version", row.Version)
	add("Author", row.Author)
	add("Registry", row.Group)
	add("Source", originLabel(row.Origin))
	if row.Kind == state.KindPackage {
		add("Kind", "package")
		add("Tools", "self-managed")
	} else {
		add("Tools", strings.Join(row.Targets, ", "))
	}
	add("Path", skillPathLabel(row))
	return pairs
}

func statusLabel(row listRow) string {
	if !row.HasStatus {
		return ""
	}
	return row.Status.Display().Label
}

func managedLabel(row listRow) string {
	if row.Local == nil || row.Managed {
		return ""
	}
	return "no"
}

func originLabel(o state.Origin) string {
	if o == state.OriginLocal {
		return "(local)"
	}
	return ""
}

func skillPathLabel(row listRow) string {
	if row.Local == nil || row.Local.LocalPath == "" {
		return ""
	}
	path := row.Local.LocalPath
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func (m listModel) renderToolsEditor(width int) string {
	var b strings.Builder
	effective := "none"
	if names := m.selectedToolNames(); len(names) > 0 {
		effective = strings.Join(names, ", ")
	}
	modeLabel := "inherit"
	if m.toolMode == state.ToolsModePinned {
		modeLabel = "pinned"
	}

	b.WriteString(ltMetaKeyStyle.Render("Mode") + ltMetaValStyle.Render(modeLabel) + "\n")
	b.WriteString(ltMetaKeyStyle.Render("Effective") + ltMetaValStyle.Render(effective) + "\n")
	if m.toolMode != state.ToolsModePinned {
		b.WriteString(ltDimStyle.Render("Toggle a tool to switch this skill to a custom tool set.") + "\n")
	}
	b.WriteString("\n")

	cursorPrefix := func(i int) string {
		if i == m.toolCursor && m.focus == focusActions {
			return ltCursorStyle.Render("▸") + " "
		}
		return "  "
	}

	b.WriteString(cursorPrefix(0) + ltNeutralStyle.Render("mode: toggle inherit/pinned") + "\n")
	for i, st := range m.toolStatuses {
		selected := m.toolSelection[st.Name]
		marker := "[ ]"
		if selected {
			if m.toolMode == state.ToolsModePinned {
				marker = "[x]"
			} else {
				marker = "[~]"
			}
		}
		line := marker + " " + st.Name
		style := ltNeutralStyle
		if available, reason := toolStatusAvailable(st); !available {
			style = ltDimStyle
			line += " " + ltReasonStyle.Render(reason)
		}
		b.WriteString(cursorPrefix(i+1) + style.Render(line) + "\n")
	}

	saveIndex := len(m.toolStatuses) + 1
	cancelIndex := len(m.toolStatuses) + 2
	saveLabel := "save"
	saveStyle := ltUpdateStyle
	if err := m.validateToolsEditor(); err != nil {
		saveLabel += " " + ltReasonStyle.Render(err.Error())
		saveStyle = ltDimStyle
	}
	b.WriteString("\n")
	b.WriteString(cursorPrefix(saveIndex) + saveStyle.Render(saveLabel) + "\n")
	b.WriteString(cursorPrefix(cancelIndex) + ltNeutralStyle.Render("cancel") + "\n")
	if m.statusMsg != "" {
		b.WriteString("\n" + m.statusMsg + "\n")
	}

	return lipgloss.NewStyle().Width(width - 2).Render(strings.TrimRight(b.String(), "\n"))
}

// buildExcerptLines parses the raw SKILL.md excerpt into styled, width-wrapped
// display lines. Each slice entry is one visual line suitable for scrolling.
func buildExcerptLines(excerpt string, width int) []string {
	var lines []string
	prevWasHeading := false
	for _, raw := range strings.Split(excerpt, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			prevWasHeading = false
			continue
		}

		style := ltExcerptStyle
		text := trimmed
		isHeading := false
		switch {
		case strings.HasPrefix(trimmed, "# "):
			style = ltExcerptH1
			text = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			isHeading = true
		case strings.HasPrefix(trimmed, "## "):
			style = ltExcerptH2
			text = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			isHeading = true
		case strings.HasPrefix(trimmed, "### "):
			style = ltExcerptH2
			text = strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			isHeading = true
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			style = ltExcerptList
			text = "• " + workflow.NormalizeExcerptLine(trimmed)
		case isNumberedListLine(trimmed):
			style = ltExcerptList
			text = trimmed
		default:
			text = workflow.NormalizeExcerptLine(trimmed)
		}

		if text == "" {
			continue
		}
		for _, wrapped := range wrapExcerptLine(text, width) {
			lines = append(lines, renderInlineCode(style, wrapped))
		}
		if isHeading {
			lines = append(lines, "")
		} else if prevWasHeading {
			lines = append(lines, "")
		}
		prevWasHeading = isHeading
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// wrapExcerptLine splits a single plain-text line into visual lines that fit
// within the supplied cell width. Cuts on rune boundaries so multi-byte
// characters (emoji, CJK, accents) stay intact.
func wrapExcerptLine(text string, width int) []string {
	if width <= 0 || runewidth.StringWidth(text) <= width {
		return []string{text}
	}
	var out []string
	var segment strings.Builder
	segWidth := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if segWidth+rw > width && segment.Len() > 0 {
			out = append(out, segment.String())
			segment.Reset()
			segWidth = 0
		}
		segment.WriteRune(r)
		segWidth += rw
	}
	if segment.Len() > 0 {
		out = append(out, segment.String())
	}
	return out
}

func renderInlineCode(base lipgloss.Style, text string) string {
	parts := strings.Split(text, "`")
	if len(parts) == 1 {
		return base.Render(text)
	}
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString(ltExcerptCode.Render(part))
		} else if part != "" {
			b.WriteString(base.Render(part))
		}
	}
	return b.String()
}

func isNumberedListLine(text string) bool {
	if len(text) < 3 {
		return false
	}
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(text) && text[i] == '.' && text[i+1] == ' '
}

func (m listModel) contentHeight() int {
	if m.height == 0 {
		return 20
	}
	chrome := 0
	if m.selected {
		chrome = 6
	} else if m.isBrowseMode() {
		chrome = 6
	} else {
		chrome = 7
	}
	if m.backgroundLoad {
		chrome++
	}
	if m.commandMode {
		chrome++
	}
	h := m.height - chrome
	if h < 5 {
		h = 5
	}
	return h
}

func (m listModel) paneWidths() (int, int) {
	left := m.width * 45 / 100
	if maxDynamic := m.width - 40; left > maxDynamic {
		left = maxDynamic
	}
	if left > 60 {
		left = 60
	}
	if left < 28 {
		left = 28
	}
	right := m.width - left - 1
	if right < 30 {
		right = 30
		left = m.width - right - 1
	}
	return left, right
}
