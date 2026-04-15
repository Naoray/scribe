package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
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
)

type listRow = workflow.ListRow

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
	pendingTickID  int

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
		stage:  stageLoading,
		search: bag.InitialQuery,
		ctx:    ctx,
		bag:    bag,
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
		if m.stage == stageLoading {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, tickSpinnerCmd()
		}
		return m, nil
	case rowsLoadedMsg:
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
