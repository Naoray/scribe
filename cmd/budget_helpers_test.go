package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/state"
)

func TestBudgetSkillsForAgentExcludesPinnedSkillWithoutAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd := t.TempDir()
	t.Chdir(wd)

	writeBudgetSkill(t, home, "claude-only", "claude")
	writeBudgetSkill(t, home, "shared", "shared")

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"claude-only": {
			ToolsMode: state.ToolsModePinned,
			Tools:     []string{"claude"},
		},
		"shared": {
			Tools: []string{"claude", "codex"},
		},
	}}

	set, err := resolveBudgetSet(st)
	if err != nil {
		t.Fatalf("resolveBudgetSet: %v", err)
	}

	codex := budgetSkillsForAgent(set, st, "codex")
	claude := budgetSkillsForAgent(set, st, "claude")

	if skillNames(codex).has("claude-only") {
		t.Fatal("codex budget should exclude claude-only")
	}
	if !skillNames(claude).has("claude-only") {
		t.Fatal("claude budget should include claude-only")
	}
}

func writeBudgetSkill(t *testing.T, home, name, content string) {
	t.Helper()

	dir := filepath.Join(home, ".scribe", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

type testBudgetSkillNames map[string]bool

func skillNames(skills []budget.Skill) testBudgetSkillNames {
	names := make(testBudgetSkillNames, len(skills))
	for _, skill := range skills {
		names[skill.Name] = true
	}
	return names
}

func (names testBudgetSkillNames) has(name string) bool {
	return names[name]
}
