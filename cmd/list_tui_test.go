package cmd

import (
	"testing"

	"github.com/Naoray/scribe/internal/discovery"
)

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
		{Name: "zeta", Package: "gstack"},
		{Name: "alpha", Package: ""},
		{Name: "beta", Package: "gstack"},
		{Name: "gamma", Package: ""},
	}
	rows := buildLocalRows(skills)

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Uncategorized first, alphabetical within group.
	want := []struct{ name, group string }{
		{"alpha", "uncategorized"},
		{"gamma", "uncategorized"},
		{"beta", "gstack"},
		{"zeta", "gstack"},
	}
	for i, w := range want {
		if rows[i].Name != w.name || rows[i].Group != w.group {
			t.Errorf("row %d = %+v, want name=%q group=%q", i, rows[i], w.name, w.group)
		}
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
