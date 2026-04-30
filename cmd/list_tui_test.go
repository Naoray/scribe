package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

type panicBuildRowsProvider struct{}

func (panicBuildRowsProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	panic("Discover should not be called")
}

func (panicBuildRowsProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	panic("Fetch should not be called")
}

// key builds a KeyPressMsg whose String() matches the given label. Letter/char
// keys go through Text; named keys (tab, esc, up, ...) use Code with the
// matching rune from bubbletea's key table.
func key(label string) tea.KeyPressMsg {
	switch label {
	case "tab":
		return tea.KeyPressMsg{Code: 0x09}
	case "shift+tab":
		return tea.KeyPressMsg{Code: 0x09, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: 0x0d}
	case "esc":
		return tea.KeyPressMsg{Code: 0x1b}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	}
	return tea.KeyPressMsg{Text: label}
}

func testActionsForRow(row listRow, browseMode bool) []actionItem {
	return listModel{bag: &workflow.Bag{BrowseFlag: browseMode}}.actionsForRow(row)
}

func TestActionsForRow(t *testing.T) {
	t.Run("row with local skill has copy and edit enabled", func(t *testing.T) {
		row := listRow{
			Name:    "my-skill",
			Managed: true,
			Local: &discovery.Skill{
				Name:      "my-skill",
				LocalPath: "/home/user/.claude/skills/my-skill",
			},
		}
		actions := testActionsForRow(row, false)

		if len(actions) != 7 {
			t.Fatalf("expected 7 actions, got %d", len(actions))
		}
		if findAction(actions, "repair").key != "repair" {
			t.Fatal("managed local row should expose repair action")
		}
		if findAction(actions, "tools").key != "tools" {
			t.Fatal("managed local row should expose tools action")
		}
		if findAction(actions, "copy").disabled {
			t.Error("copy action should not be disabled when row has local path")
		}
		if findAction(actions, "edit").disabled {
			t.Error("edit action should not be disabled when row has local path")
		}
		if findAction(actions, "remove").disabled {
			t.Error("remove action should not be disabled when row has local path")
		}
		if actions[len(actions)-1].key != "remove" {
			t.Fatalf("last action = %q, want remove", actions[len(actions)-1].key)
		}
	})

	t.Run("row without local skill has actions disabled", func(t *testing.T) {
		row := listRow{Name: "ghost-skill", Local: nil}
		actions := testActionsForRow(row, false)

		copyAction := findAction(actions, "copy")
		if !copyAction.disabled {
			t.Error("copy action should be disabled when row has no local skill")
		}
		if copyAction.reason != "not on disk" {
			t.Errorf("copy reason = %q, want %q", copyAction.reason, "not on disk")
		}

		editAction := findAction(actions, "edit")
		if !editAction.disabled {
			t.Error("edit action should be disabled when row has no local skill")
		}

		removeAction := findAction(actions, "remove")
		if !removeAction.disabled {
			t.Error("remove action should be disabled when row has no local skill")
		}
	})

	t.Run("row with local skill but empty path has actions disabled", func(t *testing.T) {
		row := listRow{
			Name:  "tracked-only",
			Local: &discovery.Skill{Name: "tracked-only", LocalPath: ""},
		}
		actions := testActionsForRow(row, false)

		if !findAction(actions, "copy").disabled {
			t.Error("copy should be disabled when local path is empty")
		}
	})

	t.Run("update is always disabled", func(t *testing.T) {
		row := listRow{
			Name:  "any",
			Local: &discovery.Skill{Name: "any", LocalPath: "/some/path"},
		}
		if !findAction(testActionsForRow(row, false), "update").disabled {
			t.Error("update action should always be disabled")
		}
	})

	t.Run("category is always disabled", func(t *testing.T) {
		row := listRow{
			Name:  "any",
			Local: &discovery.Skill{Name: "any", LocalPath: "/some/path"},
		}
		if !findAction(testActionsForRow(row, false), "category").disabled {
			t.Error("category action should always be disabled")
		}
	})

	t.Run("cached registry rows do not claim no registry", func(t *testing.T) {
		row := listRow{
			Name:    "recap",
			Group:   "artistfy/hq",
			Managed: true,
			Origin:  state.OriginRegistry,
			Local:   &discovery.Skill{Name: "recap", LocalPath: "/home/user/.scribe/skills/recap"},
		}
		updateAction := findAction(testActionsForRow(row, false), "update")
		if updateAction.reason == "no registry" {
			t.Fatalf("update reason = %q, should use cached-registry wording", updateAction.reason)
		}
	})

	t.Run("browse mode only offers install for non-current rows", func(t *testing.T) {
		row := listRow{
			Name:      "remote-skill",
			Entry:     &manifest.Entry{Name: "remote-skill"},
			HasStatus: true,
			Status:    sync.StatusMissing,
		}
		actions := testActionsForRow(row, true)

		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].key != "install" {
			t.Fatalf("action key = %q, want install", actions[0].key)
		}
		if actions[0].disabled {
			t.Fatal("install action should be enabled for missing browse rows")
		}
	})

	t.Run("unmanaged local row offers adopt action", func(t *testing.T) {
		row := listRow{
			Name:    "bare-skill",
			Managed: false,
			Local:   &discovery.Skill{Name: "bare-skill", LocalPath: "/home/user/.claude/skills/bare-skill"},
		}
		actions := testActionsForRow(row, false)

		adoptAction := findAction(actions, "adopt")
		if adoptAction.key != "adopt" {
			t.Fatalf("adopt action missing: %+v", actions)
		}
		if adoptAction.disabled {
			t.Fatal("adopt action should be enabled for unmanaged local rows")
		}
		if findAction(actions, "repair").key != "" {
			t.Fatal("unmanaged row should not expose repair action")
		}
	})

	t.Run("adopted managed row disables adopt action", func(t *testing.T) {
		row := listRow{
			Name:    "adopted-skill",
			Managed: true,
			Origin:  state.OriginLocal,
			Local:   &discovery.Skill{Name: "adopted-skill", LocalPath: "/home/user/.scribe/skills/adopted-skill"},
		}
		adoptAction := findAction(testActionsForRow(row, false), "adopt")
		if adoptAction.key != "" {
			t.Fatal("adopt action should not be shown for already managed/adopted rows")
		}
	})
}

func TestEnsureListBagLoaded_InitializesLocalBag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bag := &workflow.Bag{
		Factory:    nil,
		LazyGitHub: true,
	}
	if err := ensureListBagLoaded(context.Background(), bag); err != nil {
		t.Fatalf("ensureListBagLoaded() error = %v", err)
	}
	if bag.Config == nil {
		t.Fatal("bag.Config should be loaded")
	}
	if bag.State == nil {
		t.Fatal("bag.State should be loaded")
	}
}

func TestBuildLocalRows_GroupsAndOrders(t *testing.T) {
	skills := []discovery.Skill{
		{Name: "gstack/zeta", Managed: true},
		{Name: "alpha", Managed: false},
		{Name: "gstack/beta", Managed: true},
		{Name: "local/gamma", Managed: false},
		{Name: "recap", Managed: true},
	}
	rows := workflow.BuildLocalRows(skills, &state.State{Installed: map[string]state.InstalledSkill{
		"recap": {
			Origin: state.OriginRegistry,
			Sources: []state.SkillSource{{
				Registry: "artistfy/hq",
			}},
		},
	}})

	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}

	// Managed rows first, then unmanaged rows. Bare local names no longer get
	// a synthetic "Local (unmanaged)" group header.
	want := []struct{ name, group string }{
		{"recap", "artistfy/hq"},
		{"gstack/beta", "gstack"},
		{"gstack/zeta", "gstack"},
		{"alpha", ""},
		{"local/gamma", ""},
	}
	for i, w := range want {
		if rows[i].Name != w.name || rows[i].Group != w.group {
			t.Errorf("row %d = %+v, want name=%q group=%q", i, rows[i], w.name, w.group)
		}
	}
}

func TestBuildRows_DefaultListIsLocalOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "local-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: local-skill\ndescription: local\n---\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	rows, warnings, err := workflow.BuildRows(context.Background(), &workflow.Bag{
		Config: &config.Config{
			Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}},
		},
		State:      &state.State{Installed: map[string]state.InstalledSkill{}},
		RemoteFlag: false,
		Provider:   panicBuildRowsProvider{},
		RepoFlag:   "",
		JSONFlag:   false,
		Factory:    nil,
		FilterRegistries: func(flag string, repos []string) ([]string, error) {
			return repos, nil
		},
	})
	if err != nil {
		t.Fatalf("buildRows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].Name != "local-skill" {
		t.Fatalf("rows[0].Name = %q, want local-skill", rows[0].Name)
	}
	if rows[0].HasStatus {
		t.Fatal("rows[0] should not have remote status in local mode")
	}
}

func TestRegistryGroupFromName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Artistfy-hq/deploy", "Artistfy-hq"},
		{"local/foo", ""},
		{"bare", ""},
		{"owner/skill-name", "owner"},
	}
	for _, tt := range tests {
		got := workflow.RegistryGroupFromName(tt.name)
		if got != tt.want {
			t.Errorf("RegistryGroupFromName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFormatGroupHeader_EmptyGroupRendersNothing(t *testing.T) {
	m := listModel{groupCounts: map[string]int{"": 2}}
	if got := m.formatGroupHeader(""); got != "" {
		t.Fatalf("formatGroupHeader(\"\") = %q, want empty", got)
	}
}

func TestParseListCommand(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr string
	}{
		{input: "add cleanup", want: []string{"browse", "--query", "cleanup"}},
		{input: "remove cleanup", want: []string{"remove", "cleanup"}},
		{input: "sync", want: []string{"sync"}},
		{input: "help", want: []string{"browse", "--help"}},
		{input: "add --registry acme", wantErr: "flags are not supported"},
		{input: "", wantErr: "empty command"},
	}

	for _, tt := range tests {
		got, err := parseListCommand(tt.input)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseListCommand(%q) error = %v, want substring %q", tt.input, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseListCommand(%q) error = %v", tt.input, err)
		}
		if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
			t.Fatalf("parseListCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestUpdateCommandMode_UnknownCommandShowsErrorAndExitsMode(t *testing.T) {
	m := listModel{
		commandMode:  true,
		commandInput: "whatever",
	}

	nm, cmd := m.updateCommandMode(key("enter"))
	if cmd != nil {
		t.Fatal("unknown command should not start a subprocess")
	}
	lm := nm.(listModel)
	if lm.commandMode {
		t.Fatal("command mode should exit after submitting an unknown command")
	}
	if !strings.Contains(lm.statusMsg, "unknown command") {
		t.Fatalf("statusMsg = %q, want unknown-command error", lm.statusMsg)
	}
}

func TestUpdateList_ColonEntersCommandModeAndAcceptsTypedText(t *testing.T) {
	m := listModel{
		rows:        []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		filtered:    []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		groupCounts: map[string]int{"artistfy/hq": 1},
	}

	nm, _ := m.updateList(tea.KeyPressMsg{Text: ":"})
	lm := nm.(listModel)
	if !lm.commandMode {
		t.Fatal("typing ':' should enter command mode")
	}

	nm, _ = lm.updateCommandMode(tea.KeyPressMsg{Text: "x"})
	lm = nm.(listModel)
	if lm.commandInput != "x" {
		t.Fatalf("commandInput = %q, want x", lm.commandInput)
	}
}

func TestUpdateList_SlashShowsSearchPromptImmediately(t *testing.T) {
	m := listModel{
		rows:        []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		filtered:    []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		groupCounts: map[string]int{"artistfy/hq": 1},
	}

	nm, _ := m.updateList(tea.KeyPressMsg{Text: "/"})
	lm := nm.(listModel)
	if !lm.searchMode {
		t.Fatal("typing '/' should enter search mode")
	}
	if got := lm.renderQueryLine(); got != "/ " {
		t.Fatalf("renderQueryLine() = %q, want %q", got, "/ ")
	}
}

func TestViewListFull_ShowsCommandModeHint(t *testing.T) {
	m := listModel{
		width:        80,
		commandMode:  true,
		commandInput: "whatever",
		rows:         []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		filtered:     []listRow{{Name: "alpha", Group: "artistfy/hq"}},
		groupCounts:  map[string]int{"artistfy/hq": 1},
		bag:          &workflow.Bag{},
	}

	view := m.viewListFull()
	if !strings.Contains(view, "Command mode") {
		t.Fatalf("view missing command-mode hint:\n%s", view)
	}
	if !strings.Contains(view, "enter run") || !strings.Contains(view, "esc cancel") {
		t.Fatalf("view missing command-mode controls:\n%s", view)
	}
}

func TestRenderQueryLine_CommandModeShowsVisiblePrompt(t *testing.T) {
	m := listModel{commandMode: true}

	got := m.renderQueryLine()
	if !strings.Contains(got, ":") {
		t.Fatalf("renderQueryLine() = %q, want visible ':' prompt", got)
	}
	if strings.Contains(got, "command...") {
		t.Fatalf("renderQueryLine() = %q, should not hide behind placeholder copy", got)
	}
}

func TestContentHeight_AccountsForListChrome(t *testing.T) {
	m := listModel{
		height:         30,
		backgroundLoad: true,
		bag:            &workflow.Bag{},
	}

	if got := m.contentHeight(); got != 22 {
		t.Fatalf("contentHeight() = %d, want 22", got)
	}
}

func TestViewListFull_DoesNotOverflowViewportAndKeepsTopChrome(t *testing.T) {
	rows := make([]listRow, 0, 24)
	for i := 0; i < 24; i++ {
		rows = append(rows, listRow{Name: "skill-" + string(rune('a'+(i%26))), Group: "artistfy/hq"})
	}

	m := listModel{
		width:       80,
		height:      20,
		commandMode: true,
		rows:        rows,
		filtered:    rows,
		groupCounts: map[string]int{"artistfy/hq": len(rows)},
		bag:         &workflow.Bag{},
	}

	view := strings.TrimRight(m.viewListFull(), "\n")
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("view rendered %d lines for height %d:\n%s", len(lines), m.height, view)
	}
	if !strings.Contains(view, "Installed Skills") {
		t.Fatalf("view missing header:\n%s", view)
	}
	if !strings.Contains(view, ":") {
		t.Fatalf("view missing visible command prompt:\n%s", view)
	}
}

func TestEnsureCursorVisible_ReservesSlotsForMoreAboveAndBelowIndicators(t *testing.T) {
	// Regression: ensureCursorVisible must mirror renderRows' budget so the
	// cursor row is actually drawn. Before the fix, the top/bottom "↑/↓ more"
	// indicators were not subtracted from the viewport, so the cursor could
	// sit one or two rows past the last rendered line and the list appeared
	// frozen on arrow presses until offset caught up.
	rows := make([]listRow, 80)
	for i := range rows {
		rows[i] = listRow{Name: "s", Group: "g"}
	}
	m := listModel{
		width:    80,
		height:   27, // contentHeight = 27 - 7 = 20
		rows:     rows,
		filtered: rows,
		cursor:   20,
	}
	m = m.ensureCursorVisible()

	if !m.cursorFitsAt(m.offset, m.contentHeight()) {
		t.Fatalf("cursor (%d) does not fit at offset %d with viewport %d", m.cursor, m.offset, m.contentHeight())
	}
	if m.offset == 0 {
		t.Fatalf("offset should have advanced past 0 to keep cursor visible, got offset=%d", m.offset)
	}
}

func TestEnsureCursorVisible_CursorArrowAlwaysRendered(t *testing.T) {
	// Regression for #486: pressing ↓ past the visible region must scroll the
	// viewport so the focused-row arrow is actually drawn. Renders the rows
	// and asserts the cursor marker is present in the output for every cursor
	// position from 0..N-1.
	cases := []struct {
		name        string
		rows        []listRow
		groupCounts map[string]int
		height      int
	}{
		{
			name: "single group",
			rows: func() []listRow {
				rs := make([]listRow, 50)
				for i := range rs {
					rs[i] = listRow{Name: "s", Group: "g"}
				}
				return rs
			}(),
			groupCounts: map[string]int{"g": 50},
			height:      27,
		},
		{
			name: "multi group",
			rows: func() []listRow {
				rs := make([]listRow, 0, 60)
				for g := 0; g < 3; g++ {
					group := string(rune('A' + g))
					for i := 0; i < 20; i++ {
						rs = append(rs, listRow{Name: "s", Group: group})
					}
				}
				return rs
			}(),
			groupCounts: map[string]int{"A": 20, "B": 20, "C": 20},
			height:      27,
		},
		{
			name: "mixed empty and non-empty groups",
			rows: func() []listRow {
				// 25 unmanaged (Group="") + 25 grouped — exposes the
				// header-budget divergence between cursorFitsAt and
				// renderRows for empty group transitions.
				rs := make([]listRow, 0, 50)
				for i := 0; i < 25; i++ {
					rs = append(rs, listRow{Name: "s", Group: "owner/repo"})
				}
				for i := 0; i < 25; i++ {
					rs = append(rs, listRow{Name: "s"})
				}
				return rs
			}(),
			groupCounts: map[string]int{"owner/repo": 25},
			height:      27,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := listModel{
				width:       80,
				height:      tc.height,
				rows:        tc.rows,
				filtered:    tc.rows,
				groupCounts: tc.groupCounts,
			}
			for cursor := 0; cursor < len(tc.rows); cursor++ {
				m.cursor = cursor
				m = m.ensureCursorVisible()

				var sb strings.Builder
				m.renderRows(&sb, m.contentHeight(), m.width-4, false)
				out := sb.String()
				if !strings.Contains(out, "▸") {
					t.Fatalf("cursor=%d offset=%d: cursor arrow not rendered\n%s", cursor, m.offset, out)
				}
			}
		})
	}
}

func TestEnsureCursorVisible_LastRowSkipsBottomIndicator(t *testing.T) {
	// Regression for #486: when cursor is on the last row there is no
	// "↓ N more below" indicator, so cursorFitsAt must not reserve a slot
	// for it. Otherwise scrolling to the very end leaves cursor off-screen.
	rows := make([]listRow, 50)
	for i := range rows {
		rows[i] = listRow{Name: "s", Group: "g"}
	}
	m := listModel{
		width:       80,
		height:      27,
		rows:        rows,
		filtered:    rows,
		cursor:      49,
		groupCounts: map[string]int{"g": 50},
	}
	m = m.ensureCursorVisible()

	var sb strings.Builder
	m.renderRows(&sb, m.contentHeight(), m.width-4, false)
	out := sb.String()
	if !strings.Contains(out, "▸") {
		t.Fatalf("cursor arrow missing at last row\n%s", out)
	}
	if strings.Contains(out, "↓ ") {
		t.Fatalf("bottom indicator should not appear when cursor is on last row\n%s", out)
	}
}

func TestEnsureCursorVisible_FirstRowResetsOffset(t *testing.T) {
	// Regression for #486: pressing Home (or scrolling to top) must reset
	// the offset back to 0, not leave it stale where the cursor is no
	// longer in view.
	rows := make([]listRow, 50)
	for i := range rows {
		rows[i] = listRow{Name: "s", Group: "g"}
	}
	m := listModel{
		width:       80,
		height:      27,
		rows:        rows,
		filtered:    rows,
		cursor:      0,
		offset:      20,
		groupCounts: map[string]int{"g": 50},
	}
	m = m.ensureCursorVisible()
	if m.offset != 0 {
		t.Fatalf("offset should reset to 0 when cursor is at top, got %d", m.offset)
	}
}

func TestViewListFull_PadsContentToPinSummaryToBottom(t *testing.T) {
	// Regression: the summary + help footer must sit at the bottom of the
	// terminal, not float mid-screen when the filtered list is shorter than
	// the viewport.
	rows := []listRow{
		{Name: "alpha", Group: "local"},
		{Name: "beta", Group: "local"},
	}
	m := listModel{
		width:       80,
		height:      25,
		rows:        rows,
		filtered:    rows,
		groupCounts: map[string]int{"local": 2},
		bag:         &workflow.Bag{},
	}

	view := m.viewListFull()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("view rendered %d lines, want %d (full viewport):\n%s", len(lines), m.height, view)
	}
	// Commands help is the very last line when pinned to the terminal bottom.
	if !strings.Contains(lines[len(lines)-1], "Commands:") {
		t.Fatalf("last line should be the Commands help, got %q", lines[len(lines)-1])
	}
}

func TestRefreshFilteredBuildsGroupCounts(t *testing.T) {
	m := listModel{
		rows: []listRow{
			{Name: "alpha", Group: "g1"},
			{Name: "beta", Group: "g1"},
			{Name: "delta", Group: "g1"},
			{Name: "gamma", Group: "g2"},
		},
		search: "elt",
		cursor: 2,
		offset: 1,
	}

	m = m.refreshFiltered()

	if got := len(m.filtered); got != 1 {
		t.Fatalf("filtered len = %d, want 1", got)
	}
	if got := m.filtered[0].Name; got != "delta" {
		t.Fatalf("filtered[0].Name = %q, want %q", got, "delta")
	}
	if got := m.groupCounts["g1"]; got != 1 {
		t.Fatalf("groupCounts[g1] = %d, want 1", got)
	}
	if got := m.groupCounts["g2"]; got != 0 {
		t.Fatalf("groupCounts[g2] = %d, want 0", got)
	}
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("cursor/offset = %d/%d, want 0/0", m.cursor, m.offset)
	}
}

func TestRefreshFiltered_BrowseSkipsCurrentRows(t *testing.T) {
	m := listModel{
		bag: &workflow.Bag{BrowseFlag: true},
		rows: []listRow{
			{Name: "current", Group: "acme/skills", Status: sync.StatusCurrent},
			{Name: "missing", Group: "acme/skills", Status: sync.StatusMissing},
			{Name: "outdated", Group: "acme/skills", Status: sync.StatusOutdated},
		},
	}

	m = m.refreshFiltered()

	if got := len(m.filtered); got != 2 {
		t.Fatalf("filtered len = %d, want 2", got)
	}
	for _, row := range m.filtered {
		if row.Status == sync.StatusCurrent {
			t.Fatalf("current row leaked into browse results: %+v", row)
		}
	}
}

func TestRefreshFiltered_BrowseSkipsLocallyInstalledRows(t *testing.T) {
	m := listModel{
		bag: &workflow.Bag{BrowseFlag: true},
		rows: []listRow{
			{
				Name:   "installed",
				Group:  "acme/skills",
				Status: sync.StatusMissing,
				Local:  &discovery.Skill{Name: "installed", LocalPath: "/tmp/installed"},
			},
			{
				Name:   "remote-only",
				Group:  "acme/skills",
				Status: sync.StatusMissing,
			},
		},
	}

	m = m.refreshFiltered()

	if got := len(m.filtered); got != 1 {
		t.Fatalf("filtered len = %d, want 1", got)
	}
	if m.filtered[0].Name != "remote-only" {
		t.Fatalf("filtered[0].Name = %q, want remote-only", m.filtered[0].Name)
	}
}

func TestViewListFull_ShowsCommandHelpInListMode(t *testing.T) {
	m := listModel{
		width: 80,
		rows: []listRow{
			{Name: "alpha", Group: "local"},
		},
		filtered: []listRow{
			{Name: "alpha", Group: "local"},
		},
		groupCounts: map[string]int{"local": 1},
		bag:         &workflow.Bag{},
	}

	view := m.viewListFull()
	if !strings.Contains(view, ":commands") {
		t.Fatalf("view missing :commands hint:\n%s", view)
	}
	if !strings.Contains(view, "Commands: :add <query> · :sync · :remove <name> · :help") {
		t.Fatalf("view missing command reference:\n%s", view)
	}
}

func TestViewListFull_ShowsBackgroundRegistryIndicator(t *testing.T) {
	m := listModel{
		width: 80,
		rows: []listRow{
			{Name: "alpha", Group: "artistfy/hq"},
		},
		filtered: []listRow{
			{Name: "alpha", Group: "artistfy/hq"},
		},
		groupCounts:    map[string]int{"artistfy/hq": 1},
		backgroundLoad: true,
		spinnerFrame:   0,
		bag:            &workflow.Bag{},
	}

	view := m.viewListFull()
	if !strings.Contains(view, "checking registry updates in background...") {
		t.Fatalf("view missing background indicator:\n%s", view)
	}
}

func TestRenderHeader_UsesBrowseTitle(t *testing.T) {
	m := listModel{
		width: 80,
		rows:  []listRow{{Name: "alpha", Group: "acme/skills"}},
		bag:   &workflow.Bag{BrowseFlag: true},
	}

	header := m.renderHeader()
	if !strings.Contains(header, "Browse Skills") {
		t.Fatalf("header = %q, want browse title", header)
	}
}

func findAction(actions []actionItem, key string) actionItem {
	for _, a := range actions {
		if a.key == key {
			return a
		}
	}
	return actionItem{}
}

// detailModel builds a listModel with two filtered rows in the detail state,
// ready for exercising updateDetail.
func detailModel(focus detailFocus) listModel {
	rows := []listRow{
		{Name: "a", Group: "g1", Local: &discovery.Skill{Name: "a", LocalPath: "/p/a"}},
		{Name: "b", Group: "g2", Local: &discovery.Skill{Name: "b", LocalPath: "/p/b"}},
	}
	return listModel{
		selected: true,
		focus:    focus,
		cursor:   0,
		rows:     rows,
		filtered: rows,
	}
}

func TestUpdateDetail_FocusToggle(t *testing.T) {
	cases := []struct {
		name      string
		start     detailFocus
		keyLabel  string
		wantFocus detailFocus
	}{
		{"tab actions->list", focusActions, "tab", focusList},
		{"tab list->actions", focusList, "tab", focusActions},
		{"shift+tab actions->list", focusActions, "shift+tab", focusList},
		{"shift+tab list->actions", focusList, "shift+tab", focusActions},
		{"right from list -> actions", focusList, "right", focusActions},
		{"l from list -> actions", focusList, "l", focusActions},
		{"enter from list -> actions", focusList, "enter", focusActions},
		{"left from actions -> list", focusActions, "left", focusList},
		{"h from actions -> list", focusActions, "h", focusList},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := detailModel(tc.start)
			nm, _ := m.updateDetail(key(tc.keyLabel))
			lm := nm.(listModel)
			if lm.focus != tc.wantFocus {
				t.Fatalf("focus = %v, want %v", lm.focus, tc.wantFocus)
			}
		})
	}
}

func TestUpdateDetail_EscapeResetsFocus(t *testing.T) {
	m := detailModel(focusActions)
	m.actionCursor = 3
	m.statusMsg = "stale"
	nm, _ := m.updateDetail(key("esc"))
	lm := nm.(listModel)
	if lm.selected {
		t.Error("esc should clear selected")
	}
	if lm.focus != focusList {
		t.Errorf("focus = %v, want focusList", lm.focus)
	}
	if lm.actionCursor != 0 {
		t.Errorf("actionCursor = %d, want 0", lm.actionCursor)
	}
	if lm.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty", lm.statusMsg)
	}
}

func TestUpdateDetail_FocusListMovesRowCursor(t *testing.T) {
	m := detailModel(focusList)
	m.actionCursor = 2

	nm, _ := m.updateDetail(key("j"))
	lm := nm.(listModel)
	if lm.cursor != 1 {
		t.Fatalf("after j: cursor = %d, want 1", lm.cursor)
	}
	if lm.actionCursor != 0 {
		t.Fatalf("after j: actionCursor = %d, want 0 (reset on row change)", lm.actionCursor)
	}

	nm, _ = lm.updateDetail(key("j"))
	lm = nm.(listModel)
	if lm.cursor != 1 {
		t.Fatalf("j at last row should be no-op, cursor = %d, want 1", lm.cursor)
	}

	nm, _ = lm.updateDetail(key("k"))
	lm = nm.(listModel)
	if lm.cursor != 0 {
		t.Fatalf("after k: cursor = %d, want 0", lm.cursor)
	}
}

func TestUpdateDetail_FocusActionsMovesActionCursor(t *testing.T) {
	m := detailModel(focusActions)
	m.cursor = 0

	nm, _ := m.updateDetail(key("j"))
	lm := nm.(listModel)
	if lm.cursor != 0 {
		t.Errorf("focusActions j should not move row cursor, got %d", lm.cursor)
	}
	if lm.actionCursor != 1 {
		t.Errorf("focusActions j should advance actionCursor, got %d", lm.actionCursor)
	}
}

func TestUpdateDetail_FocusListTypingFiltersRows(t *testing.T) {
	m := detailModel(focusList)

	nm, _ := m.updateDetail(key("b"))
	lm := nm.(listModel)
	if lm.search != "b" {
		t.Fatalf("search = %q, want %q", lm.search, "b")
	}
	if len(lm.filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(lm.filtered))
	}
	if lm.filtered[0].Name != "b" {
		t.Fatalf("filtered[0] = %q, want %q", lm.filtered[0].Name, "b")
	}
	if !lm.selected {
		t.Fatal("detail pane should stay open when search still has matches")
	}
	if lm.focus != focusList {
		t.Fatalf("focus = %v, want focusList", lm.focus)
	}
}

func TestUpdateDetail_FocusListColonEntersCommandMode(t *testing.T) {
	m := detailModel(focusList)

	nm, _ := m.updateDetail(tea.KeyPressMsg{Text: ":"})
	lm := nm.(listModel)
	if !lm.commandMode {
		t.Fatal("typing ':' in focusList detail mode should enter command mode")
	}
}

func TestUpdateDetail_FocusListBackspaceUpdatesSearch(t *testing.T) {
	m := detailModel(focusList)
	m.search = "b"
	m.filtered = []listRow{
		{Name: "b", Group: "g2", Local: &discovery.Skill{Name: "b", LocalPath: "/p/b"}},
	}

	nm, _ := m.updateDetail(key("backspace"))
	lm := nm.(listModel)
	if lm.search != "" {
		t.Fatalf("search = %q, want empty", lm.search)
	}
	if len(lm.filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(lm.filtered))
	}
	if !lm.selected {
		t.Fatal("detail pane should stay open after restoring matching rows")
	}
}

func TestUpdateDetail_FocusListTypingNoMatchesClosesDetail(t *testing.T) {
	m := detailModel(focusList)

	nm, _ := m.updateDetail(key("z"))
	lm := nm.(listModel)
	if lm.search != "z" {
		t.Fatalf("search = %q, want %q", lm.search, "z")
	}
	if len(lm.filtered) != 0 {
		t.Fatalf("filtered len = %d, want 0", len(lm.filtered))
	}
	if lm.selected {
		t.Fatal("detail pane should close when the current search removes all rows")
	}
}

func TestBuildLocalRowsExcluding_DedupsSlugQualified(t *testing.T) {
	skills := []discovery.Skill{
		{Name: "Artistfy-hq/ascii", Package: "Artistfy-hq", LocalPath: "/home/u/.scribe/skills/Artistfy-hq/ascii"},
		{Name: "other", Package: ""},
	}
	// Simulate buildRows having marked the slug-qualified form as matched
	// (because a registry row was emitted for it).
	matched := map[string]bool{
		"Artistfy-hq/ascii": true,
	}
	rows := workflow.BuildLocalRowsExcluding(skills, matched, &state.State{Installed: map[string]state.InstalledSkill{}})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %+v", len(rows), rows)
	}
	if rows[0].Name != "other" {
		t.Errorf("expected only 'other' to survive, got %q", rows[0].Name)
	}
}

func TestBuildLocalRowsExcluding_PreservesUnmatchedSameName(t *testing.T) {
	// A bare-name local skill should NOT be suppressed by a registry row
	// that matched only its slug-qualified sibling. The new lookup order
	// only marks the key that actually matched.
	skills := []discovery.Skill{
		{Name: "ascii", Package: ""},
	}
	matched := map[string]bool{
		"Artistfy-hq/ascii": true, // bare "ascii" NOT in matched set
	}
	rows := workflow.BuildLocalRowsExcluding(skills, matched, &state.State{Installed: map[string]state.InstalledSkill{}})
	if len(rows) != 1 {
		t.Fatalf("expected bare-name 'ascii' to survive, got %d rows", len(rows))
	}
	if rows[0].Name != "ascii" {
		t.Errorf("row name = %q, want 'ascii'", rows[0].Name)
	}
}

func TestExecuteActionUpdate_PackageUsesExecutor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	markerFile := filepath.Join(home, "updated.txt")
	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"pkg-skill": {
				Type: "package",
			},
		},
	}

	m := listModel{
		ctx: context.Background(),
		bag: &workflow.Bag{
			State: st,
			Tools: nil,
		},
		filtered: []listRow{
			{
				Name:      "pkg-skill",
				Group:     "owner/repo",
				Status:    sync.StatusOutdated,
				HasStatus: true,
				Entry: &manifest.Entry{
					Name:   "pkg-skill",
					Source: "github:owner/repo@main",
					Type:   manifest.EntryTypePackage,
					Update: "printf updated > " + markerFile,
				},
				Local: &discovery.Skill{Name: "pkg-skill", LocalPath: filepath.Join(home, ".scribe", "skills", "pkg-skill")},
			},
		},
	}

	nm, cmd := m.executeAction("update")
	if cmd != nil {
		t.Fatal("expected update to prompt before running")
	}
	lm := nm.(listModel)
	if lm.substate != listSubstateUpdateChoice {
		t.Fatalf("substate = %v, want listSubstateUpdateChoice", lm.substate)
	}
	if lm.updateHasMods {
		t.Fatal("package update without local edits should not offer merge choices")
	}

	_, cmd = lm.updateUpdateChoice(key("u"))
	if cmd == nil {
		t.Fatal("expected update command after confirmation")
	}
	msg := cmd()
	done, ok := msg.(updateDoneMsg)
	if !ok {
		t.Fatalf("expected updateDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("update returned error: %v", done.err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("expected update command to write marker file: %v", err)
	}
	if string(data) != "updated" {
		t.Errorf("marker content = %q, want %q", string(data), "updated")
	}
}

func TestExecuteActionUpdate_ModifiedSkillPromptsForStrategy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".scribe", "skills", "recap")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("local change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		SchemaVersion: 2,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				InstalledHash: sync.ComputeFileHash([]byte("upstream base\n")),
			},
		},
	}

	m := listModel{
		bag: &workflow.Bag{State: st},
		filtered: []listRow{
			{
				Name:      "recap",
				Group:     "owner/repo",
				Status:    sync.StatusOutdated,
				HasStatus: true,
				Entry: &manifest.Entry{
					Name:   "recap",
					Source: "github:owner/repo@main",
				},
				Local: &discovery.Skill{Name: "recap", LocalPath: skillDir},
			},
		},
	}

	nm, cmd := m.executeAction("update")
	if cmd != nil {
		t.Fatal("expected modified skill update to wait for user choice")
	}
	lm := nm.(listModel)
	if lm.substate != listSubstateUpdateChoice {
		t.Fatalf("substate = %v, want listSubstateUpdateChoice", lm.substate)
	}
	if lm.statusMsg == "" {
		t.Fatal("expected update choice explanation")
	}
}

func TestRunUpdate_LoadsRemoteDepsInsteadOfPanicking(t *testing.T) {
	original := listEnsureRemoteDepsFn
	defer func() { listEnsureRemoteDepsFn = original }()

	listEnsureRemoteDepsFn = func(context.Context, *workflow.Bag) error {
		return context.Canceled
	}

	m := listModel{
		ctx: context.Background(),
		bag: &workflow.Bag{
			Config:     &config.Config{},
			State:      &state.State{Installed: map[string]state.InstalledSkill{"review-triage": {}}},
			LazyGitHub: true,
		},
		filtered: []listRow{
			{
				Name:      "review-triage",
				Group:     "artistfy/hq",
				Status:    sync.StatusOutdated,
				HasStatus: true,
				Entry: &manifest.Entry{
					Name:   "review-triage",
					Source: "github:artistfy/hq@main",
				},
				Local: &discovery.Skill{Name: "review-triage", LocalPath: "/tmp/review-triage"},
			},
		},
	}

	msg := m.runUpdate(updateChoiceMerge)()
	done, ok := msg.(updateDoneMsg)
	if !ok {
		t.Fatalf("runUpdate() returned %T, want updateDoneMsg", msg)
	}
	if done.err == nil {
		t.Fatal("runUpdate should surface dependency-loader failure")
	}
}

func TestUpdateUpdateChoice_KeepLocalSkipsUpdate(t *testing.T) {
	m := listModel{
		substate:      listSubstateUpdateChoice,
		statusMsg:     "Local edits detected.",
		updateHasMods: true,
	}

	nm, cmd := m.updateUpdateChoice(key("l"))
	if cmd != nil {
		t.Fatal("keep local should not start an update command")
	}
	lm := nm.(listModel)
	if lm.substate != listSubstateNone {
		t.Fatalf("substate = %v, want listSubstateNone", lm.substate)
	}
	if lm.statusMsg != "Kept local version. Registry update skipped." {
		t.Fatalf("statusMsg = %q", lm.statusMsg)
	}
}

func TestRowHasLocalModifications_UsesDiscoveredSkillState(t *testing.T) {
	row := listRow{
		Name: "recap",
		Local: &discovery.Skill{
			Name:      "recap",
			LocalPath: "/some/non-canonical/path",
			Modified:  true,
		},
	}

	if !rowHasLocalModifications(row, &state.State{Installed: map[string]state.InstalledSkill{}}) {
		t.Fatal("expected discovered Modified=true to trigger update choice")
	}
}

func TestFormatRow_UnmanagedMarker(t *testing.T) {
	m := listModel{width: 120}

	unmanagedRow := listRow{
		Name:    "bare-skill",
		Group:   "Local (unmanaged)",
		Managed: false,
		Local:   &discovery.Skill{Name: "bare-skill", LocalPath: "/home/user/.claude/skills/bare-skill"},
	}
	managedRow := listRow{
		Name:    "managed-skill",
		Group:   "owner/repo",
		Managed: true,
		Local:   &discovery.Skill{Name: "managed-skill", LocalPath: "/home/user/.scribe/skills/managed-skill"},
	}

	nameCol := 20

	unmanagedFormatted := m.formatRow(unmanagedRow, false, nameCol, true)
	if !strings.Contains(unmanagedFormatted, "[unmanaged]") {
		t.Errorf("unmanaged row should contain [unmanaged] marker, got: %q", unmanagedFormatted)
	}

	managedFormatted := m.formatRow(managedRow, false, nameCol, true)
	if strings.Contains(managedFormatted, "[unmanaged]") {
		t.Errorf("managed row should not contain [unmanaged] marker, got: %q", managedFormatted)
	}
}

func TestFormatRow_LocalRowsDoNotShowPlaceholderColumns(t *testing.T) {
	m := listModel{width: 120}
	row := listRow{
		Name:    "recap",
		Group:   "artistfy/hq",
		Managed: true,
		Origin:  state.OriginRegistry,
		Local:   &discovery.Skill{Name: "recap", LocalPath: "/home/user/.scribe/skills/recap"},
	}

	formatted := m.formatRow(row, false, 20, false)
	if strings.Contains(formatted, "  -  -") || strings.Count(formatted, " -") >= 2 {
		t.Fatalf("local row should not contain placeholder columns, got: %q", formatted)
	}
}

func TestFormatRow_LocalRowsShowSourceAttributionMarker(t *testing.T) {
	m := listModel{width: 120}
	row := listRow{
		Name:    "recap",
		Managed: true,
		Local: &discovery.Skill{
			Name: "recap",
			Source: discovery.Source{
				Author: "acme",
			},
		},
		Source: discovery.Source{
			Author: "acme",
		},
	}

	formatted := m.formatRow(row, false, 20, false)
	if !strings.Contains(formatted, "ℹ️ (via acme)") {
		t.Fatalf("local row should show source marker, got: %q", formatted)
	}
}

func TestFormatRow_LocalRegistryRowsShowSkeletonWhileBackgroundLoading(t *testing.T) {
	m := listModel{width: 120, backgroundLoad: true, spinnerFrame: 0}
	row := listRow{
		Name:    "recap",
		Group:   "artistfy/hq",
		Managed: true,
		Origin:  state.OriginRegistry,
		Local:   &discovery.Skill{Name: "recap", LocalPath: "/home/user/.scribe/skills/recap"},
	}

	formatted := m.formatRow(row, false, 20, false)
	if !strings.Contains(formatted, "░") && !strings.Contains(formatted, "▒") {
		t.Fatalf("local registry row should show loading skeleton, got: %q", formatted)
	}
	if !strings.Contains(formatted, " ") {
		t.Fatalf("local registry row skeleton should be segmented, got: %q", formatted)
	}
}

func TestFormatRow_RemoteRowsKeepStatusColumns(t *testing.T) {
	m := listModel{width: 120}
	row := listRow{
		Name:      "recap",
		Group:     "artistfy/hq",
		Managed:   true,
		HasStatus: true,
		Status:    sync.StatusCurrent,
		Version:   "v1.2.3",
		Author:    "artistfy",
	}

	formatted := m.formatRow(row, false, 20, false)
	if !strings.Contains(formatted, "v1.2.3") {
		t.Fatalf("remote row missing version column: %q", formatted)
	}
	if !strings.Contains(formatted, "artistfy") {
		t.Fatalf("remote row missing author column: %q", formatted)
	}
}

func TestRegistriesForBackgroundCheck_UsesInstalledRegistrySourcesOnly(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "artistfy/hq", Enabled: true},
			{Repo: "other/skills", Enabled: true},
			{Repo: "unused/skills", Enabled: true},
		},
	}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Origin: state.OriginRegistry,
				Sources: []state.SkillSource{
					{Registry: "artistfy/hq"},
					{Registry: "other/skills"},
				},
			},
			"local-only": {
				Origin: state.OriginLocal,
			},
		},
	}

	got := registriesForBackgroundCheck(cfg, st)
	want := []string{"artistfy/hq", "other/skills"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("registriesForBackgroundCheck() = %v, want %v", got, want)
	}
}

func TestApplyRegistryStatuses_UpdatesCachedRegistryRows(t *testing.T) {
	m := listModel{
		rows: []listRow{
			{
				Name:    "recap",
				Group:   "artistfy/hq",
				Managed: true,
				Origin:  state.OriginRegistry,
				Local:   &discovery.Skill{Name: "recap", LocalPath: "/tmp/recap"},
			},
		},
		filtered: []listRow{
			{
				Name:    "recap",
				Group:   "artistfy/hq",
				Managed: true,
				Origin:  state.OriginRegistry,
				Local:   &discovery.Skill{Name: "recap", LocalPath: "/tmp/recap"},
			},
		},
		groupCounts: map[string]int{"artistfy/hq": 1},
	}

	m = m.applyRegistryStatuses(map[string][]sync.SkillStatus{
		"artistfy/hq": {{
			Name:       "recap",
			Status:     sync.StatusCurrent,
			LoadoutRef: "v1.2.3",
			Maintainer: "artistfy",
			Entry:      &manifest.Entry{Name: "recap"},
		}},
	})

	if !m.rows[0].HasStatus {
		t.Fatal("row should be marked as having status after background diff")
	}
	if m.rows[0].Status != sync.StatusCurrent {
		t.Fatalf("row status = %v, want current", m.rows[0].Status)
	}
	if m.rows[0].Version != "v1.2.3" {
		t.Fatalf("row version = %q, want v1.2.3", m.rows[0].Version)
	}
	if m.rows[0].Author != "artistfy" {
		t.Fatalf("row author = %q, want artistfy", m.rows[0].Author)
	}
}

func TestRegistryStatusCache_ReusesFreshStatuses(t *testing.T) {
	oldNow := nowFn
	defer func() { nowFn = oldNow }()
	registryStatusCache.mu.Lock()
	oldItems := registryStatusCache.items
	registryStatusCache.items = map[string]cachedRegistryStatuses{}
	registryStatusCache.mu.Unlock()
	defer func() {
		registryStatusCache.mu.Lock()
		registryStatusCache.items = oldItems
		registryStatusCache.mu.Unlock()
	}()

	base := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return base }

	want := []sync.SkillStatus{{Name: "recap", Status: sync.StatusCurrent}}
	storeCachedRegistryStatuses("artistfy/hq", want)

	got, ok := loadCachedRegistryStatuses("artistfy/hq")
	if !ok {
		t.Fatal("expected fresh cache hit")
	}
	if len(got) != 1 || got[0].Name != "recap" {
		t.Fatalf("cached statuses = %+v, want recap", got)
	}

	nowFn = func() time.Time { return base.Add(registryStatusCacheTTL + time.Second) }
	if _, ok := loadCachedRegistryStatuses("artistfy/hq"); ok {
		t.Fatal("expected expired cache miss")
	}
}

func TestRenderDetailPane_UnmanagedHint(t *testing.T) {
	m := listModel{width: 80}

	unmanagedRow := listRow{
		Name:    "bare-skill",
		Group:   "Local (unmanaged)",
		Managed: false,
		Local:   &discovery.Skill{Name: "bare-skill", LocalPath: "/home/user/.claude/skills/bare-skill"},
	}

	out := m.renderDetailPane(unmanagedRow, 60)

	if !strings.Contains(out, "Managed") || !strings.Contains(out, "no") {
		t.Errorf("detail pane for unmanaged skill should contain 'Managed' and 'no', got:\n%s", out)
	}
	if !strings.Contains(out, "scribe adopt bare-skill") {
		t.Errorf("detail pane for unmanaged skill should contain 'scribe adopt bare-skill', got:\n%s", out)
	}
}

func TestRenderDetailPane_LocalOriginTag(t *testing.T) {
	m := listModel{width: 80}

	localRow := listRow{
		Name:    "my-custom-skill",
		Group:   "owner/repo",
		Managed: true,
		Origin:  state.OriginLocal,
		Local:   &discovery.Skill{Name: "my-custom-skill", LocalPath: "/home/user/.scribe/skills/my-custom-skill"},
	}

	out := m.renderDetailPane(localRow, 60)

	if !strings.Contains(out, "(local)") {
		t.Errorf("detail pane for local-origin skill should contain '(local)', got:\n%s", out)
	}
	// No adopt hint for managed rows.
	if strings.Contains(out, "scribe adopt") {
		t.Errorf("detail pane for managed skill should not contain adopt hint, got:\n%s", out)
	}
}

func TestRenderDetailPane_RegistryOriginNoLocalTag(t *testing.T) {
	m := listModel{width: 80}

	registryRow := listRow{
		Name:    "registry-skill",
		Group:   "owner/repo",
		Managed: true,
		Origin:  state.OriginRegistry,
		Local:   &discovery.Skill{Name: "registry-skill", LocalPath: "/home/user/.scribe/skills/registry-skill"},
	}

	out := m.renderDetailPane(registryRow, 60)

	if strings.Contains(out, "(local)") {
		t.Errorf("detail pane for registry-origin skill should not contain '(local)', got:\n%s", out)
	}
}

func TestRenderDetailPane_UsesRegistryDescriptionForBrowseRows(t *testing.T) {
	m := listModel{
		width: 80,
		bag:   &workflow.Bag{BrowseFlag: true},
	}

	row := listRow{
		Name:   "registry-skill",
		Group:  "owner/repo",
		Status: sync.StatusMissing,
		Entry: &manifest.Entry{
			Name:        "registry-skill",
			Description: "Summarizes a repository and proposes follow-up cleanup steps.",
		},
	}

	out := m.renderDetailPane(row, 60)

	if !strings.Contains(out, "Summarizes a repository and proposes follow-up cleanup") {
		t.Fatalf("detail pane missing registry description:\n%s", out)
	}
}

func TestRenderDetailPane_ShowsInlineUpdateChoices(t *testing.T) {
	m := listModel{
		width:         80,
		substate:      listSubstateUpdateChoice,
		updateHasMods: false,
		statusMsg:     "No local edits detected. Update will replace the local copy with the registry version.",
	}
	row := listRow{
		Name:      "review-triage",
		Group:     "artistfy/hq",
		Managed:   true,
		HasStatus: true,
		Status:    sync.StatusOutdated,
		Local:     &discovery.Skill{Name: "review-triage", LocalPath: "/tmp/review-triage"},
	}

	out := m.renderDetailPane(row, 60)
	if !strings.Contains(out, "[u] update now") {
		t.Fatalf("detail pane missing inline update action:\n%s", out)
	}
	if !strings.Contains(out, "[esc] cancel") {
		t.Fatalf("detail pane missing inline cancel action:\n%s", out)
	}
}

func TestRenderDetailPane_StripsMarkdownMarkersFromExcerpt(t *testing.T) {
	m := listModel{width: 80}
	row := listRow{
		Name:    "init-command",
		Managed: true,
		Local:   &discovery.Skill{Name: "init-command", LocalPath: "/tmp/init-command"},
		Excerpt: "# Add New Init Command\n**Reference**: follow the `/init-conventions` skill.",
	}

	out := m.renderDetailPane(row, 60)

	if strings.Contains(out, "# Add New Init Command") {
		t.Fatalf("excerpt should not render markdown headings literally:\n%s", out)
	}
	if strings.Contains(out, "**Reference**") {
		t.Fatalf("excerpt should not render markdown emphasis literally:\n%s", out)
	}
	if strings.Contains(out, "`/init-conventions`") {
		t.Fatalf("excerpt should not render markdown code ticks literally:\n%s", out)
	}
	if !strings.Contains(out, "Add New Init Command") {
		t.Fatalf("excerpt missing normalized heading text:\n%s", out)
	}
}

func TestRenderDetailPane_RendersMarkdownListsWithStructure(t *testing.T) {
	m := listModel{width: 80}
	row := listRow{
		Name:    "list-skill",
		Managed: true,
		Local:   &discovery.Skill{Name: "list-skill", LocalPath: "/tmp/list-skill"},
		Excerpt: "## Steps\n- gather context\n- write code\n1. run tests\n2. ship it",
	}

	out := m.renderDetailPane(row, 60)

	if !strings.Contains(out, "• gather context") {
		t.Fatalf("excerpt missing bullet rendering:\n%s", out)
	}
	if !strings.Contains(out, "1. run tests") {
		t.Fatalf("excerpt missing numbered-list rendering:\n%s", out)
	}
}

func TestExecuteActionTools_OpensToolsEditor(t *testing.T) {
	m := listModel{
		bag: &workflow.Bag{
			Config: &config.Config{},
			State: &state.State{
				Installed: map[string]state.InstalledSkill{
					"scribe-agent": {
						ToolsMode: state.ToolsModeInherit,
						Tools:     []string{"claude"},
					},
				},
			},
		},
		filtered: []listRow{
			{
				Name:    "scribe-agent",
				Managed: true,
				Local:   &discovery.Skill{Name: "scribe-agent", LocalPath: "/tmp/scribe-agent"},
			},
		},
	}

	nm, cmd := m.executeAction("tools")
	if cmd != nil {
		t.Fatal("tools action should open an inline editor, not spawn a command")
	}
	lm := nm.(listModel)
	if lm.substate != listSubstateTools {
		t.Fatalf("substate = %v, want listSubstateTools", lm.substate)
	}
	if len(lm.toolStatuses) == 0 {
		t.Fatal("tools editor should load tool statuses")
	}
	if lm.toolCursor != 1 {
		t.Fatalf("toolCursor = %d, want first tool row selected", lm.toolCursor)
	}
}

func TestActivateToolsEditorCursor_TogglingInheritedToolPinsAndUnselects(t *testing.T) {
	m := listModel{
		toolCursor:   1,
		toolMode:     state.ToolsModeInherit,
		toolStatuses: []tools.Status{{Name: "claude", Enabled: true}},
		toolSelection: map[string]bool{
			"claude": true,
		},
	}

	nm, _ := m.activateToolsEditorCursor()
	lm := nm.(listModel)
	if lm.toolMode != state.ToolsModePinned {
		t.Fatalf("toolMode = %q, want pinned", lm.toolMode)
	}
	if lm.toolSelection["claude"] {
		t.Fatal("claude should be deselected after toggling from inherit mode")
	}
}

func TestUpdateToolsEditor_SpaceTogglesCurrentTool(t *testing.T) {
	m := listModel{
		substate:     listSubstateTools,
		toolCursor:   1,
		toolMode:     state.ToolsModePinned,
		toolStatuses: []tools.Status{{Name: "claude", Enabled: true}},
		toolSelection: map[string]bool{
			"claude": true,
		},
	}

	nm, _ := m.updateToolsEditor(tea.KeyPressMsg{Code: tea.KeySpace})
	lm := nm.(listModel)
	if lm.toolSelection["claude"] {
		t.Fatal("space should toggle the current tool off")
	}
}

func TestRenderDetailPane_ToolsEditor(t *testing.T) {
	m := listModel{
		width:      80,
		focus:      focusActions,
		substate:   listSubstateTools,
		toolCursor: 0,
		toolMode:   state.ToolsModePinned,
		toolStatuses: []tools.Status{
			{Name: "claude", Enabled: true, Detected: true, DetectKnown: true},
			{Name: "cursor", Enabled: false},
		},
		toolSelection: map[string]bool{"claude": true},
	}
	row := listRow{
		Name:    "scribe-agent",
		Managed: true,
		Local:   &discovery.Skill{Name: "scribe-agent", LocalPath: "/tmp/scribe-agent"},
	}

	out := m.renderDetailPane(row, 60)
	if !strings.Contains(out, "mode: toggle inherit/pinned") {
		t.Fatalf("tools editor missing mode line:\n%s", out)
	}
	if !strings.Contains(out, "[x] claude") {
		t.Fatalf("tools editor missing checked tool:\n%s", out)
	}
	if !strings.Contains(out, "save") || !strings.Contains(out, "cancel") {
		t.Fatalf("tools editor missing save/cancel:\n%s", out)
	}
}

func TestRenderDetailPane_ToolsEditorInheritUsesInheritedMarker(t *testing.T) {
	m := listModel{
		width:        80,
		focus:        focusActions,
		substate:     listSubstateTools,
		toolCursor:   1,
		toolMode:     state.ToolsModeInherit,
		toolStatuses: []tools.Status{{Name: "claude", Enabled: true}},
		toolSelection: map[string]bool{
			"claude": true,
		},
	}
	row := listRow{
		Name:    "scribe-agent",
		Managed: true,
		Local:   &discovery.Skill{Name: "scribe-agent", LocalPath: "/tmp/scribe-agent"},
	}

	out := m.renderDetailPane(row, 60)
	if !strings.Contains(out, "[~] claude") {
		t.Fatalf("inherit-mode tools should use inherited marker:\n%s", out)
	}
}

func TestRestoreSelection_PreservesFocusedSkillAfterReload(t *testing.T) {
	m := listModel{
		restoreName:   "review-triage",
		restoreGroup:  "artistfy/hq",
		restoreDetail: true,
		rows: []listRow{
			{Name: "ascii", Group: "artistfy/hq"},
			{Name: "review-triage", Group: "artistfy/hq"},
		},
	}

	m = m.refreshFiltered()
	m = m.restoreSelection()

	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	if !m.selected {
		t.Fatal("detail pane should stay open after reload")
	}
	if m.focus != focusActions {
		t.Fatalf("focus = %v, want focusActions", m.focus)
	}
}

func TestBuildLocalRows_ManagedAndOriginPopulated(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"adopted": {Origin: state.OriginLocal},
			"synced": {
				Origin: state.OriginRegistry,
				Sources: []state.SkillSource{{
					Registry: "artistfy/hq",
				}},
			},
		},
	}
	skills := []discovery.Skill{
		{Name: "adopted", Managed: true},
		{Name: "synced", Managed: true},
		{Name: "bare", Managed: false},
	}

	rows := workflow.BuildLocalRows(skills, st)

	byName := map[string]listRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}

	if !byName["adopted"].Managed {
		t.Error("adopted: Managed should be true")
	}
	if byName["adopted"].Origin != state.OriginLocal {
		t.Errorf("adopted: Origin = %q, want %q", byName["adopted"].Origin, state.OriginLocal)
	}
	if !byName["synced"].Managed {
		t.Error("synced: Managed should be true")
	}
	if byName["synced"].Origin != state.OriginRegistry {
		t.Errorf("synced: Origin = %q, want %q", byName["synced"].Origin, state.OriginRegistry)
	}
	if byName["synced"].Group != "artistfy/hq" {
		t.Errorf("synced: Group = %q, want %q", byName["synced"].Group, "artistfy/hq")
	}
	if byName["bare"].Managed {
		t.Error("bare: Managed should be false")
	}
	// bare skill has no state entry → Origin stays at zero value, not an explicit OriginRegistry assignment.
	if byName["bare"].Origin != "" {
		t.Errorf("bare row: expected zero-value origin, got %q", byName["bare"].Origin)
	}
}

// wrapText keeps long per-registry error messages and Diff errors inside the
// viewport. Regression: a single-line "anthropic/skills: parse loadout: ..."
// was bleeding past the right edge because viewError / warning rendering
// passed raw strings straight to lipgloss without any width-aware wrapping.
func TestWrapText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		width  int
		expect []string // each produced line
	}{
		{
			name:   "short line unchanged",
			input:  "hello",
			width:  20,
			expect: []string{"hello"},
		},
		{
			name:   "long line wraps into multiple lines within width",
			input:  strings.Repeat("x", 50),
			width:  10,
			expect: []string{strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 10)},
		},
		{
			name:   "existing newlines preserved",
			input:  "first\nsecond",
			width:  20,
			expect: []string{"first", "second"},
		},
		{
			name:   "zero width returns input untouched",
			input:  "anything",
			width:  0,
			expect: []string{"anything"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapText(tc.input, tc.width)
			lines := strings.Split(got, "\n")
			if len(lines) != len(tc.expect) {
				t.Fatalf("line count = %d (%q), want %d (%q)", len(lines), lines, len(tc.expect), tc.expect)
			}
			for i, line := range lines {
				if line != tc.expect[i] {
					t.Errorf("line %d = %q, want %q", i, line, tc.expect[i])
				}
			}
		})
	}
}
