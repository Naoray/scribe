package sync

import (
	"os"
	"path/filepath"
	"testing"

	ibudget "github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func TestBudgetSkillsForProjectionExcludesPinnedSkillWithoutAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	writeProjectionBudgetSkill(t, storeDir, "claude-only", "claude")
	writeProjectionBudgetSkill(t, storeDir, "codex-skill", "codex")

	projectRoot := t.TempDir()
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"claude-only": {
			ToolsMode: state.ToolsModePinned,
			Tools:     []string{"claude"},
			Projections: []state.ProjectionEntry{{
				Project: projectRoot,
				Tools:   []string{"codex"},
			}},
		},
		"codex-skill": {
			ToolsMode: state.ToolsModePinned,
			Tools:     []string{"codex"},
			Projections: []state.ProjectionEntry{{
				Project: projectRoot,
				Tools:   []string{"codex"},
			}},
		},
	}}

	skills, err := budgetSkillsForProjection(st, "incoming", []byte("incoming"), projectRoot, "codex")
	if err != nil {
		t.Fatalf("budgetSkillsForProjection: %v", err)
	}

	names := projectionBudgetSkillNames(skills)
	if names.has("claude-only") {
		t.Fatal("codex budget should exclude claude-only")
	}
	if !names.has("codex-skill") {
		t.Fatal("codex budget should include codex-skill")
	}
	if !names.has("incoming") {
		t.Fatal("codex budget should include incoming")
	}
}

func writeProjectionBudgetSkill(t *testing.T, storeDir, name, content string) {
	t.Helper()

	dir := filepath.Join(storeDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

type projectionBudgetSkillNameSet map[string]bool

func projectionBudgetSkillNames(skills []ibudget.Skill) projectionBudgetSkillNameSet {
	names := make(projectionBudgetSkillNameSet, len(skills))
	for _, skill := range skills {
		names[skill.Name] = true
	}
	return names
}

func (names projectionBudgetSkillNameSet) has(name string) bool {
	return names[name]
}
