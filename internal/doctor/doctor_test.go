package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/snippet"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func TestInspectSkillReportsMissingDescription(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	writeSkill(t, "recap", []byte(`---
name: recap
---

# Recap

Keep daily notes and summaries.
`))

	st := managedState("recap", []string{"claude"}, nil)

	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Skill != "recap" {
		t.Fatalf("Skill = %q, want recap", issue.Skill)
	}
	if issue.Kind != IssueCanonicalMetadata {
		t.Fatalf("Kind = %q, want %q", issue.Kind, IssueCanonicalMetadata)
	}
	if issue.Status != "warn" {
		t.Fatalf("Status = %q, want warn", issue.Status)
	}
	if !strings.Contains(issue.Message, "description") {
		t.Fatalf("Message = %q, want description-related issue", issue.Message)
	}
}

func TestInspectManagedSkillsReportsMissingSnippetProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".scribe.yaml"), []byte("snippets:\n  - commit-discipline\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetContent := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(filepath.Join(snippetDir, "commit-discipline.md"), []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	t.Chdir(project)

	cfg := &config.Config{Tools: []config.ToolConfig{{Name: "claude", Enabled: true}}}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Kind != IssueSnippetProjectionDrift {
		t.Fatalf("Kind = %q, want %q", issue.Kind, IssueSnippetProjectionDrift)
	}
	if issue.Skill != "snippet:commit-discipline" || issue.Tool != "claude" {
		t.Fatalf("Issue = %+v, want commit-discipline claude drift", issue)
	}
	if !strings.Contains(issue.Message, "scribe sync") {
		t.Fatalf("Message = %q, want sync remediation", issue.Message)
	}
}

func TestInspectManagedSkillsHonorsExistingOnlyAllSnippetTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".scribe.yaml"), []byte("snippets:\n  - all-rules\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetContent := "---\nname: all-rules\ndescription: All rules\ntargets: all\n---\n# Rules\n"
	if err := os.WriteFile(filepath.Join(snippetDir, "all-rules.md"), []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	snippets, err := snippet.LoadProject(snippetDir, []string{"all-rules"})
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if _, err := snippet.Project(project, snippets, []string{"claude", "codex", "cursor"}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	t.Chdir(project)

	cfg := &config.Config{Tools: []config.ToolConfig{
		{Name: "claude", Enabled: true},
		{Name: "codex", Enabled: true},
		{Name: "cursor", Enabled: true},
	}}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	for _, issue := range report.Issues {
		if issue.Kind == IssueSnippetProjectionDrift {
			t.Fatalf("unexpected snippet drift issue: %+v", issue)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md should not be created by existing-only all target projection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".cursorrules")); !os.IsNotExist(err) {
		t.Fatalf(".cursorrules should not be created by existing-only all target projection: %v", err)
	}
}

func TestInspectSkillReportsInvalidFrontmatter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	writeSkill(t, "recap", []byte(`---
name: recap

# missing closing delimiter
`))

	st := managedState("recap", []string{"claude"}, nil)

	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Skill != "recap" {
		t.Fatalf("Skill = %q, want recap", issue.Skill)
	}
	if issue.Kind != IssueCanonicalMetadata {
		t.Fatalf("Kind = %q, want %q", issue.Kind, IssueCanonicalMetadata)
	}
	if issue.Status != "error" {
		t.Fatalf("Status = %q, want error", issue.Status)
	}
	if !strings.Contains(issue.Message, "frontmatter") {
		t.Fatalf("Message = %q, want frontmatter-related issue", issue.Message)
	}
}

func TestInspectSkillReportsBrokenProjectionState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	canonical := writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	codexPath := filepath.Join(home, ".agents", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(canonical, codexPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "claude", Enabled: true},
			{Name: "codex", Enabled: true},
		},
	}
	st := managedState("recap", []string{"claude"}, []string{codexPath})

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Skill != "recap" {
		t.Fatalf("Skill = %q, want recap", issue.Skill)
	}
	if issue.Kind != IssueProjectionDrift {
		t.Fatalf("Kind = %q, want %q", issue.Kind, IssueProjectionDrift)
	}
	if issue.Tool != "codex" {
		t.Fatalf("Tool = %q, want codex", issue.Tool)
	}
	if issue.Status != "warn" {
		t.Fatalf("Status = %q, want warn", issue.Status)
	}
}

func TestInspectCanLimitToOneSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	writeSkill(t, "recap", []byte(`---
name: recap
---

Keep daily notes and summaries.
`))
	writeSkill(t, "notes", []byte(`---
name: notes
---

Capture anything useful.
`))

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "a",
				Tools:         []string{"claude"},
				ToolsMode:     state.ToolsModePinned,
			},
			"notes": {
				Revision:      1,
				InstalledHash: "b",
				Tools:         []string{"claude"},
				ToolsMode:     state.ToolsModePinned,
			},
		},
	}

	report, err := InspectManagedSkills(nil, st, "recap")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	if report.Issues[0].Skill != "recap" {
		t.Fatalf("Skill = %q, want recap", report.Issues[0].Skill)
	}
}

func TestInspectSkipsPackageSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	writeSkill(t, "bundle", []byte(`---
name: bundle

# broken package metadata that should be ignored
`))

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"bundle": {
				Revision:      1,
				InstalledHash: "pkg",
				Tools:         []string{"claude"},
				ToolsMode:     state.ToolsModePinned,
				Type:          "package",
			},
		},
	}

	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("Issues = %d, want 0: %+v", len(report.Issues), report.Issues)
	}
}

func TestInspectSkipsOpaqueToolProjections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	ghostPath := filepath.Join(home, ".ghost", "skills", "recap")
	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "hash",
				Tools:         []string{"ghost"},
				ToolsMode:     state.ToolsModePinned,
				ManagedPaths:  []string{ghostPath},
				Paths:         []string{ghostPath},
			},
		},
	}
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{
				Name:      "ghost",
				Enabled:   true,
				Install:   "echo install",
				Uninstall: "echo uninstall",
				Path:      ghostPath,
			},
		},
	}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("Issues = %d, want 0: %+v", len(report.Issues), report.Issues)
	}
}

// TestInspectSkipsOpaqueToolWhenSkillPathErrors covers the gemini-style
// failure mode: an opaque tool that intentionally returns an error from
// SkillPath (because it cannot expose a filesystem location). The opacity
// check must run BEFORE SkillPath so the intentional error is not wrapped
// as a bogus projection_drift issue. See todo #488.
func TestInspectSkipsOpaqueToolWhenSkillPathErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	// Custom tool with no Path template — SkillPath will return an error,
	// mirroring the gemini case. CommandTool.CanonicalTarget always returns
	// ok=false, so this tool is opaque from doctor's perspective.
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{
				Name:      "ghost",
				Enabled:   true,
				Install:   "echo install",
				Uninstall: "echo uninstall",
			},
		},
	}
	st := managedState("recap", []string{"ghost"}, []string{"ghost:user:recap"})

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Kind == IssueProjectionDrift {
			t.Fatalf("unexpected projection_drift issue for opaque tool: %+v", issue)
		}
	}
}

func TestInspectManagedSkillsReportsRegistryKitIssues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	kitsDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatalf("mkdir kits: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kitsDir, "baseline.yaml"), []byte("name: baseline\nskills:\n  - other/skills:debugging\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{},
		Kits: map[string]state.InstalledKit{
			"baseline": {Name: "baseline", SourceRegistry: "acme/skills"},
			"missing":  {Name: "missing", SourceRegistry: "acme/skills"},
		},
	}
	cfg := &config.Config{Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}}}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	if !hasIssue(report, "kit:baseline", IssueKitRefBroken) {
		t.Fatalf("missing cross-registry ref issue: %+v", report.Issues)
	}
	if !hasIssue(report, "kit:missing", IssueKitOrphaned) {
		t.Fatalf("missing orphaned kit issue: %+v", report.Issues)
	}
}

func TestInspectManagedSkillsReportsForgottenKitRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	kitsDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatalf("mkdir kits: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kitsDir, "baseline.yaml"), []byte("name: baseline\nskills:\n  - tdd\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{},
		Kits:      map[string]state.InstalledKit{"baseline": {Name: "baseline", SourceRegistry: "acme/skills"}},
	}

	report, err := InspectManagedSkills(&config.Config{}, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	if !hasIssue(report, "kit:baseline", IssueKitRegistryForgotten) {
		t.Fatalf("missing forgotten registry issue: %+v", report.Issues)
	}
}

func hasIssue(report Report, skill string, kind IssueKind) bool {
	for _, issue := range report.Issues {
		if issue.Skill == skill && issue.Kind == kind {
			return true
		}
	}
	return false
}

// TestInspectStillReportsDriftForInspectableTool guards against regressing
// drift detection while fixing the opacity bug. A real inspectable tool with
// a missing projection on disk must still surface as projection_drift.
func TestInspectStillReportsDriftForInspectableTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	// claude is an inspectable builtin. Skill state claims a projection at
	// the expected claude path, but no symlink is on disk → drift.
	claudePath := filepath.Join(home, ".claude", "skills", "recap")
	st := managedState("recap", []string{"claude"}, []string{claudePath})
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "claude", Enabled: true},
		},
	}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	var drift *Issue
	for i := range report.Issues {
		if report.Issues[i].Kind == IssueProjectionDrift {
			drift = &report.Issues[i]
			break
		}
	}
	if drift == nil {
		t.Fatalf("expected a projection_drift issue, got: %+v", report.Issues)
	}
	if drift.Tool != "claude" {
		t.Fatalf("Tool = %q, want claude", drift.Tool)
	}
	if !strings.Contains(drift.Message, "missing managed projection") &&
		!strings.Contains(drift.Message, "does not point to the canonical target") {
		t.Fatalf("Message = %q, want drift detail", drift.Message)
	}
}

// TestInspectSkipsConflictsFromOpaqueTools verifies the conflicts loop
// suppresses entries whose tool is opaque. Without the opaqueTools-by-name
// guard, conflicts recorded against an opaque tool (which has no
// scribe-known path) would still surface as drift.
func TestInspectSkipsConflictsFromOpaqueTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	canonical := writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	codexPath := filepath.Join(home, ".agents", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(canonical, codexPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "hash",
				Tools:         []string{"ghost", "codex"},
				ToolsMode:     state.ToolsModePinned,
				ManagedPaths:  []string{codexPath, "ghost:user:recap"},
				Paths:         []string{codexPath, "ghost:user:recap"},
				Conflicts: []state.ProjectionConflict{
					{Tool: "ghost", Path: "ghost:user:recap"},
					{Tool: "codex", Path: codexPath},
				},
			},
		},
	}
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "codex", Enabled: true},
			{
				Name:      "ghost",
				Enabled:   true,
				Install:   "echo install",
				Uninstall: "echo uninstall",
			},
		},
	}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}

	var drift *Issue
	for i := range report.Issues {
		if report.Issues[i].Kind == IssueProjectionDrift {
			drift = &report.Issues[i]
			break
		}
	}
	if drift == nil {
		t.Fatalf("expected drift issue from codex conflict, got: %+v", report.Issues)
	}
	if strings.Contains(drift.Message, "ghost") {
		t.Fatalf("opaque ghost conflict leaked into details: %q", drift.Message)
	}
	if !strings.Contains(drift.Message, "codex") {
		t.Fatalf("expected codex conflict detail, got: %q", drift.Message)
	}
}

func TestInspectContinuesWithInvalidUnrelatedToolConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonical := writeSkill(t, "recap", []byte(`---
name: recap
---

# Recap

Keep daily notes and summaries.
`))

	claudePath := filepath.Join(home, ".claude", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(canonical, claudePath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	st := managedState("recap", []string{"claude"}, []string{claudePath})
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "claude", Enabled: true},
			{Name: "broken", Enabled: true, Install: "echo install"},
		},
	}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	if report.Issues[0].Kind != IssueCanonicalMetadata {
		t.Fatalf("Kind = %q, want %q", report.Issues[0].Kind, IssueCanonicalMetadata)
	}
}

func TestInspectIgnoresExplicitlyDisabledBuiltinTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	writeSkill(t, "recap", []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`))

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "hash",
				ToolsMode:     state.ToolsModeInherit,
			},
		},
	}
	cfg := &config.Config{
		Tools: []config.ToolConfig{
			{Name: "claude", Enabled: false},
		},
	}

	report, err := InspectManagedSkills(cfg, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("Issues = %d, want 0: %+v", len(report.Issues), report.Issues)
	}
}

func TestDoctor_WarnsMigrationBudgetOverflow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"claude": 20}
	t.Cleanup(func() { budget.AgentBudgets = old })
	writeSkill(t, "recap", []byte("---\nname: recap\ndescription: "+strings.Repeat("x", 200)+"\n---\n"))
	project := filepath.Join(home, "project")
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision: 1,
				Projections: []state.ProjectionEntry{{
					Project: project,
					Tools:   []string{"claude"},
					Source:  state.SourceMigration,
				}},
			},
		},
	}
	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Kind == IssueMigrationBudgetOverflow && issue.Tool == "claude" && issue.Status == "warn" {
			return
		}
	}
	t.Fatalf("Issues = %+v, want migration budget overflow", report.Issues)
}

func TestDoctor_WarnsGlobalListingBudgetOverflow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"claude": 50}
	t.Cleanup(func() { budget.AgentBudgets = old })
	writeSkill(t, "small", []byte("---\nname: small\ndescription: "+strings.Repeat("s", 20)+"\n---\n"))
	writeSkill(t, "large", []byte("---\nname: large\ndescription: "+strings.Repeat("l", 80)+"\n---\n"))
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"small": {
				Revision:  1,
				Tools:     []string{"claude"},
				ToolsMode: state.ToolsModePinned,
			},
			"large": {
				Revision:  1,
				Tools:     []string{"claude"},
				ToolsMode: state.ToolsModePinned,
			},
		},
	}
	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	var matches []Issue
	for _, issue := range report.Issues {
		if issue.Kind == IssueGlobalListingBudgetOverflow {
			matches = append(matches, issue)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("global listing budget issues = %d, want 1: %+v", len(matches), report.Issues)
	}
	issue := matches[0]
	if issue.Tool != "claude" {
		t.Fatalf("Tool = %q, want claude", issue.Tool)
	}
	if len(issue.LargestSkills) != 2 {
		t.Fatalf("LargestSkills = %+v, want 2 entries", issue.LargestSkills)
	}
	if issue.LargestSkills[0].Skill != "large" {
		t.Fatalf("largest skill = %q, want large: %+v", issue.LargestSkills[0].Skill, issue.LargestSkills)
	}
}

func TestDoctor_NoNewWarningsAfterMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	old := budget.AgentBudgets
	budget.AgentBudgets = map[string]int{"claude": 8000}
	t.Cleanup(func() { budget.AgentBudgets = old })
	writeSkill(t, "recap", []byte("---\nname: recap\ndescription: small\n---\n"))
	project := filepath.Join(home, "project")
	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision: 1,
				Projections: []state.ProjectionEntry{{
					Project: project,
					Tools:   []string{"claude"},
					Source:  state.SourceMigration,
				}},
			},
		},
	}
	report, err := InspectManagedSkills(nil, st, "")
	if err != nil {
		t.Fatalf("InspectManagedSkills: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Kind == IssueMigrationBudgetOverflow {
			t.Fatalf("unexpected migration budget issue: %+v", issue)
		}
	}
}

func writeSkill(t *testing.T, name string, content []byte) string {
	t.Helper()

	dir, err := tools.WriteToStore(name, []tools.SkillFile{{Path: "SKILL.md", Content: content}})
	if err != nil {
		t.Fatalf("WriteToStore(%s): %v", name, err)
	}
	return dir
}

func managedState(name string, toolsList []string, managedPaths []string) *state.State {
	return &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			name: {
				Revision:      1,
				InstalledHash: "hash",
				Tools:         append([]string(nil), toolsList...),
				ToolsMode:     state.ToolsModePinned,
				ManagedPaths:  append([]string(nil), managedPaths...),
				Paths:         append([]string(nil), managedPaths...),
			},
		},
	}
}
