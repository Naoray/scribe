package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/doctor"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func TestDoctorCommandReportsIssues(t *testing.T) {
	setupDoctorIssueFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}

	got := out.String()
	if !strings.Contains(got, "scribe doctor — 1 issues across 1 skills") {
		t.Fatalf("expected summary header in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Canonical metadata (1)") {
		t.Fatalf("expected grouped canonical metadata in output, got:\n%s", got)
	}
	if !strings.Contains(got, "recap") {
		t.Fatalf("expected recap in output, got:\n%s", got)
	}
	if !strings.Contains(got, "missing a description") {
		t.Fatalf("expected canonical metadata message in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Run `scribe doctor --fix`") {
		t.Fatalf("expected fix CTA in output, got:\n%s", got)
	}
}

func TestDoctorTextRendersGlobalListingBudgetIssue(t *testing.T) {
	report := doctor.Report{Issues: []doctor.Issue{{
		Tool:          "claude",
		Kind:          doctor.IssueGlobalListingBudgetOverflow,
		Status:        "warn",
		BudgetUsed:    78,
		BudgetLimit:   80,
		BudgetPercent: 98,
		LargestSkills: []budget.Overflow{
			{Skill: "claude-api", Bytes: 45},
			{Skill: "obsidian-vault", Bytes: 30},
		},
	}}}
	var out bytes.Buffer
	if err := writeDoctorText(&out, "", report); err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Claude Code skill-listing budget at 78/80 bytes (98%)",
		"Largest contributors:",
		"- claude-api (45 bytes)",
		"skillListingBudgetFraction",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorJSONIncludesGlobalListingBudgetDetails(t *testing.T) {
	report := doctor.Report{Issues: []doctor.Issue{{
		Tool:          "claude",
		Kind:          doctor.IssueGlobalListingBudgetOverflow,
		Status:        "warn",
		BudgetUsed:    78,
		BudgetLimit:   80,
		BudgetPercent: 98,
		LargestSkills: []budget.Overflow{
			{Skill: "claude-api", Bytes: 45},
		},
	}}}
	out := buildDoctorReportJSON("", report)
	if len(out.Issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(out.Issues))
	}
	issue := out.Issues[0]
	if issue.BudgetUsed != 78 || issue.BudgetLimit != 80 || issue.BudgetPercent != 98 {
		t.Fatalf("budget fields = %+v, want used/limit/percent", issue)
	}
	if len(issue.LargestSkills) != 1 || issue.LargestSkills[0].Skill != "claude-api" || issue.LargestSkills[0].Bytes != 45 {
		t.Fatalf("largest skills = %+v, want claude-api byte details", issue.LargestSkills)
	}
}

func TestDoctorRejectsUnknownSkill(t *testing.T) {
	setupDoctorCleanFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--skill", "missing"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not-installed error, got %q", err.Error())
	}
}

func TestDoctorFixNoopsWhenClean(t *testing.T) {
	setupDoctorCleanFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--fix"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "No managed skill issues found.") {
		t.Fatalf("expected no-op output, got:\n%s", out.String())
	}
}

func TestDoctorFixRepairsAffectedProjections(t *testing.T) {
	setupDoctorFixFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}

	home := os.Getenv("HOME")
	canonicalDir := filepath.Join(home, ".scribe", "skills", "recap")
	canonicalResolved, err := filepath.EvalSymlinks(canonicalDir)
	if err != nil {
		t.Fatalf("EvalSymlinks canonical: %v", err)
	}

	codexPath := filepath.Join(home, ".agents", "skills", "recap")
	resolved, err := filepath.EvalSymlinks(codexPath)
	if err != nil {
		t.Fatalf("EvalSymlinks codex: %v", err)
	}
	if resolved != canonicalResolved {
		t.Fatalf("codex projection resolves to %q, want %q", resolved, canonicalResolved)
	}
}

func TestDoctorFixUpdatesInstalledHash(t *testing.T) {
	setupDoctorFixFixture(t)

	root := newRootCmd()
	root.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home := os.Getenv("HOME")
	content, err := os.ReadFile(filepath.Join(home, ".scribe", "skills", "recap", "SKILL.md"))
	if err != nil {
		t.Fatalf("read canonical SKILL.md: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	got := st.Installed["recap"].InstalledHash
	want := sync.ComputeFileHash(content)
	if got != want {
		t.Fatalf("InstalledHash = %q, want %q", got, want)
	}
}

func TestDoctorFixClearsProjectionConflicts(t *testing.T) {
	setupDoctorFixFixture(t)

	root := newRootCmd()
	root.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Installed["recap"].Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, want cleared", st.Installed["recap"].Conflicts)
	}
}

func TestDoctorFixIsIdempotent(t *testing.T) {
	setupDoctorFixFixture(t)

	first := newRootCmd()
	first.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first execute: %v", err)
	}

	home := os.Getenv("HOME")
	canonicalPath := filepath.Join(home, ".scribe", "skills", "recap", "SKILL.md")
	contentBefore, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical SKILL.md: %v", err)
	}

	stBefore, err := state.Load()
	if err != nil {
		t.Fatalf("load state before second run: %v", err)
	}

	second := newRootCmd()
	second.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})
	if err := second.Execute(); err != nil {
		t.Fatalf("second execute: %v", err)
	}

	contentAfter, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical SKILL.md after second run: %v", err)
	}
	if !bytes.Equal(contentAfter, contentBefore) {
		t.Fatalf("canonical SKILL.md changed on second run:\nbefore:\n%s\nafter:\n%s", contentBefore, contentAfter)
	}

	stAfter, err := state.Load()
	if err != nil {
		t.Fatalf("load state after second run: %v", err)
	}

	beforeSkill := stBefore.Installed["recap"]
	afterSkill := stAfter.Installed["recap"]
	if afterSkill.InstalledHash != beforeSkill.InstalledHash {
		t.Fatalf("InstalledHash changed on second run: %q -> %q", beforeSkill.InstalledHash, afterSkill.InstalledHash)
	}
	if strings.Join(afterSkill.ManagedPaths, ",") != strings.Join(beforeSkill.ManagedPaths, ",") {
		t.Fatalf("ManagedPaths changed on second run: %v -> %v", beforeSkill.ManagedPaths, afterSkill.ManagedPaths)
	}
	if len(afterSkill.Conflicts) != 0 {
		t.Fatalf("Conflicts after second run = %v, want empty", afterSkill.Conflicts)
	}
}

func TestDoctorFixCleansZeroEffectiveToolDrift(t *testing.T) {
	setupDoctorZeroEffectiveToolsFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--fix", "--skill", "recap"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}

	home := os.Getenv("HOME")
	stalePath := filepath.Join(home, ".agents", "skills", "recap")
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale projection still exists at %s", stalePath)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	got := st.Installed["recap"]
	if len(got.ManagedPaths) != 0 {
		t.Fatalf("ManagedPaths = %v, want empty", got.ManagedPaths)
	}
	if len(got.Paths) != 0 {
		t.Fatalf("Paths = %v, want empty", got.Paths)
	}
	if len(got.Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, want empty", got.Conflicts)
	}
	if !strings.Contains(out.String(), "repaired projections") {
		t.Fatalf("expected repair output, got:\n%s", out.String())
	}
}

func TestDoctorFixRollsBackEarlierSkillsWhenLaterSnapshotFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	cfg := &config.Config{BuiltinsVersion: 3}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	original := []byte(`---
name: recap
---

# Recap

Keep daily notes and summaries.
`)
	if _, err := tools.WriteToStore("recap", []tools.SkillFile{{Path: "SKILL.md", Content: original}}); err != nil {
		t.Fatalf("write recap store: %v", err)
	}

	brokenDir, err := tools.WriteToStore("zzbroken", []tools.SkillFile{{Path: "SKILL.md", Content: []byte(`# Broken
`)}})
	if err != nil {
		t.Fatalf("write broken store: %v", err)
	}
	if err := os.Remove(filepath.Join(brokenDir, "SKILL.md")); err != nil {
		t.Fatalf("remove broken SKILL.md: %v", err)
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "oldhash",
				ToolsMode:     state.ToolsModePinned,
			},
			"zzbroken": {
				Revision:      1,
				InstalledHash: "brokenhash",
				ToolsMode:     state.ToolsModePinned,
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	root := newRootCmd()
	root.SetArgs([]string{"doctor", "--fix"})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error when later skill snapshot fails")
	}

	recapPath := filepath.Join(home, ".scribe", "skills", "recap", "SKILL.md")
	gotContent, err := os.ReadFile(recapPath)
	if err != nil {
		t.Fatalf("read recap SKILL.md: %v", err)
	}
	if !bytes.Equal(gotContent, original) {
		t.Fatalf("recap SKILL.md was not rolled back:\nwant:\n%s\ngot:\n%s", original, gotContent)
	}

	st, err = state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got := st.Installed["recap"].InstalledHash; got != "oldhash" {
		t.Fatalf("InstalledHash after rollback = %q, want oldhash", got)
	}
}

func TestDoctorNoIssueOutput(t *testing.T) {
	setupDoctorCleanFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}

	got := out.String()
	if !strings.Contains(got, "No managed skill issues found.") {
		t.Fatalf("expected no-issue output, got:\n%s", got)
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	setupDoctorIssueFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v (stderr=%s)", err, errBuf.String())
	}

	var env struct {
		Data struct {
			Issues []struct {
				Skill   string `json:"skill"`
				Tool    string `json:"tool"`
				Kind    string `json:"kind"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out.String())
	}
	if len(env.Data.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %+v", len(env.Data.Issues), env.Data.Issues)
	}
}

func TestDoctorTextIncludesGroupedTool(t *testing.T) {
	var buf bytes.Buffer
	err := writeDoctorText(&buf, "", doctor.Report{
		Issues: []doctor.Issue{{
			Skill:   "recap",
			Tool:    "codex",
			Kind:    doctor.IssueProjectionDrift,
			Status:  "warn",
			Message: "missing managed projection",
		}},
	})
	if err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Projection drift (1)") {
		t.Fatalf("expected grouped projection drift output, got:\n%s", got)
	}
	if !strings.Contains(got, "codex") {
		t.Fatalf("expected tool in text output, got:\n%s", got)
	}
	if strings.Contains(got, "tool=codex") {
		t.Fatalf("expected compact tool column instead of old tool label, got:\n%s", got)
	}
	if !strings.Contains(got, "[warn]") {
		t.Fatalf("expected status in text output, got:\n%s", got)
	}
}

func TestDoctorTextShowsErrorStatus(t *testing.T) {
	var buf bytes.Buffer
	err := writeDoctorText(&buf, "", doctor.Report{
		Issues: []doctor.Issue{{
			Skill:   "recap",
			Kind:    doctor.IssueCanonicalMetadata,
			Status:  "error",
			Message: "read canonical SKILL.md: denied",
		}},
	})
	if err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[error]") {
		t.Fatalf("expected error status in text output, got:\n%s", got)
	}
}

func TestDoctorTextFoldsMigrationBudgetRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	report := doctor.Report{Issues: []doctor.Issue{
		{
			Skill:   filepath.Join(home, "Workspace", "Artistfy", "Dashboard"),
			Tool:    "claude",
			Kind:    doctor.IssueMigrationBudgetOverflow,
			Status:  "warn",
			Message: "migration-derived projections exceed claude budget by 27443 bytes",
		},
		{
			Skill:   filepath.Join(home, "Workspace", "Artistfy", "Dashboard"),
			Tool:    "codex",
			Kind:    doctor.IssueMigrationBudgetOverflow,
			Status:  "warn",
			Message: "migration-derived projections exceed codex budget by 22732 bytes",
		},
		{
			Skill:   filepath.Join(home, "Workspace", "Mine", "site"),
			Tool:    "claude",
			Kind:    doctor.IssueMigrationBudgetOverflow,
			Status:  "warn",
			Message: "migration-derived projections exceed claude budget by 1048576 bytes",
		},
	}}

	var buf bytes.Buffer
	if err := writeDoctorText(&buf, "", report); err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "Migration budget overflow (3)") {
		t.Fatalf("expected migration group, got:\n%s", got)
	}
	if !strings.Contains(got, "~/Workspace/Artistfy/Dashboard") {
		t.Fatalf("expected tilde path, got:\n%s", got)
	}
	if !strings.Contains(got, "claude +26.8 KB · codex +22.2 KB") {
		t.Fatalf("expected folded tool byte summary, got:\n%s", got)
	}
	if !strings.Contains(got, "claude +1.0 MB") {
		t.Fatalf("expected MB formatting, got:\n%s", got)
	}
	if strings.Contains(got, "27443 bytes") {
		t.Fatalf("expected human byte sizes only, got:\n%s", got)
	}
}

func TestDoctorTextSummarizesProjectionDrift(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorPath := filepath.Join(home, ".cursor", "rules", "deploy.mdc")
	claudePath := filepath.Join(home, ".claude", "skills", "deploy")

	var buf bytes.Buffer
	err := writeDoctorText(&buf, "", doctor.Report{
		Issues: []doctor.Issue{{
			Skill:   "deploy",
			Tool:    "cursor",
			Kind:    doctor.IssueProjectionDrift,
			Status:  "warn",
			Message: "unexpected managed projection cursor at " + cursorPath + "; missing managed projection for claude at " + claudePath,
		}},
	})
	if err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "unexpected projection at ~/.cursor/rules/deploy.mdc") {
		t.Fatalf("expected compact unexpected projection detail, got:\n%s", got)
	}
	if !strings.Contains(got, "missing projection at ~/.claude/skills/deploy") {
		t.Fatalf("expected compact missing projection detail, got:\n%s", got)
	}
	if strings.Contains(got, "managed projection") {
		t.Fatalf("expected managed projection noise removed, got:\n%s", got)
	}
}

func TestDoctorTextTruncatesOnlyForTTY(t *testing.T) {
	var issues []doctor.Issue
	for i := 0; i < 12; i++ {
		issues = append(issues, doctor.Issue{
			Skill:   fmt.Sprintf("skill-%02d", i),
			Kind:    doctor.IssueCanonicalMetadata,
			Status:  "warn",
			Message: "SKILL.md is missing a description",
		})
	}
	report := doctor.Report{Issues: issues}

	var ttyBuf bytes.Buffer
	if err := writeDoctorTextWithOptions(&ttyBuf, "", report, true); err != nil {
		t.Fatalf("writeDoctorTextWithOptions tty: %v", err)
	}
	ttyOut := stripANSI(ttyBuf.String())
	if !strings.Contains(ttyOut, "… 2 more  (run with --skill <name> or --json for full list)") {
		t.Fatalf("expected truncation hint, got:\n%s", ttyOut)
	}
	if strings.Contains(ttyOut, "skill-11") {
		t.Fatalf("expected truncated tty output, got:\n%s", ttyOut)
	}

	var pipeBuf bytes.Buffer
	if err := writeDoctorTextWithOptions(&pipeBuf, "", report, false); err != nil {
		t.Fatalf("writeDoctorTextWithOptions pipe: %v", err)
	}
	pipeOut := stripANSI(pipeBuf.String())
	if strings.Contains(pipeOut, "… 2 more") {
		t.Fatalf("expected full piped output, got:\n%s", pipeOut)
	}
	if !strings.Contains(pipeOut, "skill-11") {
		t.Fatalf("expected final row in piped output, got:\n%s", pipeOut)
	}
}

func TestDoctorTextBufferOutputIsPlainAndUntruncated(t *testing.T) {
	var issues []doctor.Issue
	for i := 0; i < 12; i++ {
		issues = append(issues, doctor.Issue{
			Skill:   fmt.Sprintf("skill-%02d", i),
			Kind:    doctor.IssueCanonicalMetadata,
			Status:  "warn",
			Message: "SKILL.md is missing a description",
		})
	}

	var buf bytes.Buffer
	if err := writeDoctorText(&buf, "", doctor.Report{Issues: issues}); err != nil {
		t.Fatalf("writeDoctorText: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected plain buffer output, got:\n%s", got)
	}
	if strings.Contains(got, "… 2 more") {
		t.Fatalf("expected untruncated buffer output, got:\n%s", got)
	}
	if !strings.Contains(got, "skill-11") {
		t.Fatalf("expected final row in buffer output, got:\n%s", got)
	}
}

func TestDoctorFixTextUsesRepairSummary(t *testing.T) {
	var buf bytes.Buffer
	err := writeDoctorFixText(&buf, "", []doctorFixResult{{
		Name:             "deploy",
		UpdatedCanonical: true,
		RepairedTools:    []string{"cursor", "claude"},
	}})
	if err != nil {
		t.Fatalf("writeDoctorFixText: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"Repaired managed skills:",
		"✓ deploy normalized canonical SKILL.md",
		"✓ deploy repaired projections (cursor, claude)",
		"Repaired 1 skills.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func setupDoctorCleanFixture(t *testing.T) {
	t.Helper()
	setupDoctorFixture(t, false)
}

func setupDoctorIssueFixture(t *testing.T) {
	t.Helper()
	setupDoctorFixture(t, true)
}

func setupDoctorFixture(t *testing.T, withIssue bool) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	cfg := &config.Config{BuiltinsVersion: 3}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if _, err := tools.WriteToStore("recap", []tools.SkillFile{{
		Path: "SKILL.md",
		Content: []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`),
	}}); err != nil {
		t.Fatalf("write recap store: %v", err)
	}

	if _, err := tools.WriteToStore("notes", []tools.SkillFile{{
		Path: "SKILL.md",
		Content: []byte(`---
name: notes
description: Capture anything useful.
---

# Notes
`),
	}}); err != nil {
		t.Fatalf("write notes store: %v", err)
	}

	if withIssue {
		recapDir := filepath.Join(home, ".scribe", "skills", "recap")
		if err := os.WriteFile(filepath.Join(recapDir, "SKILL.md"), []byte(`---
name: recap
---

# Recap
`), 0o644); err != nil {
			t.Fatalf("overwrite recap store: %v", err)
		}
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "hash-recap",
				ToolsMode:     state.ToolsModePinned,
			},
			"notes": {
				Revision:      1,
				InstalledHash: "hash-notes",
				ToolsMode:     state.ToolsModePinned,
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

func setupDoctorFixFixture(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	cfg := &config.Config{
		BuiltinsVersion: 3,
		Tools: []config.ToolConfig{
			{Name: "codex", Type: tools.ToolTypeBuiltin, Enabled: true},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	canonicalDir, err := tools.WriteToStore("recap", []tools.SkillFile{{
		Path: "SKILL.md",
		Content: []byte(`---
name: recap
---

# Recap

Keep daily notes and summaries.
`),
	}})
	if err != nil {
		t.Fatalf("write recap store: %v", err)
	}

	codex := tools.CodexTool{}
	codexPath, err := codex.SkillPath("recap", "")
	if err != nil {
		t.Fatalf("codex skill path: %v", err)
	}
	if err := os.MkdirAll(codexPath, 0o755); err != nil {
		t.Fatalf("mkdir codex projection: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexPath, "SKILL.md"), []byte("# recap\nlocal drift\n"), 0o644); err != nil {
		t.Fatalf("write conflicted projection: %v", err)
	}

	normalizedContent, err := os.ReadFile(filepath.Join(canonicalDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read canonical content: %v", err)
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "stalehash",
				Tools:         []string{"codex"},
				ToolsMode:     state.ToolsModePinned,
				Paths:         []string{codexPath},
				ManagedPaths:  []string{codexPath},
				Conflicts: []state.ProjectionConflict{{
					Tool:      "codex",
					Path:      codexPath,
					FoundHash: sync.ComputeFileHash([]byte("# recap\nlocal drift\n")),
					SeenAt:    time.Now().UTC(),
				}},
			},
		},
	}
	if st.Installed["recap"].InstalledHash == sync.ComputeFileHash(normalizedContent) {
		t.Fatal("fixture installed hash unexpectedly matches normalized content")
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

func setupDoctorZeroEffectiveToolsFixture(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	cfg := &config.Config{
		BuiltinsVersion: 3,
		Tools: []config.ToolConfig{
			{Name: "codex", Type: tools.ToolTypeBuiltin, Enabled: false},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if _, err := tools.WriteToStore("recap", []tools.SkillFile{{
		Path: "SKILL.md",
		Content: []byte(`---
name: recap
description: Keep daily notes and summaries.
---

# Recap
`),
	}}); err != nil {
		t.Fatalf("write recap store: %v", err)
	}

	codex := tools.CodexTool{}
	codexPath, err := codex.SkillPath("recap", "")
	if err != nil {
		t.Fatalf("codex skill path: %v", err)
	}
	if err := os.MkdirAll(codexPath, 0o755); err != nil {
		t.Fatalf("mkdir stale codex projection: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexPath, "SKILL.md"), []byte("# recap\nstale drift\n"), 0o644); err != nil {
		t.Fatalf("write stale codex projection: %v", err)
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:      1,
				InstalledHash: "abc12345",
				Tools:         []string{"codex"},
				ToolsMode:     state.ToolsModePinned,
				Paths:         []string{codexPath},
				ManagedPaths:  []string{codexPath},
				Conflicts: []state.ProjectionConflict{{
					Tool:      "codex",
					Path:      codexPath,
					FoundHash: sync.ComputeFileHash([]byte("# recap\nstale drift\n")),
					SeenAt:    time.Now().UTC(),
				}},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
}
