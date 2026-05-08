package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/textdiff"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

type listStage int

const (
	stageLoading listStage = iota
	stageBrowse
)

type listSubstate int

const (
	listSubstateNone listSubstate = iota
	listSubstateConfirm
	listSubstateUpdateChoice
	listSubstateUpdateConflictExists
	listSubstateTools
)

type updateChoice int

const (
	updateChoiceMerge updateChoice = iota
	updateChoicePreferTheirs
)

type detailFocus int

const (
	focusList detailFocus = iota
	focusActions
	focusPreview
)

type viewportTarget int

const (
	viewportYours viewportTarget = iota
	viewportIncoming
)

type listRow = workflow.ListRow

const (
	updatePreviewMaxLines = 500
	updatePreviewMaxBytes = 64 * 1024
)

type listModel struct {
	stage          listStage
	spinnerFrame   int
	backgroundLoad bool
	rows           []listRow
	filtered       []listRow
	groupCounts    map[string]int
	cursor         int
	offset         int
	search         string
	searchMode     bool
	commandMode    bool
	commandInput   string
	selected       bool
	focus          detailFocus
	actionCursor   int
	excerptOffset  int
	substate       listSubstate
	toolCursor     int
	toolStatuses   []tools.Status
	toolSelection  map[string]bool
	toolMode       state.ToolsMode
	restoreName    string
	restoreGroup   string
	restoreDetail  bool
	statusMsg      string
	updateHasMods  bool
	updatePreview  struct {
		loading           bool
		err               error
		requestID         uint64
		rowName           string
		rowGroup          string
		diffYours         string
		diffIncoming      string
		diffOverflowed    bool
		yoursOverflowN    int
		incomingOverflowN int
		baseSkillMD       []byte
		localSkillMD      []byte
	}
	viewYours        viewport.Model
	viewIncoming     viewport.Model
	activeViewport   viewportTarget
	previewIDCounter uint64
	pendingTickID    int

	ctx context.Context
	bag *workflow.Bag

	width  int
	height int

	err      error
	warnings []string
	quitting bool
}

func newListModel(ctx context.Context, bag *workflow.Bag) listModel {
	return listModel{
		stage:        stageLoading,
		search:       bag.InitialQuery,
		ctx:          ctx,
		bag:          bag,
		viewYours:    viewport.New(viewport.WithWidth(0), viewport.WithHeight(0)),
		viewIncoming: viewport.New(viewport.WithWidth(0), viewport.WithHeight(0)),
	}
}

func (m listModel) isBrowseMode() bool {
	return m.bag != nil && m.bag.BrowseFlag
}

func (m listModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestWindowSize,
		tickSpinnerCmd(),
		loadRowsCmd(m.ctx, m.bag),
	)
}

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
		if m.stage == stageLoading || m.updatePreview.loading || m.backgroundLoad {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, tickSpinnerCmd()
		}
		return m, nil
	case rowsLoadedMsg:
		m = m.clearUpdatePreview()
		m.stage = stageBrowse
		m.rows = msg.rows
		m.warnings = msg.warnings
		m = m.refreshFiltered()
		m = m.restoreSelection()
		if !m.isBrowseMode() {
			repos := registriesForBackgroundCheck(m.bag.Config, m.bag.State)
			if len(repos) > 0 {
				m.backgroundLoad = true
				return m, loadRegistryStatusesCmd(m.ctx, m.bag, repos)
			}
		}
		return m, nil
	case loadErrMsg:
		m.stage = stageBrowse
		m.err = msg.err
		return m, nil
	case registryStatusesLoadedMsg:
		m = m.clearUpdatePreview()
		m.backgroundLoad = false
		m = m.applyRegistryStatuses(msg.statuses)
		if len(msg.warnings) > 0 {
			m.warnings = append(m.warnings, msg.warnings...)
		}
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
	case commandDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Command failed: %v", msg.err)
			return m, nil
		}
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.restoreName = m.filtered[m.cursor].Name
			m.restoreGroup = m.filtered[m.cursor].Group
			m.restoreDetail = true
		}
		m.statusMsg = ""
		return m, loadRowsCmd(m.ctx, m.bag)
	case updateDoneMsg:
		m = m.clearUpdatePreview()
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.restoreName = m.filtered[m.cursor].Name
			m.restoreGroup = m.filtered[m.cursor].Group
			m.restoreDetail = true
		}
		if msg.conflicted {
			m.statusMsg = "Merge conflict — resolve in editor"
		} else if msg.merged {
			m.statusMsg = "Updated! (merged with local changes)"
		} else {
			m.statusMsg = "Updated!"
		}
		m.updateHasMods = false
		var cmds []tea.Cmd
		cmds = append(cmds, loadRowsCmd(m.ctx, m.bag))
		if msg.openPath != "" {
			editor := resolveEditor(m.bag.Config)
			cmds = append(cmds, tea.ExecProcess(exec.Command(editor, msg.openPath), func(err error) tea.Msg {
				return editorDoneMsg{err: err}
			}))
		}
		return m, tea.Batch(cmds...)
	case upstreamPreviewMsg:
		if msg.requestID != m.updatePreview.requestID || msg.rowName != m.updatePreview.rowName {
			return m, nil
		}
		m.updatePreview.loading = false
		m.updatePreview.err = msg.err
		if msg.err != nil {
			return m, nil
		}
		incoming := textdiff.Unified("SKILL.md", m.updatePreview.baseSkillMD, msg.skillMD)
		truncatedIncoming, incomingOverflowed := textdiff.TruncateUnified(incoming, updatePreviewMaxLines, updatePreviewMaxBytes)
		m.updatePreview.diffIncoming = truncatedIncoming
		m.updatePreview.incomingOverflowN = diffOverflowLines(truncatedIncoming)
		m.updatePreview.diffOverflowed = m.updatePreview.diffOverflowed || incomingOverflowed
		m.viewYours.SetContent(m.updatePreview.diffYours)
		m.viewIncoming.SetContent(m.updatePreview.diffIncoming)
		return m, nil
	case toolsSavedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.restoreName = m.filtered[m.cursor].Name
			m.restoreGroup = m.filtered[m.cursor].Group
			m.restoreDetail = true
		}
		m.substate = listSubstateNone
		m.toolCursor = 0
		m.toolStatuses = nil
		m.toolSelection = nil
		m.statusMsg = fmt.Sprintf("Updated tools: %s", strings.Join(msg.result.Tools, ", "))
		return m, loadRowsCmd(m.ctx, m.bag)
	case repairSavedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.restoreName = m.filtered[m.cursor].Name
			m.restoreGroup = m.filtered[m.cursor].Group
			m.restoreDetail = true
		}
		m.substate = listSubstateNone
		m.toolCursor = 0
		m.statusMsg = fmt.Sprintf("Reinstalled for: %s", strings.Join(msg.result.Tools, ", "))
		return m, loadRowsCmd(m.ctx, m.bag)
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

func (m listModel) nextPreviewID() (listModel, uint64) {
	m.previewIDCounter++
	return m, m.previewIDCounter
}

func (m listModel) clearUpdatePreview() listModel {
	if m.updatePreview.requestID != 0 {
		m.previewIDCounter++
	}
	m.updatePreview.loading = false
	m.updatePreview.err = nil
	m.updatePreview.requestID = 0
	m.updatePreview.rowName = ""
	m.updatePreview.rowGroup = ""
	m.updatePreview.diffYours = ""
	m.updatePreview.diffIncoming = ""
	m.updatePreview.diffOverflowed = false
	m.updatePreview.yoursOverflowN = 0
	m.updatePreview.incomingOverflowN = 0
	m.updatePreview.baseSkillMD = nil
	m.updatePreview.localSkillMD = nil
	m.viewYours.SetContent("")
	m.viewIncoming.SetContent("")
	return m
}

func diffOverflowLines(diff string) int {
	const marker = "… diff truncated: "
	idx := strings.LastIndex(diff, marker)
	if idx < 0 {
		return 0
	}
	var n int
	for _, r := range diff[idx+len(marker):] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (m listModel) applyRegistryStatuses(statusesByRepo map[string][]sync.SkillStatus) listModel {
	if len(statusesByRepo) == 0 {
		return m
	}
	currentName := ""
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		currentName = m.filtered[m.cursor].Name
	}
	for i := range m.rows {
		statuses := statusesByRepo[m.rows[i].Group]
		for _, ss := range statuses {
			if ss.Name != m.rows[i].Name {
				continue
			}
			m.rows[i].HasStatus = true
			m.rows[i].Status = ss.Status
			m.rows[i].Entry = ss.Entry
			m.rows[i].Version = ss.DisplayVersion()
			m.rows[i].Author = ss.Maintainer
			m.rows[i].LatestSHA = ss.LatestSHA
			if ss.Installed != nil {
				m.rows[i].Targets = ss.Installed.Tools
			}
			break
		}
	}
	m = m.refreshFiltered()
	if currentName != "" {
		for i := range m.filtered {
			if m.filtered[i].Name == currentName {
				m.cursor = i
				m = m.ensureCursorVisible()
				break
			}
		}
	}
	return m
}

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

	return tea.NewView(s)
}
