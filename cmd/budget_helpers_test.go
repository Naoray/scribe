package cmd

import (
	"os"
	"path/filepath"
	"strings"
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

func TestProjectLocalBudgetUsesShortCodexDescriptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd := t.TempDir()
	t.Chdir(wd)

	writeBudgetSkill(t, home, "review-triage", longBudgetSkillContent("review-triage", 3000))
	writeBudgetSkill(t, home, "solo-orchestration", longBudgetSkillContent("solo-orchestration", 3000))
	writeBudgetSkill(t, home, "not-in-project", longBudgetSkillContent("not-in-project", 3000))
	writeBudgetKit(t, home, "orchestrator", []string{"review-triage", "solo-orchestration"})
	if err := os.WriteFile(filepath.Join(wd, ".scribe.yaml"), []byte("kits:\n - orchestrator\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"review-triage": {
			Tools: []string{"codex"},
		},
		"solo-orchestration": {
			Tools: []string{"codex"},
		},
		"not-in-project": {
			Tools: []string{"codex"},
		},
	}}

	set, err := resolveBudgetSet(st)
	if err != nil {
		t.Fatalf("resolveBudgetSet: %v", err)
	}
	if set.ProjectRoot != wd {
		t.Fatalf("ProjectRoot = %q, want %q", set.ProjectRoot, wd)
	}
	names := skillNames(set.Skills)
	if !names.has("review-triage") || !names.has("solo-orchestration") {
		t.Fatalf("skills = %#v, want project kit skills", names)
	}
	if names.has("not-in-project") {
		t.Fatal("project-local budget should not include skills outside the project kit")
	}

	raw := budget.CheckBudget(budgetSkillsForAgent(set, st, "codex"), "codex")
	if raw.Status != budget.StatusRefuse {
		t.Fatalf("raw Status = %s, want %s", raw.Status, budget.StatusRefuse)
	}
	projected := budget.CheckProjectionBudget(budgetSkillsForAgent(set, st, "codex"), "codex")
	if projected.Status == budget.StatusRefuse {
		t.Fatalf("projected Status = %s, want non-refuse; used %d", projected.Status, projected.Used)
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

func writeBudgetKit(t *testing.T, home, name string, skills []string) {
	t.Helper()

	dir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir kit dir: %v", err)
	}
	var content strings.Builder
	content.WriteString("name: " + name + "\n")
	content.WriteString("skills:\n")
	for _, skill := range skills {
		content.WriteString(" - " + skill + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
}

func longBudgetSkillContent(name string, descriptionBytes int) string {
	return "---\n" +
		"name: " + name + "\n" +
		"description: " + strings.Repeat("x", descriptionBytes) + "\n" +
		"---\n"
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
