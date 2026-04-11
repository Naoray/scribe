package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/workflow"
)

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

func TestActionsForRow(t *testing.T) {
	t.Run("row with local skill has copy and edit enabled", func(t *testing.T) {
		row := listRow{
			Name: "my-skill",
			Local: &discovery.Skill{
				Name:      "my-skill",
				LocalPath: "/home/user/.claude/skills/my-skill",
			},
		}
		actions := actionsForRow(row)

		if len(actions) != 5 {
			t.Fatalf("expected 5 actions, got %d", len(actions))
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
	})

	t.Run("row without local skill has actions disabled", func(t *testing.T) {
		row := listRow{Name: "ghost-skill", Local: nil}
		actions := actionsForRow(row)

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
		actions := actionsForRow(row)

		if !findAction(actions, "copy").disabled {
			t.Error("copy should be disabled when local path is empty")
		}
	})

	t.Run("update is always disabled", func(t *testing.T) {
		row := listRow{
			Name:  "any",
			Local: &discovery.Skill{Name: "any", LocalPath: "/some/path"},
		}
		if !findAction(actionsForRow(row), "update").disabled {
			t.Error("update action should always be disabled")
		}
	})

	t.Run("category is always disabled", func(t *testing.T) {
		row := listRow{
			Name:  "any",
			Local: &discovery.Skill{Name: "any", LocalPath: "/some/path"},
		}
		if !findAction(actionsForRow(row), "category").disabled {
			t.Error("category action should always be disabled")
		}
	})
}

func TestBuildLocalRows_GroupsAndOrders(t *testing.T) {
	skills := []discovery.Skill{
		{Name: "gstack/zeta"},
		{Name: "alpha"},
		{Name: "gstack/beta"},
		{Name: "local/gamma"},
	}
	rows := buildLocalRows(skills, &state.State{Installed: map[string]state.InstalledSkill{}})

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Registry groups alphabetical first, "Local (unmanaged)" last; rows
	// within a group sorted by name.
	want := []struct{ name, group string }{
		{"gstack/beta", "gstack"},
		{"gstack/zeta", "gstack"},
		{"alpha", "Local (unmanaged)"},
		{"local/gamma", "Local (unmanaged)"},
	}
	for i, w := range want {
		if rows[i].Name != w.name || rows[i].Group != w.group {
			t.Errorf("row %d = %+v, want name=%q group=%q", i, rows[i], w.name, w.group)
		}
	}
}

func TestRegistryGroupFromName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Artistfy-hq/deploy", "Artistfy-hq"},
		{"local/foo", "Local (unmanaged)"},
		{"bare", "Local (unmanaged)"},
		{"owner/skill-name", "owner"},
	}
	for _, tt := range tests {
		got := registryGroupFromName(tt.name)
		if got != tt.want {
			t.Errorf("registryGroupFromName(%q) = %q, want %q", tt.name, got, tt.want)
		}
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
	rows := buildLocalRowsExcluding(skills, matched, &state.State{Installed: map[string]state.InstalledSkill{}})
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
	rows := buildLocalRowsExcluding(skills, matched, &state.State{Installed: map[string]state.InstalledSkill{}})
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
