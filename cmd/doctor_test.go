package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(got, "recap") {
		t.Fatalf("expected recap in output, got:\n%s", got)
	}
	if !strings.Contains(got, "missing a description") {
		t.Fatalf("expected canonical metadata message in output, got:\n%s", got)
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

	codexPath := filepath.Join(home, ".codex", "skills", "recap")
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
	stalePath := filepath.Join(home, ".codex", "skills", "recap")
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

	var report struct {
		Issues []struct {
			Skill   string `json:"skill"`
			Tool    string `json:"tool"`
			Kind    string `json:"kind"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out.String())
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %+v", len(report.Issues), report.Issues)
	}
}

func TestDoctorTextIncludesTool(t *testing.T) {
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
	if !strings.Contains(got, "tool=codex") {
		t.Fatalf("expected tool in text output, got:\n%s", got)
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
	codexPath, err := codex.SkillPath("recap")
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
	codexPath, err := codex.SkillPath("recap")
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
