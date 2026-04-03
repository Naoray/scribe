package cmd

import (
	"testing"

	"github.com/Naoray/scribe/internal/discovery"
)

func TestActionsForSkill(t *testing.T) {
	t.Run("local skill has copy and edit enabled", func(t *testing.T) {
		sk := discovery.Skill{
			Name:      "my-skill",
			LocalPath: "/home/user/.claude/skills/my-skill",
		}
		actions := actionsForSkill(sk)

		if len(actions) != 5 {
			t.Fatalf("expected 5 actions, got %d", len(actions))
		}

		// copy and edit should NOT be disabled
		copyAction := findAction(actions, "copy")
		if copyAction.disabled {
			t.Error("copy action should not be disabled for local skill")
		}

		editAction := findAction(actions, "edit")
		if editAction.disabled {
			t.Error("edit action should not be disabled for local skill")
		}
	})

	t.Run("ghost skill has copy and edit disabled", func(t *testing.T) {
		sk := discovery.Skill{
			Name:      "ghost-skill",
			LocalPath: "", // ghost — no local path
		}
		actions := actionsForSkill(sk)

		if len(actions) != 5 {
			t.Fatalf("expected 5 actions, got %d", len(actions))
		}

		copyAction := findAction(actions, "copy")
		if !copyAction.disabled {
			t.Error("copy action should be disabled for ghost skill")
		}
		if copyAction.reason != "no local path" {
			t.Errorf("copy reason = %q, want %q", copyAction.reason, "no local path")
		}

		editAction := findAction(actions, "edit")
		if !editAction.disabled {
			t.Error("edit action should be disabled for ghost skill")
		}
		if editAction.reason != "no local path" {
			t.Errorf("edit reason = %q, want %q", editAction.reason, "no local path")
		}
	})

	t.Run("update is always disabled", func(t *testing.T) {
		sk := discovery.Skill{Name: "any", LocalPath: "/some/path"}
		actions := actionsForSkill(sk)

		updateAction := findAction(actions, "update")
		if !updateAction.disabled {
			t.Error("update action should always be disabled")
		}
	})

	t.Run("category is always disabled", func(t *testing.T) {
		sk := discovery.Skill{Name: "any", LocalPath: "/some/path"}
		actions := actionsForSkill(sk)

		catAction := findAction(actions, "category")
		if !catAction.disabled {
			t.Error("category action should always be disabled")
		}
	})
}

func findAction(actions []actionItem, key string) actionItem {
	for _, a := range actions {
		if a.key == key {
			return a
		}
	}
	return actionItem{}
}
