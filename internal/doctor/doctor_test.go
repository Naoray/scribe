package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
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

	codexPath := filepath.Join(home, ".codex", "skills", "recap")
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
