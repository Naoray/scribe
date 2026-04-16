package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/doctor"
	"github.com/Naoray/scribe/internal/state"
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

func TestDoctorRejectsBareFix(t *testing.T) {
	setupDoctorCleanFixture(t)

	root := newRootCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"doctor", "--fix"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for bare --fix")
	}
	if !strings.Contains(err.Error(), "--fix not implemented yet") {
		t.Fatalf("expected not-implemented error, got %q", err.Error())
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
