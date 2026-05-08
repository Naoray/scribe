package workflow_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/workflow"
)

func TestTextFormatter_SingleRegistry(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, false, &out, &errOut)

	fmtr.OnSyncStart(1)
	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", false, 1)
	fmtr.OnSkillSkipped("formatter", sync.SkillStatus{
		Installed: &state.InstalledSkill{Revision: 1},
	})
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1, Skipped: 1})
	fmtr.Flush()

	if !strings.Contains(errOut.String(), "→ checking 1 connected registry ...") {
		t.Errorf("expected sync-start header, got stderr: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "── acme/skills ──") {
		t.Errorf("expected '── acme/skills ──' section header, got stderr: %s", errOut.String())
	}

	if !strings.Contains(out.String(), "linter") {
		t.Errorf("expected 'linter' in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("expected 'installed' verb, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "(rev 1)") {
		t.Errorf("expected '(rev 1)' suffix, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "done: 1 installed") {
		t.Errorf("expected 'done: 1 installed' in output, got: %s", out.String())
	}
}

func TestTextFormatter_MultiRegistry(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, true, &out, &errOut)

	fmtr.OnSyncStart(2)
	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", false, 1)
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1})

	fmtr.OnRegistryStart("acme/tools")
	fmtr.OnSkillInstalled("debugger", true, 3)
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Updated: 1})

	fmtr.Flush()

	if !strings.Contains(errOut.String(), "→ checking 2 connected registries ...") {
		t.Errorf("expected sync-start header, got stderr: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "── acme/skills ──") {
		t.Errorf("expected '── acme/skills ──' header, got stderr: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "── acme/tools ──") {
		t.Errorf("expected '── acme/tools ──' header, got stderr: %s", errOut.String())
	}

	if !strings.Contains(out.String(), "(rev 1)") || !strings.Contains(out.String(), "(rev 3)") {
		t.Errorf("expected per-row revisions in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "1 installed, 1 updated") {
		t.Errorf("expected aggregated summary, got: %s", out.String())
	}
}

func TestTextFormatter_BudgetWarning(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, false, &out, &errOut)

	fmtr.OnBudgetWarning("codex", "Codex budget: 78% (4280 / 5440 bytes)")

	if !strings.Contains(errOut.String(), "! Codex budget: 78% (4280 / 5440 bytes)") {
		t.Errorf("expected '!' prefixed budget warning, got: %s", errOut.String())
	}
}

func TestTextFormatter_AdoptionWithDeferredConflicts(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, false, &out, &errOut)

	fmtr.OnAdoptionConflictsDeferred([]string{"commit"})
	fmtr.OnAdoptionStarted(3)
	fmtr.OnAdopted("tdd", []string{"claude"})
	fmtr.OnAdopted("code-review", []string{"claude"})
	fmtr.OnAdopted("deploy", []string{"codex"})
	fmtr.OnAdoptionComplete(3, 1, 0)

	if !strings.Contains(out.String(), "✓ adopted 3 skills via symlink (originals untouched)") {
		t.Errorf("expected adopted summary line, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "! 1 skipped (commit · use --force to override)") {
		t.Errorf("expected '! 1 skipped' line, got: %s", errOut.String())
	}
}

func TestJSONFormatter(t *testing.T) {
	var out bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(true, false, &out, &bytes.Buffer{})

	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", false, 1)
	fmtr.OnSkillSkipped("formatter", sync.SkillStatus{
		Status:    sync.StatusCurrent,
		Installed: &state.InstalledSkill{Revision: 2},
	})
	fmtr.OnSkillSkippedByDenyList("removed", "acme/skills")
	fmtr.OnSkillError("broken", fmt.Errorf("download failed"))
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1, Skipped: 2, Failed: 1})

	if err := fmtr.Flush(); err != nil {
		t.Fatalf("Flush() error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out.String())
	}
	if result["status"] != "partial_success" {
		t.Fatalf("status = %v, want partial_success", result["status"])
	}
	if result["format_version"] != "1" {
		t.Fatalf("format_version = %v, want 1", result["format_version"])
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing from envelope: %v", result)
	}

	registries, ok := data["registries"].([]any)
	if !ok || len(registries) != 1 {
		t.Fatalf("expected 1 registry, got: %v", data["registries"])
	}

	reg := registries[0].(map[string]any)
	if reg["registry"] != "acme/skills" {
		t.Errorf("expected registry 'acme/skills', got: %v", reg["registry"])
	}

	skills := reg["skills"].([]any)
	if len(skills) != 4 {
		t.Errorf("expected 4 skills, got %d", len(skills))
	}

	summary := data["summary"].(map[string]any)
	if summary["installed"].(float64) != 1 {
		t.Errorf("expected installed=1, got: %v", summary["installed"])
	}
	denied := data["skipped_by_deny_list"].([]any)
	if len(denied) != 1 {
		t.Fatalf("expected 1 deny-list skip, got %d", len(denied))
	}
	if denied[0].(map[string]any)["name"] != "removed" {
		t.Fatalf("unexpected deny-list skip payload: %v", denied[0])
	}
}
