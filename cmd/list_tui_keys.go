package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type actionItem struct {
	label    string
	key      string
	disabled bool
	reason   string
	style    lipgloss.Style
}

func (m listModel) actionsForRow(row listRow) []actionItem {
	if m.isBrowseMode() {
		canInstall := row.Entry != nil && row.Status != sync.StatusCurrent
		reason := "already installed"
		if row.Entry == nil {
			reason = "source unknown"
		}
		return []actionItem{
			{label: "install", key: "install", disabled: !canInstall, reason: reason, style: ltUpdateStyle},
		}
	}
	hasLocal := row.Local != nil && row.Local.LocalPath != ""
	hasRepair := row.Managed && hasLocal
	canAdopt := hasLocal && !row.Managed
	canUpdate := row.HasStatus && row.Status == sync.StatusOutdated && row.Entry != nil
	updateReason := "up to date"
	if !row.HasStatus {
		updateReason = "no registry"
		if row.Managed && row.Origin == state.OriginRegistry && row.Group != "" {
			updateReason = "checking registry..."
		}
	} else if row.Entry == nil {
		updateReason = "source unknown"
	}
	actions := []actionItem{
		{label: "update", key: "update", disabled: !canUpdate, reason: updateReason, style: ltUpdateStyle},
	}
	if hasRepair {
		actions = append(actions, actionItem{label: "repair", key: "repair", style: ltNeutralStyle})
	}
	if row.Managed {
		actions = append(actions, actionItem{label: "tools", key: "tools", disabled: !hasLocal, reason: "not on disk", style: ltNeutralStyle})
	}
	if canAdopt {
		actions = append(actions, actionItem{label: "adopt", key: "adopt", style: ltUpdateStyle})
	}
	actions = append(actions,
		actionItem{label: "add to category", key: "category", disabled: true, reason: "coming soon", style: ltDimStyle},
		actionItem{label: "copy path", key: "copy", disabled: !hasLocal, reason: "not on disk", style: ltNeutralStyle},
		actionItem{label: "open in editor", key: "edit", disabled: !hasLocal, reason: "not on disk", style: ltNeutralStyle},
		actionItem{label: "remove", key: "remove", disabled: !hasLocal, reason: "not on disk", style: ltRemoveStyle},
	)
	return actions
}

type listCommandHandler func(args []string) ([]string, error)

var listCommandHandlers = map[string]listCommandHandler{
	"add": func(args []string) ([]string, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("usage: :add <query>")
		}
		return []string{"browse", "--query", strings.Join(args, " ")}, nil
	},
	"remove": func(args []string) ([]string, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("usage: :remove <name>")
		}
		return []string{"remove", args[0]}, nil
	},
	"sync": func(args []string) ([]string, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("usage: :sync")
		}
		return []string{"sync"}, nil
	},
	"help": func(args []string) ([]string, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("usage: :help")
		}
		return []string{"browse", "--help"}, nil
	},
}

func parseListCommand(input string) ([]string, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			return nil, fmt.Errorf("flags are not supported in ':' commands")
		}
	}

	handler, ok := listCommandHandlers[fields[0]]
	if !ok {
		return nil, fmt.Errorf("unknown command: %s", fields[0])
	}
	return handler(fields[1:])
}

func (m listModel) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.commandMode {
		return m.updateCommandMode(msg)
	}
	text := typedText(msg)
	switch msg.String() {
	case "ctrl+c":
		if m.searchMode || m.search != "" {
			m = m.resetSearch()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "q":
		if m.searchMode {
			m = m.appendSearch("q")
			return m, nil
		}
		if m.search != "" {
			m = m.resetSearch()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "esc", "escape":
		if m.searchMode || m.search != "" {
			m = m.resetSearch()
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
		m = m.backspaceSearch()
	default:
		if text == "/" && !m.searchMode && m.search == "" {
			m.searchMode = true
			return m, nil
		}
		if text == ":" && m.search == "" {
			m.commandMode = true
			m.commandInput = ""
			return m, nil
		}
		m = m.appendSearch(text)
	}
	return m, nil
}

func (m listModel) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.substate == listSubstateConfirm {
		return m.updateConfirm(msg)
	}
	if m.substate == listSubstateUpdateChoice {
		return m.updateUpdateChoice(msg)
	}
	if m.substate == listSubstateTools {
		return m.updateToolsEditor(msg)
	}

	if m.cursor >= len(m.filtered) {
		m.selected = false
		m.focus = focusList
		return m, nil
	}

	key := msg.String()

	switch key {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "q":
		if m.searchMode {
			if m.focus == focusList {
				m = m.appendSearch("q")
			}
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "esc", "escape":
		m.selected = false
		m.focus = focusList
		m.actionCursor = 0
		m.excerptOffset = 0
		m.statusMsg = ""
		return m, nil
	case "tab":
		row := m.filtered[m.cursor]
		switch m.focus {
		case focusList:
			m.focus = focusActions
			m.actionCursor = 0
		case focusActions:
			if rowHasPreview(row) {
				m.focus = focusPreview
				m.excerptOffset = 0
			} else {
				m.focus = focusList
			}
		default:
			m.focus = focusList
			m.excerptOffset = 0
		}
		return m, nil
	case "shift+tab":
		row := m.filtered[m.cursor]
		switch m.focus {
		case focusList:
			if rowHasPreview(row) {
				m.focus = focusPreview
				m.excerptOffset = 0
			} else {
				m.focus = focusActions
				m.actionCursor = 0
			}
		case focusActions:
			m.focus = focusList
		default:
			m.focus = focusActions
			m.excerptOffset = 0
		}
		return m, nil
	}

	if m.focus == focusList {
		if m.commandMode {
			return m.updateCommandMode(msg)
		}
		text := typedText(msg)
		switch key {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m = m.ensureCursorVisible()
				m.actionCursor = 0
				m.excerptOffset = 0
				m.statusMsg = ""
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m = m.ensureCursorVisible()
				m.actionCursor = 0
				m.excerptOffset = 0
				m.statusMsg = ""
			}
		case "right", "l", "enter":
			m.focus = focusActions
			m.actionCursor = 0
		case "backspace":
			m = m.backspaceSearch()
			m.actionCursor = 0
			m.excerptOffset = 0
			m.statusMsg = ""
			if !m.selected {
				m.focus = focusList
			}
		default:
			if text == "/" && !m.searchMode && m.search == "" {
				m.searchMode = true
				return m, nil
			}
			if text == ":" && m.search == "" {
				m.commandMode = true
				m.commandInput = ""
				return m, nil
			}
			next := m.appendSearch(text)
			if next.search != m.search {
				m = next
				m.actionCursor = 0
				m.statusMsg = ""
				if !m.selected {
					m.focus = focusList
				}
			}
		}
		return m, nil
	}

	row := m.filtered[m.cursor]
	if m.focus == focusPreview {
		return m.updatePreviewFocus(row, key), nil
	}

	actions := m.actionsForRow(row)
	switch key {
	case "up", "k":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down", "j":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		} else if rowHasPreview(row) {
			m.focus = focusPreview
			m.excerptOffset = 0
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

// rowHasPreview reports whether a skill row exposes any excerpt content.
func rowHasPreview(row listRow) bool {
	return strings.TrimSpace(row.Excerpt) != ""
}

func (m listModel) updatePreviewFocus(row listRow, key string) listModel {
	maxOffset := previewMaxOffset(row.Excerpt)
	switch key {
	case "up", "k":
		if m.excerptOffset > 0 {
			m.excerptOffset--
			return m
		}
		m.focus = focusActions
		actions := m.actionsForRow(row)
		if len(actions) > 0 {
			m.actionCursor = len(actions) - 1
		}
	case "down", "j":
		if m.excerptOffset < maxOffset {
			m.excerptOffset++
		}
	case "left", "h":
		m.focus = focusActions
		actions := m.actionsForRow(row)
		if len(actions) > 0 {
			m.actionCursor = len(actions) - 1
		}
	case "home":
		m.excerptOffset = 0
	case "end":
		m.excerptOffset = maxOffset
	}
	return m
}

// previewMaxOffset returns the largest valid scroll offset for the excerpt.
// Keeping a small tail visible at the bottom feels nicer than letting the
// last line scroll off entirely.
func previewMaxOffset(excerpt string) int {
	total := strings.Count(excerpt, "\n") + 1
	if total <= 1 {
		return 0
	}
	return total - 1
}

func (m listModel) updateCommandMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	text := typedText(msg)
	switch msg.String() {
	case "ctrl+c", "q", "esc", "escape":
		m.commandMode = false
		m.commandInput = ""
		return m, nil
	case "backspace":
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
			return m, nil
		}
		m.commandMode = false
		return m, nil
	case "enter":
		args, err := parseListCommand(m.commandInput)
		m.commandMode = false
		m.commandInput = ""
		if err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		cmd := exec.Command(os.Args[0], args...)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return commandDoneMsg{err: err}
		})
	default:
		if text != "" {
			m.commandInput += text
		}
	}
	return m, nil
}

func typedText(msg tea.KeyPressMsg) string {
	if msg.Text != "" {
		return msg.Text
	}
	if len(msg.String()) == 1 {
		return msg.String()
	}
	return ""
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

func (m listModel) updateUpdateChoice(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !m.updateHasMods {
		switch msg.String() {
		case "u", "enter":
			m.substate = listSubstateNone
			m.statusMsg = "Updating..."
			return m, m.runUpdate(updateChoiceMerge)
		case "esc", "escape":
			m.substate = listSubstateNone
			m.statusMsg = ""
		}
		return m, nil
	}

	switch msg.String() {
	case "m":
		m.substate = listSubstateNone
		m.statusMsg = "Updating..."
		return m, m.runUpdate(updateChoiceMerge)
	case "r":
		m.substate = listSubstateNone
		m.statusMsg = "Updating..."
		return m, m.runUpdate(updateChoicePreferTheirs)
	case "l":
		m.substate = listSubstateNone
		m.statusMsg = "Kept local version. Registry update skipped."
		m.updateHasMods = false
		return m, nil
	case "esc", "escape":
		m.substate = listSubstateNone
		m.statusMsg = ""
		m.updateHasMods = false
	}
	return m, nil
}

func (m listModel) openToolsEditor(row listRow) listModel {
	if m.bag == nil || m.bag.State == nil {
		m.statusMsg = "State unavailable"
		return m
	}
	installed, ok := m.bag.State.Installed[row.Name]
	if !ok {
		m.statusMsg = fmt.Sprintf("Skill %q is not managed", row.Name)
		return m
	}

	statuses, err := tools.ResolveStatuses(m.bag.Config)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Resolve tools: %v", err)
		return m
	}

	selection := make(map[string]bool, len(installed.Tools))
	for _, toolName := range installed.Tools {
		selection[toolName] = true
	}
	if installed.ToolsMode != state.ToolsModePinned {
		for _, toolName := range availableToolNames(statuses) {
			selection[toolName] = true
		}
	}

	m.substate = listSubstateTools
	m.toolCursor = 0
	if len(statuses) > 0 {
		m.toolCursor = 1
	}
	m.toolStatuses = statuses
	m.toolSelection = selection
	m.toolMode = installed.ToolsMode
	m.statusMsg = ""
	return m
}

func (m listModel) updateToolsEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	maxCursor := len(m.toolStatuses) + 2
	switch key {
	case "esc", "escape":
		m.substate = listSubstateNone
		m.toolCursor = 0
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.toolCursor > 0 {
			m.toolCursor--
		}
		return m, nil
	case "down", "j":
		if m.toolCursor < maxCursor {
			m.toolCursor++
		}
		return m, nil
	case "enter", " ", "space":
		return m.activateToolsEditorCursor()
	}
	return m, nil
}

func (m listModel) activateToolsEditorCursor() (tea.Model, tea.Cmd) {
	switch {
	case m.toolCursor == 0:
		if m.toolMode == state.ToolsModePinned {
			m.toolMode = state.ToolsModeInherit
			m.toolSelection = make(map[string]bool, len(m.toolStatuses))
			for _, toolName := range availableToolNames(m.toolStatuses) {
				m.toolSelection[toolName] = true
			}
		} else {
			m.toolMode = state.ToolsModePinned
		}
		return m, nil
	case m.toolCursor <= len(m.toolStatuses):
		status := m.toolStatuses[m.toolCursor-1]
		available, reason := toolStatusAvailable(status)
		if !available {
			m.statusMsg = reason
			return m, nil
		}
		if m.toolMode != state.ToolsModePinned {
			m.toolMode = state.ToolsModePinned
		}
		if m.toolSelection == nil {
			m.toolSelection = map[string]bool{}
		}
		m.toolSelection[status.Name] = !m.toolSelection[status.Name]
		if !m.toolSelection[status.Name] {
			delete(m.toolSelection, status.Name)
		}
		return m, nil
	case m.toolCursor == len(m.toolStatuses)+1:
		if err := m.validateToolsEditor(); err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		row := m.filtered[m.cursor]
		m.statusMsg = "Saving tool changes..."
		return m, m.runSaveTools(row.Name)
	default:
		m.substate = listSubstateNone
		m.toolCursor = 0
		m.statusMsg = ""
		return m, nil
	}
}

func (m listModel) validateToolsEditor() error {
	if m.toolMode != state.ToolsModePinned {
		return nil
	}
	if len(m.selectedToolNames()) == 0 {
		return fmt.Errorf("select at least one tool or switch back to inherit")
	}
	return nil
}

func (m listModel) selectedToolNames() []string {
	names := make([]string, 0, len(m.toolSelection))
	for _, st := range m.toolStatuses {
		if m.toolSelection[st.Name] {
			names = append(names, st.Name)
		}
	}
	return names
}

func toolStatusAvailable(st tools.Status) (bool, string) {
	if !st.Enabled {
		return false, fmt.Sprintf("%s is disabled in config", st.Name)
	}
	if st.DetectKnown && !st.Detected {
		return false, fmt.Sprintf("%s is not available on this machine", st.Name)
	}
	return true, ""
}

func (m listModel) executeAction(key string) (tea.Model, tea.Cmd) {
	row := m.filtered[m.cursor]
	switch key {
	case "install":
		if m.isBrowseMode() && row.Entry != nil {
			m.statusMsg = "Installing..."
			return m, m.runInstall(row)
		}
		return m, nil
	case "update":
		if row.Entry == nil {
			return m, nil
		}
		if rowHasLocalModifications(row, m.bag.State) && !row.Entry.IsPackage() {
			m.substate = listSubstateUpdateChoice
			m.updateHasMods = true
			m.statusMsg = "Local edits detected. Choose: [r]egistry version, keep [l]ocal version, or [m]erge with upstream."
			return m, nil
		}
		m.substate = listSubstateUpdateChoice
		m.updateHasMods = false
		m.statusMsg = "No local edits detected. Update will replace the local copy with the registry version."
		return m, nil
	case "adopt":
		if row.Local == nil || row.Managed {
			return m, nil
		}
		m.statusMsg = "Adopting..."
		return m, m.runAdopt(row)
	case "repair":
		if row.Local == nil || !row.Managed {
			return m, nil
		}
		m.statusMsg = "Repairing..."
		return m, m.runRepair(row.Name)
	case "tools":
		return m.openToolsEditor(row), nil
	}
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
		editor := resolveEditor(m.bag.Config)
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

func (m listModel) resetSearch() listModel {
	m.search = ""
	m.searchMode = false
	m = m.refreshFiltered()
	if len(m.filtered) == 0 {
		m.selected = false
	}
	return m
}

func (m listModel) backspaceSearch() listModel {
	if len(m.search) == 0 {
		m.searchMode = false
		return m
	}
	m.search = m.search[:len(m.search)-1]
	if m.search == "" {
		m.searchMode = false
	}
	m = m.refreshFiltered()
	if len(m.filtered) == 0 {
		m.selected = false
	}
	return m
}

func (m listModel) appendSearch(key string) listModel {
	if len(key) != 1 {
		return m
	}
	m.searchMode = true
	m.search += key
	m = m.refreshFiltered()
	if len(m.filtered) == 0 {
		m.selected = false
	}
	return m
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
		filepath.Join(home, ".codex", "skills"),
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

	if installed, ok := m.bag.State.Installed[sk.Name]; ok {
		for _, name := range installed.Tools {
			tool, err := tools.ResolveByName(m.bag.Config, name)
			if err != nil {
				continue
			}
			_ = tool.Uninstall(sk.Name)
		}
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

	m.filtered = append(m.filtered[:m.cursor], m.filtered[m.cursor+1:]...)
	for i, r := range m.rows {
		if r.Name == sk.Name && r.Group == row.Group {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			break
		}
	}
	m.groupCounts = buildGroupCounts(m.filtered)

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

func (m listModel) applyFilter() []listRow {
	lower := strings.ToLower(m.search)
	var out []listRow
	for _, r := range m.rows {
		if m.isBrowseMode() && r.Local != nil {
			continue
		}
		if m.isBrowseMode() && r.Status == sync.StatusCurrent {
			continue
		}
		if m.search == "" {
			out = append(out, r)
			continue
		}
		if strings.Contains(strings.ToLower(r.Name), lower) ||
			strings.Contains(strings.ToLower(r.Group), lower) {
			out = append(out, r)
		}
	}
	return out
}

func (m listModel) refreshFiltered() listModel {
	m.filtered = m.applyFilter()
	m.groupCounts = buildGroupCounts(m.filtered)
	m.cursor, m.offset = 0, 0
	return m
}

func (m listModel) restoreSelection() listModel {
	if m.restoreName == "" || len(m.filtered) == 0 {
		return m
	}
	for i := range m.filtered {
		if m.filtered[i].Name != m.restoreName {
			continue
		}
		if m.restoreGroup != "" && m.filtered[i].Group != m.restoreGroup {
			continue
		}
		m.cursor = i
		m = m.ensureCursorVisible()
		m.selected = m.restoreDetail
		if m.selected {
			m.focus = focusActions
		}
		break
	}
	m.restoreName = ""
	m.restoreGroup = ""
	m.restoreDetail = false
	return m
}

func buildGroupCounts(rows []listRow) map[string]int {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Group]++
	}
	return counts
}

func (m listModel) ensureCursorVisible() listModel {
	visible := m.contentHeight()
	headersBetween := 0
	prevGroup := ""
	if m.offset > 0 && m.offset < len(m.filtered) {
		prevGroup = m.filtered[m.offset-1].Group
	}
	for i := m.offset; i <= m.cursor && i < len(m.filtered); i++ {
		if m.filtered[i].Group != prevGroup {
			headersBetween++
			prevGroup = m.filtered[i].Group
		}
	}
	effectiveVisible := visible - headersBetween
	if effectiveVisible < 3 {
		effectiveVisible = 3
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+effectiveVisible {
		m.offset = m.cursor - effectiveVisible + 1
	}
	return m
}
