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
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, false, &out, &errOut)

	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", "v1.0.0", false)
	fmtr.OnSkillSkipped("formatter", sync.SkillStatus{
		Installed: &state.InstalledSkill{Version: "v2.0.0"},
	})
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1, Skipped: 1})
	fmtr.Flush()

	// stderr should have the "syncing ..." header (single-registry mode)
	if !strings.Contains(errOut.String(), "syncing acme/skills") {
		t.Errorf("expected 'syncing acme/skills' header, got stderr: %s", errOut.String())
	}

	// stdout should have skill lines and done summary
	if !strings.Contains(out.String(), "linter") {
		t.Errorf("expected 'linter' in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "done: 1 installed") {
		t.Errorf("expected 'done: 1 installed' in output, got: %s", out.String())
	}
}

func TestTextFormatter_MultiRegistry(t *testing.T) {
	var out, errOut bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(false, true, &out, &errOut)

	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", "v1.0.0", false)
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1})

	fmtr.OnRegistryStart("acme/tools")
	fmtr.OnSkillInstalled("debugger", "v2.0.0", true)
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Updated: 1})

	fmtr.Flush()

	// stderr should have "── repo ──" headers
	if !strings.Contains(errOut.String(), "── acme/skills ──") {
		t.Errorf("expected '── acme/skills ──' header, got stderr: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "── acme/tools ──") {
		t.Errorf("expected '── acme/tools ──' header, got stderr: %s", errOut.String())
	}

	// summary should aggregate across registries
	if !strings.Contains(out.String(), "1 installed, 1 updated") {
		t.Errorf("expected aggregated summary, got: %s", out.String())
	}
}

func TestJSONFormatter(t *testing.T) {
	var out bytes.Buffer
	fmtr := workflow.NewFormatterWithWriters(true, false, &out, &bytes.Buffer{})

	fmtr.OnRegistryStart("acme/skills")
	fmtr.OnSkillInstalled("linter", "v1.0.0", false)
	fmtr.OnSkillSkipped("formatter", sync.SkillStatus{
		Status:    sync.StatusCurrent,
		Installed: &state.InstalledSkill{Version: "v2.0.0"},
	})
	fmtr.OnSkillError("broken", fmt.Errorf("download failed"))
	fmtr.OnSyncComplete(sync.SyncCompleteMsg{Installed: 1, Skipped: 1, Failed: 1})

	if err := fmtr.Flush(); err != nil {
		t.Fatalf("Flush() error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out.String())
	}

	registries, ok := result["registries"].([]any)
	if !ok || len(registries) != 1 {
		t.Fatalf("expected 1 registry, got: %v", result["registries"])
	}

	reg := registries[0].(map[string]any)
	if reg["registry"] != "acme/skills" {
		t.Errorf("expected registry 'acme/skills', got: %v", reg["registry"])
	}

	skills := reg["skills"].([]any)
	if len(skills) != 3 {
		t.Errorf("expected 3 skills, got %d", len(skills))
	}

	summary := result["summary"].(map[string]any)
	if summary["installed"].(float64) != 1 {
		t.Errorf("expected installed=1, got: %v", summary["installed"])
	}
}
