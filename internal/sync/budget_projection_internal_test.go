package sync

import (
	"os"
	"path/filepath"
	"strings"
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
				Tools:   []string{"claude"},
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

func TestBudgetSkillsForProjectionUsesCurrentProjectToolsOverGlobalPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	writeProjectionBudgetSkill(t, storeDir, "project-codex", "codex")

	projectRoot := t.TempDir()
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"project-codex": {
			ToolsMode: state.ToolsModePinned,
			Tools:     []string{"claude"},
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
	if !names.has("project-codex") {
		t.Fatal("codex budget should include current project codex projection despite global claude pin")
	}
}

func TestCheckBudgetBeforeProjectionUsesShortCodexDescriptionsForProjectScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storeDir, err := tools.StoreDir()
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}
	longStoredSkill(t, storeDir, "same-project", 3000)
	longStoredSkill(t, storeDir, "other-project", 3000)

	projectRoot := t.TempDir()
	otherProjectRoot := t.TempDir()
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"same-project": {
			Tools: []string{"codex"},
			Projections: []state.ProjectionEntry{{
				Project: projectRoot,
				Tools:   []string{"codex"},
			}},
		},
		"other-project": {
			Tools: []string{"codex"},
			Projections: []state.ProjectionEntry{{
				Project: otherProjectRoot,
				Tools:   []string{"codex"},
			}},
		},
	}}
	incoming := longSkillContent("incoming", 3000)

	skills, err := budgetSkillsForProjection(st, "incoming", incoming, projectRoot, "codex")
	if err != nil {
		t.Fatalf("budgetSkillsForProjection: %v", err)
	}
	names := projectionBudgetSkillNames(skills)
	if !names.has("same-project") || !names.has("incoming") {
		t.Fatalf("budget names = %#v, want same-project and incoming", names)
	}
	if names.has("other-project") {
		t.Fatal("project budget should not include another project-local codex projection")
	}

	raw := ibudget.CheckBudget(skills, "codex")
	if raw.Status != ibudget.StatusRefuse {
		t.Fatalf("raw Status = %s, want %s", raw.Status, ibudget.StatusRefuse)
	}

	syncer := &Syncer{ProjectRoot: projectRoot}
	err = syncer.checkBudgetBeforeProjection(st, "incoming", []tools.SkillFile{{Path: "SKILL.md", Content: incoming}}, []tools.Tool{tools.CodexTool{}})
	if err != nil {
		t.Fatalf("checkBudgetBeforeProjection: %v", err)
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

func longStoredSkill(t *testing.T, storeDir, name string, descriptionBytes int) {
	t.Helper()

	writeProjectionBudgetSkill(t, storeDir, name, string(longSkillContent(name, descriptionBytes)))
}

func longSkillContent(name string, descriptionBytes int) []byte {
	return []byte("---\n" +
		"name: " + name + "\n" +
		"description: " + strings.Repeat("x", descriptionBytes) + "\n" +
		"---\n")
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
