package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

// ── Stages / substates ──────────────────────────────────────────────────────

type listStage int

const (
	stageLoading listStage = iota
	stageBrowse
)

type listSubstate int

const (
	listSubstateNone listSubstate = iota
	listSubstateConfirm
)

// detailFocus indicates which pane has keyboard focus while the split-screen
// detail view is open. The user can toggle between them with tab/←/→.
type detailFocus int

const (
	focusList detailFocus = iota
	focusActions
)

// ── Styles ──────────────────────────────────────────────────────────────────

var (
	ltNameStyle    = lipgloss.NewStyle().Bold(true)
	ltDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ltHeaderStyle  = lipgloss.NewStyle().Bold(true)
	ltCountStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	ltCursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	ltDivStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ltGroupStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	ltSpinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF"))
)

// ── Row data ────────────────────────────────────────────────────────────────

// listRow is the unified display unit consumed by the TUI. It flattens
// either a sync.SkillStatus (registry mode) or a discovery.Skill (local-only
// mode) into a single shape so the view layer never branches on data source.
type listRow struct {
	Name      string
	Group     string // registry name (e.g. "owner/repo") or package name in local mode
	Status    sync.Status
	HasStatus bool // true in registry mode, false in local-only mode
	Version   string
	Author    string
	Source    string
	Targets   []string
	Local     *discovery.Skill // populated when the skill exists on disk
}

// ── Action items ───────────────────────────────────────────────────────────

type actionItem struct {
	label    string
	key      string
	disabled bool
	reason   string
	style    lipgloss.Style
}

func actionsForRow(row listRow) []actionItem {
	hasLocal := row.Local != nil && row.Local.LocalPath != ""
	return []actionItem{
		{label: "update", key: "update", disabled: true, reason: "source unknown", style: ltDimStyle},
		{label: "remove", key: "remove", disabled: !hasLocal, reason: "not on disk", style: lipgloss.NewStyle().Foreground(lipgloss.Color("#e06060"))},
		{label: "add to category", key: "category", disabled: true, reason: "coming soon", style: ltDimStyle},
		{label: "copy path", key: "copy", disabled: !hasLocal, reason: "not on disk", style: lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))},
		{label: "open in $EDITOR", key: "edit", disabled: !hasLocal, reason: "not on disk", style: lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))},
	}
}

// ── Messages ───────────────────────────────────────────────────────────────

type tickSpinnerMsg struct{}
type rowsLoadedMsg struct{ rows []listRow }
type loadErrMsg struct{ err error }
type clipboardTickMsg struct{ id int }
type editorDoneMsg struct{ err error }

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func tickSpinnerCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickSpinnerMsg{} })
}

// ── Model ───────────────────────────────────────────────────────────────────

type listModel struct {
	stage         listStage
	spinnerFrame  int
	rows          []listRow
	filtered      []listRow
	cursor        int
	offset        int
	search        string
	selected      bool        // true when right-side detail/action pane is open
	focus         detailFocus // which pane is focused while selected
	actionCursor  int
	substate      listSubstate
	statusMsg     string
	pendingTickID int

	ctx context.Context
	bag *workflow.Bag

	width  int
	height int

	err      error
	quitting bool
}

func newListModel(ctx context.Context, bag *workflow.Bag) listModel {
	return listModel{
		stage: stageLoading,
		ctx:   ctx,
		bag:   bag,
	}
}

func (m listModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestWindowSize,
		tickSpinnerCmd(),
		loadRowsCmd(m.ctx, m.bag),
	)
}

// ── Loader ──────────────────────────────────────────────────────────────────

func loadRowsCmd(ctx context.Context, bag *workflow.Bag) tea.Cmd {
	return func() tea.Msg {
		rows, err := buildRows(ctx, bag)
		if err != nil {
			return loadErrMsg{err: err}
		}
		return rowsLoadedMsg{rows: rows}
	}
}

func buildRows(ctx context.Context, bag *workflow.Bag) ([]listRow, error) {
	// Always discover local skills — we need them to enable copy/edit/remove
	// actions on registry rows that happen to be installed.
	localSkills, err := discovery.OnDisk(bag.State)
	if err != nil {
		return nil, err
	}
	localByName := make(map[string]*discovery.Skill, len(localSkills))
	for i := range localSkills {
		sk := &localSkills[i]
		localByName[sk.Name] = sk
	}

	repos := bag.Config.TeamRepos()

	// Local-only mode: no team registries connected.
	if len(repos) == 0 {
		return buildLocalRows(localSkills), nil
	}

	// Registry mode: filter, then diff per repo.
	if bag.FilterRegistries != nil {
		filtered, ferr := bag.FilterRegistries(bag.RepoFlag, repos)
		if ferr != nil {
			return nil, ferr
		}
		repos = filtered
	}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(bag.Client),
		Provider: bag.Provider,
		Tools:    []tools.Tool{},
	}

	// matchedLocal records which locally-discovered skills have already
	// been represented by a registry row, so we can append the rest as
	// untracked rows below.
	matchedLocal := make(map[string]bool, len(localSkills))

	var rows []listRow
	for _, repo := range repos {
		statuses, _, derr := syncer.Diff(ctx, repo, bag.State)
		if derr != nil {
			return nil, fmt.Errorf("%s: %w", repo, derr)
		}
		slug := tools.SlugifyRegistry(repo)
		for _, ss := range statuses {
			// A local skill may be discovered under either its slug-qualified
			// name (e.g. "Artistfy-hq/ascii" if it lives at
			// ~/.scribe/skills/Artistfy-hq/ascii) or its bare name
			// (e.g. "ascii" if installed at ~/.claude/skills/ascii). Prefer
			// the slug-qualified form so a same-named top-level skill from a
			// different source can't be mis-attributed to this registry row.
			// Only mark the matched key so unrelated same-named skills aren't
			// suppressed from the untracked-pass below.
			qualifiedKey := slug + "/" + ss.Name
			local := localByName[qualifiedKey]
			if local != nil {
				matchedLocal[qualifiedKey] = true
			} else if bare := localByName[ss.Name]; bare != nil {
				local = bare
				matchedLocal[ss.Name] = true
			}
			row := listRow{
				Name:      ss.Name,
				Group:     repo,
				Status:    ss.Status,
				HasStatus: true,
				Version:   ss.DisplayVersion(),
				Author:    ss.Maintainer,
				Local:     local,
			}
			if ss.Installed != nil {
				row.Targets = ss.Installed.Tools
				row.Source = ss.Installed.Source
			}
			rows = append(rows, row)
		}
	}

	// Append every local skill that didn't surface through any team registry.
	// These show up under their package (or "uncategorized") with no status
	// column — they're outside the team-registry concept entirely.
	rows = append(rows, buildLocalRowsExcluding(localSkills, matchedLocal)...)
	return rows, nil
}

// buildLocalRowsExcluding returns local rows for every skill whose name is
// not present in the matched set, grouped/sorted the same way as the
// local-only fallback view.
func buildLocalRowsExcluding(skills []discovery.Skill, matched map[string]bool) []listRow {
	var remaining []discovery.Skill
	for _, sk := range skills {
		if matched[sk.Name] {
			continue
		}
		remaining = append(remaining, sk)
	}
	return buildLocalRows(remaining)
}

func buildLocalRows(skills []discovery.Skill) []listRow {
	groups := map[string][]listRow{}
	for i := range skills {
		sk := &skills[i]
		g := sk.Package
		if g == "" {
			g = "uncategorized"
		}
		groups[g] = append(groups[g], listRow{
			Name:    sk.Name,
			Group:   g,
			Version: sk.Version,
			Source:  sk.Source,
			Targets: sk.Targets,
			Local:   sk,
		})
	}

	// Sort: "uncategorized" first, then alphabetical group names; rows
	// within a group sorted by name.
	var keys []string
	for k := range groups {
		if k != "uncategorized" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var ordered []string
	if _, ok := groups["uncategorized"]; ok {
		ordered = append(ordered, "uncategorized")
	}
	ordered = append(ordered, keys...)

	var rows []listRow
	for _, g := range ordered {
		gs := groups[g]
		sort.SliceStable(gs, func(i, j int) bool { return gs[i].Name < gs[j].Name })
		rows = append(rows, gs...)
	}
	return rows
}

// ── Update ──────────────────────────────────────────────────────────────────

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.ensureCursorVisible()
		return m, nil
	case tea.InterruptMsg:
		m.quitting = true
		return m, tea.Quit
	case tickSpinnerMsg:
		if m.stage == stageLoading {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, tickSpinnerCmd()
		}
		return m, nil
	case rowsLoadedMsg:
		m.stage = stageBrowse
		m.rows = msg.rows
		m.filtered = m.applyFilter()
		return m, nil
	case loadErrMsg:
		m.stage = stageBrowse
		m.err = msg.err
		return m, nil
	case clipboardTickMsg:
		if msg.id == m.pendingTickID {
			m.statusMsg = ""
		}
		return m, nil
	case editorDoneMsg:
		if msg.err != nil {
			m.statusMsg = "Editor exited with error"
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.stage == stageLoading {
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.selected {
			return m.updateDetail(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m listModel) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if m.search != "" {
			m.search = ""
			m.filtered = m.applyFilter()
			m.cursor, m.offset = 0, 0
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "esc", "escape":
		if m.search != "" {
			m.search = ""
			m.filtered = m.applyFilter()
			m.cursor, m.offset = 0, 0
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m = m.ensureCursorVisible()
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m = m.ensureCursorVisible()
		}
	case "home":
		m.cursor = 0
		m = m.ensureCursorVisible()
	case "end":
		m.cursor = len(m.filtered) - 1
		m = m.ensureCursorVisible()
	case "enter":
		if len(m.filtered) > 0 {
			m.selected = true
			m.focus = focusActions
			m.actionCursor = 0
			m.statusMsg = ""
		}
	case "backspace":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.filtered = m.applyFilter()
			m.cursor, m.offset = 0, 0
		}
	default:
		if len(msg.String()) == 1 {
			m.search += msg.String()
			m.filtered = m.applyFilter()
			m.cursor, m.offset = 0, 0
		}
	}
	return m, nil
}

func (m listModel) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.substate == listSubstateConfirm {
		return m.updateConfirm(msg)
	}

	if m.cursor >= len(m.filtered) {
		m.selected = false
		m.focus = focusList
		return m, nil
	}

	key := msg.String()

	// Global keys regardless of focus.
	switch key {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "escape":
		m.selected = false
		m.focus = focusList
		m.actionCursor = 0
		m.statusMsg = ""
		return m, nil
	case "tab":
		if m.focus == focusList {
			m.focus = focusActions
			m.actionCursor = 0
		} else {
			m.focus = focusList
		}
		return m, nil
	case "shift+tab":
		if m.focus == focusActions {
			m.focus = focusList
		} else {
			m.focus = focusActions
			m.actionCursor = 0
		}
		return m, nil
	}

	if m.focus == focusList {
		// Browsing the list with the detail pane open: arrow keys move
		// the row cursor and the right pane refreshes live. Right/enter
		// hands focus to the action menu.
		switch key {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m = m.ensureCursorVisible()
				m.actionCursor = 0
				m.statusMsg = ""
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m = m.ensureCursorVisible()
				m.actionCursor = 0
				m.statusMsg = ""
			}
		case "right", "l", "enter":
			m.focus = focusActions
			m.actionCursor = 0
		}
		return m, nil
	}

	// focusActions: arrow keys move within the action list, left returns
	// focus to the row list, enter executes.
	actions := actionsForRow(m.filtered[m.cursor])
	switch key {
	case "up", "k":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down", "j":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		}
	case "left", "h":
		m.focus = focusList
	case "enter":
		action := actions[m.actionCursor]
		if action.disabled {
			return m, nil
		}
		return m.executeAction(action.key)
	}
	return m, nil
}

func (m listModel) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		return m.executeRemove()
	case "n", "esc", "escape":
		m.substate = listSubstateNone
		m.statusMsg = ""
	}
	return m, nil
}

func (m listModel) executeAction(key string) (tea.Model, tea.Cmd) {
	row := m.filtered[m.cursor]
	if row.Local == nil {
		return m, nil
	}
	sk := row.Local
	switch key {
	case "copy":
		m.statusMsg = "Copied!"
		m.pendingTickID++
		tickID := m.pendingTickID
		return m, tea.Batch(
			tea.SetClipboard(sk.LocalPath),
			tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return clipboardTickMsg{id: tickID}
			}),
		)
	case "edit":
		editor := resolveEditor()
		skillMD := filepath.Join(sk.LocalPath, "SKILL.md")
		c := exec.Command(editor, skillMD)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return editorDoneMsg{err: err}
		})
	case "remove":
		m.substate = listSubstateConfirm
		m.statusMsg = fmt.Sprintf("Remove %s? (y/n)", sk.Name)
		return m, nil
	}
	return m, nil
}

func (m listModel) executeRemove() (tea.Model, tea.Cmd) {
	row := m.filtered[m.cursor]
	if row.Local == nil {
		m.substate = listSubstateNone
		return m, nil
	}
	sk := row.Local

	home, _ := os.UserHomeDir()
	allowedPrefixes := []string{
		filepath.Join(home, ".scribe", "skills"),
		filepath.Join(home, ".claude", "skills"),
	}

	pathAllowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(sk.LocalPath, prefix+string(filepath.Separator)) {
			pathAllowed = true
			break
		}
	}

	if sk.LocalPath != "" && !pathAllowed {
		m.statusMsg = "Cannot remove: path outside managed directories"
		m.substate = listSubstateNone
		return m, nil
	}

	m.bag.State.Remove(sk.Name)
	if err := m.bag.State.Save(); err != nil {
		m.statusMsg = fmt.Sprintf("Save failed: %v", err)
		m.substate = listSubstateNone
		return m, nil
	}

	if sk.LocalPath != "" {
		info, err := os.Lstat(sk.LocalPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				err = os.Remove(sk.LocalPath)
			} else {
				err = os.RemoveAll(sk.LocalPath)
			}
			if err != nil {
				m.statusMsg = fmt.Sprintf("Files may remain on disk: %v", err)
			}
		}
	}

	// Drop the row from both filtered and rows.
	m.filtered = append(m.filtered[:m.cursor], m.filtered[m.cursor+1:]...)
	for i, r := range m.rows {
		if r.Name == sk.Name && r.Group == row.Group {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			break
		}
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	m.selected = false
	m.substate = listSubstateNone
	m.actionCursor = 0
	m.statusMsg = ""
	return m, nil
}

// ── Filter ──────────────────────────────────────────────────────────────────

func (m listModel) applyFilter() []listRow {
	if m.search == "" {
		return m.rows
	}
	lower := strings.ToLower(m.search)
	var out []listRow
	for _, r := range m.rows {
		if strings.Contains(strings.ToLower(r.Name), lower) ||
			strings.Contains(strings.ToLower(r.Group), lower) {
			out = append(out, r)
		}
	}
	return out
}

// ── View ────────────────────────────────────────────────────────────────────

func (m listModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var s string
	switch m.stage {
	case stageLoading:
		s = m.viewLoading()
	case stageBrowse:
		if m.err != nil {
			s = m.viewError()
		} else if m.selected {
			s = m.viewSplit()
		} else {
			s = m.viewListFull()
		}
	}

	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m listModel) viewLoading() string {
	frame := spinnerFrames[m.spinnerFrame]
	msg := "Fetching team skills…"
	if m.bag != nil && m.bag.Config != nil && len(m.bag.Config.TeamRepos()) == 0 {
		msg = "Discovering local skills…"
	}
	return "\n  " + ltSpinnerStyle.Render(frame) + "  " + ltDimStyle.Render(msg) + "\n"
}

func (m listModel) viewError() string {
	return "\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("Error: "+m.err.Error()) + "\n"
}

// viewListFull is the default browse view: full-width list with status icons
// and a status-count footer.
func (m listModel) viewListFull() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	contentHeight := m.contentHeight()
	m.renderRows(&b, contentHeight, m.width-4, false)

	b.WriteString("\n")
	b.WriteString(m.renderSummary() + "\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · enter detail · type to search · q quit") + "\n")
	return b.String()
}

// viewSplit is the detail view: compact list on the left, detail + action
// menu on the right. Triggered by pressing Enter on a row.
func (m listModel) viewSplit() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

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

	leftRendered := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight).Render(leftContent)
	divider := strings.TrimRight(strings.Repeat("│\n", contentHeight), "\n")
	divRendered := lipgloss.NewStyle().Height(contentHeight).Foreground(lipgloss.Color("#555555")).Render(divider)
	rightRendered := lipgloss.NewStyle().Width(rightWidth).Height(contentHeight).Render(rightContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, divRendered, rightRendered)
	b.WriteString(body)

	b.WriteString("\n\n")
	switch {
	case m.substate == listSubstateConfirm:
		b.WriteString(ltDimStyle.Render("y confirm · n cancel") + "\n")
	case m.focus == focusList:
		b.WriteString(ltDimStyle.Render("↑↓ browse skills · →/enter actions · esc close · q quit") + "\n")
	default:
		b.WriteString(ltDimStyle.Render("↑↓ pick action · enter run · ←/tab back to list · esc close") + "\n")
	}
	return b.String()
}

// renderHeader prints the title row "Installed Skills · N skills".
func (m listModel) renderHeader() string {
	var b strings.Builder
	total := ltCountStyle.Render(fmt.Sprintf("%d skills", len(m.rows)))
	b.WriteString(ltHeaderStyle.Render("Installed Skills") + "  " + total + "\n")
	width := m.width
	if width < 40 {
		width = 40
	}
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width)) + "\n")
	return b.String()
}

// renderRows writes up to contentHeight lines (including group headers and
// scroll indicators) into b, computing offset from m.cursor and m.offset.
// When compact=true, only the name + status icon is rendered (used by the
// split-pane left column).
func (m listModel) renderRows(b *strings.Builder, contentHeight, maxWidth int, compact bool) {
	if len(m.filtered) == 0 {
		b.WriteString(ltDimStyle.Render("  (no skills match)") + "\n")
		return
	}

	// Pre-compute the max name width across all filtered rows so the status
	// column lines up neatly two cells after the longest name. Capped so a
	// single very long name can't push the status off-screen.
	nameCol := 0
	for _, r := range m.filtered {
		w := runewidth.StringWidth(r.Name)
		if w > nameCol {
			nameCol = w
		}
	}
	// Reserve cells for the status text: icon (1) + space (1) + longest
	// label "current" (7) = 9 cells, plus prefix (2) and a little breathing
	// room.
	statusReserve := 0
	if !compact {
		statusReserve = 12
	} else {
		statusReserve = 4 // icon + padding only
	}
	maxNameCol := maxWidth - statusReserve - 2 // -2 for the row prefix
	if maxNameCol < 8 {
		maxNameCol = 8
	}
	if nameCol > maxNameCol {
		nameCol = maxNameCol
	}
	if nameCol < 8 {
		nameCol = 8
	}

	// Determine which rows are visible based on offset/contentHeight.
	linesUsed := 0
	if m.offset > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
		linesUsed++
	}

	prevGroup := ""
	// Look backwards to find what group the offset row belongs to so we can
	// emit the group header even if the cursor scrolled past the original.
	if m.offset > 0 && m.offset < len(m.filtered) {
		prevGroup = m.filtered[m.offset-1].Group
	}

	end := m.offset
	for i := m.offset; i < len(m.filtered); i++ {
		if linesUsed >= contentHeight {
			break
		}
		row := m.filtered[i]
		if row.Group != prevGroup {
			// Reserve a line for group header. Skip if not enough room.
			if linesUsed+1 >= contentHeight {
				break
			}
			b.WriteString(m.formatGroupHeader(row.Group) + "\n")
			linesUsed++
			prevGroup = row.Group
		}
		isCursor := i == m.cursor
		b.WriteString(m.formatRow(row, isCursor, nameCol, compact) + "\n")
		linesUsed++
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}
}

func (m listModel) formatGroupHeader(group string) string {
	count := 0
	for _, r := range m.filtered {
		if r.Group == group {
			count++
		}
	}
	label := group
	if label == "" {
		label = "(local)"
	}
	return ltGroupStyle.Render(label) + " " + ltCountStyle.Render(fmt.Sprintf("(%d)", count))
}

// formatRow renders a single row with name padded to nameCol so the status
// column aligns across all visible rows. Status sits two cells right of the
// longest name in view.
func (m listModel) formatRow(row listRow, isCursor bool, nameCol int, compact bool) string {
	prefix := "  "
	nameStyle := ltNameStyle
	if isCursor {
		prefix = ltCursorStyle.Render("▸") + " "
		nameStyle = ltCursorStyle
	}

	name := runewidth.Truncate(row.Name, nameCol, "…")
	name = runewidth.FillRight(name, nameCol)

	if !row.HasStatus {
		return prefix + nameStyle.Render(name)
	}

	icon := statusStyles[row.Status].Render(row.Status.Display().Icon)
	if compact {
		return prefix + nameStyle.Render(name) + "  " + icon
	}
	label := statusStyles[row.Status].Render(row.Status.Display().Label)
	return prefix + nameStyle.Render(name) + "  " + icon + " " + label
}

// renderSummary builds the colored "N current · N update · N missing" footer.
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
	order := []sync.Status{sync.StatusCurrent, sync.StatusOutdated, sync.StatusMissing, sync.StatusExtra}
	var parts []string
	for _, s := range order {
		if part := renderStatusCount(s, counts[s]); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ltDimStyle.Render(" · "))
}

// renderDetailPane draws the right side of the split view: metadata block
// followed by an inline action menu.
func (m listModel) renderDetailPane(row listRow, width int) string {
	var b strings.Builder
	b.WriteString(ltCursorStyle.Render(row.Name) + "\n")

	if row.Local != nil && row.Local.Description != "" {
		descStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("#aaaaaa"))
		b.WriteString(descStyle.Render(row.Local.Description) + "\n")
	}
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	type kv struct{ key, value string }
	var pairs []kv

	if row.HasStatus {
		pairs = append(pairs, kv{"Status", row.Status.Display().Label})
	}
	if row.Version != "" {
		pairs = append(pairs, kv{"Version", row.Version})
	}
	if row.Author != "" {
		pairs = append(pairs, kv{"Author", row.Author})
	}
	if row.Group != "" {
		pairs = append(pairs, kv{"Group", row.Group})
	}
	if row.Source != "" {
		pairs = append(pairs, kv{"Source", row.Source})
	}
	if len(row.Targets) > 0 {
		pairs = append(pairs, kv{"Targets", strings.Join(row.Targets, ", ")})
	}
	if row.Local != nil && row.Local.LocalPath != "" {
		path := row.Local.LocalPath
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
			path = "~" + strings.TrimPrefix(path, home)
		}
		pairs = append(pairs, kv{"Path", path})
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(10)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
	for _, p := range pairs {
		b.WriteString(keyStyle.Render(p.key) + valueStyle.Render(p.value) + "\n")
	}

	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	if m.statusMsg != "" {
		b.WriteString(m.statusMsg + "\n")
		return b.String()
	}

	actions := actionsForRow(row)
	for i, a := range actions {
		isCursor := i == m.actionCursor && m.focus == focusActions
		prefix := "  "
		if isCursor {
			prefix = ltCursorStyle.Render("▸") + " "
		}
		if a.disabled {
			label := ltDimStyle.Render(a.label)
			reason := ""
			if a.reason != "" {
				reason = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Italic(true).Render(a.reason)
			}
			b.WriteString(prefix + label + reason + "\n")
		} else {
			label := a.style.Render(a.label)
			if isCursor {
				label = ltCursorStyle.Render(a.label)
			}
			b.WriteString(prefix + label + "\n")
		}
	}
	return b.String()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func resolveEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func (m listModel) contentHeight() int {
	if m.height == 0 {
		return 20
	}
	headerHeight := 2 // title + divider
	searchHeight := 0
	if m.search != "" {
		searchHeight = 1
	}
	footerHeight := 3 // blank + summary/help + help
	h := m.height - headerHeight - searchHeight - footerHeight
	if h < 5 {
		h = 5
	}
	return h
}

func (m listModel) ensureCursorVisible() listModel {
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
	return m
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
