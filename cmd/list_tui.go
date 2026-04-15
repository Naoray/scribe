package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
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
	listSubstateUpdateChoice
	listSubstateTools
)

type updateChoice int

const (
	updateChoiceMerge updateChoice = iota
	updateChoicePreferTheirs
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

// ── Row data ────────────────────────────────────────────────────────────────

type listRow = workflow.ListRow

// ── Action items ───────────────────────────────────────────────────────────

type actionItem struct {
	label    string
	key      string
	disabled bool
	reason   string
	style    lipgloss.Style
}

func actionsForRow(row listRow, browseMode bool) []actionItem {
	if browseMode {
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

// ── Messages ───────────────────────────────────────────────────────────────

type tickSpinnerMsg struct{}
type rowsLoadedMsg struct {
	rows     []listRow
	warnings []string
}
type loadErrMsg struct{ err error }
type registryStatusesLoadedMsg struct {
	statuses map[string][]sync.SkillStatus
	warnings []string
}
type clipboardTickMsg struct{ id int }
type editorDoneMsg struct{ err error }
type commandDoneMsg struct{ err error }
type updateDoneMsg struct {
	name       string
	err        error
	merged     bool
	conflicted bool
	openPath   string
}
type toolsSavedMsg struct {
	result skillEditResult
	err    error
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const registryMuteAfter = 3
const registryStatusCacheTTL = 5 * time.Minute

var listRegistryStatusesFn = loadRegistryStatuses
var listEnsureRemoteDepsFn = ensureListRemoteDepsLoaded
var nowFn = time.Now

type cachedRegistryStatuses struct {
	at       time.Time
	statuses []sync.SkillStatus
}

var registryStatusCache = struct {
	mu    stdsync.Mutex
	items map[string]cachedRegistryStatuses
}{
	items: map[string]cachedRegistryStatuses{},
}

func tickSpinnerCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickSpinnerMsg{} })
}

// ── Model ───────────────────────────────────────────────────────────────────

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
	commandMode    bool
	commandInput   string
	selected       bool        // true when right-side detail/action pane is open
	focus          detailFocus // which pane is focused while selected
	actionCursor   int
	substate       listSubstate
	toolCursor     int
	toolStatuses   []tools.Status
	toolSelection  map[string]bool
	toolMode       state.ToolsMode
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

// ── Loader ──────────────────────────────────────────────────────────────────

func loadRowsCmd(ctx context.Context, bag *workflow.Bag) tea.Cmd {
	return func() tea.Msg {
		if err := ensureListBagLoaded(ctx, bag); err != nil {
			return loadErrMsg{err: err}
		}
		rows, warnings, err := workflow.BuildRows(ctx, bag)
		if err != nil {
			return loadErrMsg{err: err}
		}
		return rowsLoadedMsg{rows: rows, warnings: warnings}
	}
}

func ensureListBagLoaded(ctx context.Context, bag *workflow.Bag) error {
	if bag == nil {
		return fmt.Errorf("list loader: missing workflow bag")
	}
	needsTools := bag.RemoteFlag || bag.BrowseFlag
	if bag.Config != nil && bag.State != nil && (!needsTools || bag.Tools != nil) {
		return nil
	}

	steps := workflow.ListLoadStepsLocal()
	if needsTools {
		steps = workflow.ListLoadStepsRemote()
	}
	return workflow.Run(ctx, steps, bag)
}

func ensureListRemoteDepsLoaded(ctx context.Context, bag *workflow.Bag) error {
	if bag == nil {
		return fmt.Errorf("list action: missing workflow bag")
	}
	if bag.Config == nil || bag.State == nil {
		if err := ensureListBagLoaded(ctx, bag); err != nil {
			return err
		}
	}
	if bag.Provider != nil && bag.Tools != nil {
		return nil
	}

	originalLazy := bag.LazyGitHub
	bag.LazyGitHub = false
	defer func() {
		bag.LazyGitHub = originalLazy
	}()

	return workflow.Run(ctx, []workflow.Step{
		{Name: "LoadConfig", Fn: workflow.StepLoadConfig},
		{Name: "LoadState", Fn: workflow.StepLoadState},
		{Name: "ResolveTools", Fn: workflow.StepResolveTools},
	}, bag)
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
		m.warnings = msg.warnings
		m = m.refreshFiltered()
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
		m.statusMsg = ""
		return m, loadRowsCmd(m.ctx, m.bag)
	case updateDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		// Find the row and update its status.
		for i := range m.filtered {
			if m.filtered[i].Name == msg.name && m.filtered[i].Group == m.filtered[m.cursor].Group {
				if msg.conflicted {
					m.filtered[i].Status = sync.StatusConflicted
					m.statusMsg = "Merge conflict — resolve in editor"
				} else if msg.merged {
					m.filtered[i].Status = sync.StatusCurrent
					m.statusMsg = "Updated! (merged with local changes)"
				} else {
					m.filtered[i].Status = sync.StatusCurrent
					m.statusMsg = "Updated!"
				}
				m.updateHasMods = false
				break
			}
		}
		// Also update in m.rows.
		for i := range m.rows {
			if m.rows[i].Name == msg.name {
				if msg.conflicted {
					m.rows[i].Status = sync.StatusConflicted
				} else {
					m.rows[i].Status = sync.StatusCurrent
				}
				break
			}
		}
		m.pendingTickID++
		tickID := m.pendingTickID
		var cmds []tea.Cmd
		if msg.openPath != "" {
			editor := resolveEditor(m.bag.Config)
			cmds = append(cmds, tea.ExecProcess(exec.Command(editor, msg.openPath), func(err error) tea.Msg {
				return editorDoneMsg{err: err}
			}))
		}
		cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return clipboardTickMsg{id: tickID}
		}))
		return m, tea.Batch(cmds...)
	case toolsSavedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.substate = listSubstateNone
		m.toolCursor = 0
		m.toolStatuses = nil
		m.toolSelection = nil
		m.statusMsg = fmt.Sprintf("Updated tools: %s", strings.Join(msg.result.Tools, ", "))
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

func (m listModel) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.commandMode {
		return m.updateCommandMode(msg)
	}
	text := typedText(msg)
	switch msg.String() {
	case "ctrl+c", "q":
		if m.search != "" {
			m = m.resetSearch()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "esc", "escape":
		if m.search != "" {
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
		if text == "/" && m.search == "" {
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

func loadRegistryStatusesCmd(ctx context.Context, bag *workflow.Bag, repos []string) tea.Cmd {
	return func() tea.Msg {
		statuses, warnings := listRegistryStatusesFn(ctx, bag, repos)
		return registryStatusesLoadedMsg{statuses: statuses, warnings: warnings}
	}
}

func registriesForBackgroundCheck(cfg *config.Config, st *state.State) []string {
	if cfg == nil || st == nil {
		return nil
	}
	enabled := make(map[string]bool, len(cfg.TeamRepos()))
	for _, repo := range cfg.TeamRepos() {
		enabled[repo] = true
	}
	set := map[string]bool{}
	for _, installed := range st.Installed {
		if installed.Origin != state.OriginRegistry {
			continue
		}
		for _, src := range installed.Sources {
			if src.Registry != "" && enabled[src.Registry] {
				set[src.Registry] = true
			}
		}
	}
	repos := make([]string, 0, len(set))
	for repo := range set {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos
}

func loadRegistryStatuses(ctx context.Context, bag *workflow.Bag, repos []string) (map[string][]sync.SkillStatus, []string) {
	if bag == nil || bag.Factory == nil {
		return nil, nil
	}
	client, err := bag.Factory.Client()
	if err != nil {
		return nil, []string{fmt.Sprintf("load github client: %v", err)}
	}
	prov := provider.NewGitHubProvider(provider.WrapGitHubClient(client))
	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(client),
		Provider: prov,
		Tools:    []tools.Tool{},
	}
	statusesByRepo := make(map[string][]sync.SkillStatus, len(repos))
	var warnings []string
	for _, repo := range repos {
		if cached, ok := loadCachedRegistryStatuses(repo); ok {
			statusesByRepo[repo] = cached
			continue
		}
		statuses, _, err := syncer.Diff(ctx, repo, bag.State)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", repo, err))
			continue
		}
		storeCachedRegistryStatuses(repo, statuses)
		statusesByRepo[repo] = statuses
	}
	return statusesByRepo, warnings
}

func loadCachedRegistryStatuses(repo string) ([]sync.SkillStatus, bool) {
	registryStatusCache.mu.Lock()
	defer registryStatusCache.mu.Unlock()
	cached, ok := registryStatusCache.items[repo]
	if !ok {
		return nil, false
	}
	if nowFn().Sub(cached.at) > registryStatusCacheTTL {
		delete(registryStatusCache.items, repo)
		return nil, false
	}
	return cached.statuses, true
}

func storeCachedRegistryStatuses(repo string, statuses []sync.SkillStatus) {
	registryStatusCache.mu.Lock()
	defer registryStatusCache.mu.Unlock()
	copied := make([]sync.SkillStatus, len(statuses))
	copy(copied, statuses)
	registryStatusCache.items[repo] = cachedRegistryStatuses{
		at:       nowFn(),
		statuses: copied,
	}
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
		if m.commandMode {
			return m.updateCommandMode(msg)
		}
		// Browsing the list with the detail pane open: arrow keys move
		// the row cursor and the right pane refreshes live. Right/enter
		// hands focus to the action menu. Character keys still filter the
		// left list without forcing the user to close the detail pane first.
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
		case "backspace":
			m = m.backspaceSearch()
			m.actionCursor = 0
			m.statusMsg = ""
			if !m.selected {
				m.focus = focusList
			}
		default:
			if key == "/" && m.search == "" {
				return m, nil
			}
			if key == ":" && m.search == "" {
				m.commandMode = true
				m.commandInput = ""
				return m, nil
			}
			next := m.appendSearch(key)
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

	// focusActions: arrow keys move within the action list, left returns
	// focus to the row list, enter executes.
	actions := actionsForRow(m.filtered[m.cursor], m.isBrowseMode())
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
	case "enter", " ":
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

func (m listModel) runAdopt(row listRow) tea.Cmd {
	if row.Local == nil {
		return nil
	}
	bag := m.bag
	return func() tea.Msg {
		candidates, _, err := adopt.FindCandidates(bag.State, bag.Config.Adoption)
		if err != nil {
			return commandDoneMsg{err: err}
		}

		var target *adopt.Candidate
		for i := range candidates {
			if candidates[i].Name == row.Name && candidates[i].LocalPath == row.Local.LocalPath {
				target = &candidates[i]
				break
			}
		}
		if target == nil {
			return commandDoneMsg{err: fmt.Errorf("skill %q is not adoptable", row.Name)}
		}

		resolvedTools, err := tools.ResolveActive(bag.Config)
		if err != nil {
			return commandDoneMsg{err: err}
		}

		adopter := &adopt.Adopter{
			State: bag.State,
			Tools: resolvedTools,
		}
		result := adopter.Apply([]adopt.Candidate{*target})
		if err := result.Failed[target.Name]; err != nil {
			return commandDoneMsg{err: err}
		}
		return commandDoneMsg{}
	}
}

func (m listModel) runSaveTools(name string) tea.Cmd {
	cfg := m.bag.Config
	st := m.bag.State
	mode := m.toolMode
	desired := append([]string(nil), m.selectedToolNames()...)
	return func() tea.Msg {
		result, err := applySkillToolSelection(cfg, st, name, mode, desired)
		return toolsSavedMsg{result: result, err: err}
	}
}

func (m listModel) runInstall(row listRow) tea.Cmd {
	if row.Entry == nil {
		return nil
	}
	ctx := m.ctx
	bag := m.bag
	return func() tea.Msg {
		if err := listEnsureRemoteDepsFn(ctx, bag); err != nil {
			return commandDoneMsg{err: err}
		}
		err := runAddDirectInstall(
			ctx,
			row.Group,
			row.Name,
			bag.Config,
			bag.State,
			newInstallSyncer(bag.Client, bag.Tools),
			bag.Client != nil && bag.Client.IsAuthenticated(),
			false,
			true,
		)
		return commandDoneMsg{err: err}
	}
}

func (m listModel) runUpdate(choice updateChoice) tea.Cmd {
	if m.cursor >= len(m.filtered) {
		return nil
	}

	row := m.filtered[m.cursor]
	repo := row.Group
	ss := sync.SkillStatus{
		Name:      row.Name,
		Status:    row.Status,
		Entry:     row.Entry,
		IsPackage: row.Entry.IsPackage(),
		LatestSHA: row.LatestSHA,
	}
	if row.Local != nil {
		if inst, ok := m.bag.State.Installed[row.Name]; ok {
			ss.Installed = &inst
		}
	}

	wasModified := rowHasLocalModifications(row, m.bag.State)

	ctx := m.ctx
	bag := m.bag
	return func() tea.Msg {
		if err := listEnsureRemoteDepsFn(ctx, bag); err != nil {
			return updateDoneMsg{name: row.Name, err: err}
		}
		syncer := &sync.Syncer{
			Client:           sync.WrapGitHubClient(bag.Client),
			Provider:         bag.Provider,
			Tools:            bag.Tools,
			Executor:         &sync.ShellExecutor{},
			TrustAll:         bag.TrustAllFlag,
			ModifiedStrategy: sync.ModifiedStrategyMerge,
		}
		if choice == updateChoicePreferTheirs {
			syncer.ModifiedStrategy = sync.ModifiedStrategyPreferTheirs
		}
		isTTY := isatty.IsTerminal(os.Stdin.Fd())
		if isTTY && !bag.TrustAllFlag && !bag.JSONFlag {
			syncer.ApprovalFunc = func(name, command, source string) bool {
				var approved bool
				err := huh.NewConfirm().
					Title(fmt.Sprintf("Package %q wants to run a shell command", name)).
					Description(fmt.Sprintf("source:  %s\ncommand: %s", source, command)).
					Affirmative("Approve").
					Negative("Deny").
					Value(&approved).
					Run()
				if err != nil {
					return false
				}
				return approved
			}
		}
		err := syncer.RunWithDiff(ctx, repo, []sync.SkillStatus{ss}, bag.State)
		if err != nil {
			return updateDoneMsg{name: row.Name, err: err}
		}
		localSkills, _ := discovery.OnDisk(bag.State)
		for _, sk := range localSkills {
			if sk.Name == row.Name && sk.Conflicted {
				return updateDoneMsg{
					name:       row.Name,
					conflicted: true,
					openPath:   filepath.Join(sk.LocalPath, "SKILL.md"),
				}
			}
		}
		return updateDoneMsg{name: row.Name, merged: wasModified && choice == updateChoiceMerge}
	}
}

func rowHasLocalModifications(row listRow, st *state.State) bool {
	if st == nil {
		return false
	}
	if row.Local != nil {
		if row.Local.Modified {
			return true
		}
		if row.Local.LocalPath != "" {
			installed, ok := st.Installed[row.Name]
			if !ok {
				return false
			}
			return sync.IsLocallyModified(row.Local.LocalPath, installed.InstalledHash)
		}
	}
	installed, ok := st.Installed[row.Name]
	if !ok {
		return false
	}
	storeDir, err := tools.StoreDir()
	if err != nil {
		return false
	}
	return sync.IsLocallyModified(filepath.Join(storeDir, row.Name), installed.InstalledHash)
}

func (m listModel) resetSearch() listModel {
	m.search = ""
	m = m.refreshFiltered()
	if len(m.filtered) == 0 {
		m.selected = false
	}
	return m
}

func (m listModel) backspaceSearch() listModel {
	if len(m.search) == 0 {
		return m
	}
	m.search = m.search[:len(m.search)-1]
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

	// Uninstall from every tool that originally installed the skill, even if
	// that tool is now disabled. Resolving from installed.Tools instead of
	// ResolveActive prevents orphaning Gemini/custom-tool installs whenever
	// the user disables a tool after installing a skill.
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

	// Drop the row from both filtered and rows.
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

// ── Filter ──────────────────────────────────────────────────────────────────

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

func buildGroupCounts(rows []listRow) map[string]int {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Group]++
	}
	return counts
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

	return tea.NewView(s)
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

// viewListFull is the default browse view: full-width list with status icons
// and a status-count footer.
func (m listModel) viewListFull() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString(m.renderQueryLine() + "\n")

	contentHeight := m.contentHeight()
	m.renderRows(&b, contentHeight, m.width-4, false)

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

// viewSplit is the detail view: compact list on the left, detail + action
// menu on the right. Triggered by pressing Enter on a row.
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
	default:
		if m.isBrowseMode() {
			b.WriteString(ltDimStyle.Render("↑↓ choose install · enter run · ←/tab back to list · esc close") + "\n")
		} else {
			b.WriteString(ltDimStyle.Render("↑↓ pick action · enter run · ←/tab back to list · esc close") + "\n")
		}
	}
	return b.String()
}

func (m listModel) renderQueryLine() string {
	if m.commandMode {
		if m.commandInput != "" {
			return ": " + m.commandInput
		}
		return ltDimStyle.Render(": command...")
	}
	if m.search != "" {
		return "/ " + m.search
	}
	if m.isBrowseMode() {
		return ltDimStyle.Render("/ search registries...")
	}
	return ltDimStyle.Render("/ search...")
}

// renderHeader prints the title row "Installed Skills · N skills".
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

// wrapText wraps s so no visual line exceeds width cells. Preserves existing
// newlines. Used for per-registry warnings and error messages so long errors
// don't bleed past the right edge of the viewport.
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
		statusReserve = 42 // version(14)+gap(2) + author(12)+gap(2) + icon(1)+space(1)+label(7)+breathing(3)
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
			header := m.formatGroupHeader(row.Group)
			if header != "" {
				if linesUsed+1 >= contentHeight {
					break
				}
				b.WriteString(header + "\n")
				linesUsed++
			}
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
	if group == "" {
		return ""
	}
	count := m.groupCounts[group]
	return ltGroupStyle.Render(group) + " " + ltCountStyle.Render(fmt.Sprintf("(%d)", count))
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

	// Full view: name + version + author + status
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
	order := []sync.Status{sync.StatusCurrent, sync.StatusModified, sync.StatusOutdated, sync.StatusConflicted, sync.StatusMissing, sync.StatusExtra}
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

	type kv struct{ key, value string }
	var pairs []kv

	if row.HasStatus {
		pairs = append(pairs, kv{"Status", row.Status.Display().Label})
	}
	if row.Local != nil && !row.Managed {
		pairs = append(pairs, kv{"Managed", "no"})
	}
	if row.Version != "" {
		pairs = append(pairs, kv{"Version", row.Version})
	}
	if row.Author != "" {
		pairs = append(pairs, kv{"Author", row.Author})
	}
	if row.Group != "" {
		pairs = append(pairs, kv{"Registry", row.Group})
	}
	if row.Origin == state.OriginLocal {
		pairs = append(pairs, kv{"Source", "(local)"})
	}
	if len(row.Targets) > 0 {
		pairs = append(pairs, kv{"Tools", strings.Join(row.Targets, ", ")})
	}
	if row.Local != nil && row.Local.LocalPath != "" {
		path := row.Local.LocalPath
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
			path = "~" + strings.TrimPrefix(path, home)
		}
		pairs = append(pairs, kv{"Path", path})
	}

	for _, p := range pairs {
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

	actions := actionsForRow(row, m.isBrowseMode())
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
				reason = " " + ltReasonStyle.Render(a.reason)
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

	if row.Excerpt != "" {
		b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")
		b.WriteString(renderExcerptPreview(row.Excerpt, width-2) + "\n")
	}
	return b.String()
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

func renderExcerptPreview(excerpt string, width int) string {
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
		lines = append(lines, renderInlineCode(style, text))
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
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
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

// ── Helpers ─────────────────────────────────────────────────────────────────

func resolveEditor(cfg *config.Config) string {
	// 1. Check config
	if cfg != nil && cfg.Editor != "" {
		return cfg.Editor
	}
	// 2. Environment
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
	// header(2: title + divider) + search(1) + footer(3: blank + summary + help) = 6 chrome lines
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	return h
}

func (m listModel) ensureCursorVisible() listModel {
	visible := m.contentHeight()
	// Count group headers between offset and cursor that consume visible lines.
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

// readExcerpt reads SKILL.md from skillDir, strips YAML frontmatter, and
// returns the first maxLines non-empty body lines as a single string.
