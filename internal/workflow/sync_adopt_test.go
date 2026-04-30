package workflow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	isync "github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

// ---------------------------------------------------------------------------
// Recording formatter
// ---------------------------------------------------------------------------

type adoptRecorder struct {
	skipped           []string
	started           []int
	adopted           []adoptedCall
	errors            []adoptErrCall
	conflictsDeferred []int
	complete          []adoptCompleteCall
}

type adoptedCall struct {
	name  string
	tools []string
}

type adoptErrCall struct {
	name string
	err  error
}

type adoptCompleteCall struct {
	adopted, skipped, failed int
}

// No-ops for non-adoption Formatter methods.
func (r *adoptRecorder) OnRegistryStart(_ string)                                 {}
func (r *adoptRecorder) OnSkillResolved(_ string, _ isync.SkillStatus)            {}
func (r *adoptRecorder) OnSkillDownloading(_ string)                              {}
func (r *adoptRecorder) OnSkillInstalled(_ string, _ bool)                        {}
func (r *adoptRecorder) OnSkillSkipped(_ string, _ isync.SkillStatus)             {}
func (r *adoptRecorder) OnSkillSkippedByDenyList(_, _ string)                     {}
func (r *adoptRecorder) OnSkillError(_ string, _ error)                           {}
func (r *adoptRecorder) OnSyncComplete(_ isync.SyncCompleteMsg)                   {}
func (r *adoptRecorder) OnReconcileConflict(_ string, _ state.ProjectionConflict) {}
func (r *adoptRecorder) OnReconcileComplete(_ isync.ReconcileCompleteMsg)         {}
func (r *adoptRecorder) OnLegacyFormat(_ string)                                  {}
func (r *adoptRecorder) OnConnectDuplicate(_ string)                              {}
func (r *adoptRecorder) OnConnectSaved(_ string)                                  {}
func (r *adoptRecorder) OnConnectSyncing()                                        {}
func (r *adoptRecorder) OnConnectSyncWarning(_ string, _ error)                   {}
func (r *adoptRecorder) OnConnectAvailable(_ string, _ int)                       {}
func (r *adoptRecorder) OnPackageInstallPrompt(_, _, _ string)                    {}
func (r *adoptRecorder) OnPackageApproved(_ string)                               {}
func (r *adoptRecorder) OnPackageDenied(_ string)                                 {}
func (r *adoptRecorder) OnPackageSkipped(_ string, _ string)                      {}
func (r *adoptRecorder) OnPackageInstalling(_ string)                             {}
func (r *adoptRecorder) OnPackageInstalled(_ string)                              {}
func (r *adoptRecorder) OnPackageUpdating(_ string)                               {}
func (r *adoptRecorder) OnPackageUpdated(_ string)                                {}
func (r *adoptRecorder) OnPackageError(_ string, _ error, _ string)               {}
func (r *adoptRecorder) OnPackageHashMismatch(_, _, _, _ string)                  {}
func (r *adoptRecorder) Flush() error                                             { return nil }

func (r *adoptRecorder) OnAdoptionSkipped(reason string) {
	r.skipped = append(r.skipped, reason)
}
func (r *adoptRecorder) OnAdoptionStarted(n int) {
	r.started = append(r.started, n)
}
func (r *adoptRecorder) OnAdopted(name string, targetTools []string) {
	r.adopted = append(r.adopted, adoptedCall{name: name, tools: targetTools})
}
func (r *adoptRecorder) OnAdoptionError(name string, err error) {
	r.errors = append(r.errors, adoptErrCall{name: name, err: err})
}
func (r *adoptRecorder) OnAdoptionConflictsDeferred(count int) {
	r.conflictsDeferred = append(r.conflictsDeferred, count)
}
func (r *adoptRecorder) OnAdoptionComplete(adopted, skipped, failed int) {
	r.complete = append(r.complete, adoptCompleteCall{adopted, skipped, failed})
}

// Compile-time interface check.
var _ workflow.Formatter = (*adoptRecorder)(nil)

// ---------------------------------------------------------------------------
// Mock Tool
// ---------------------------------------------------------------------------

type mockAdoptTool struct {
	name       string
	installErr error
	installed  []string
}

func (m *mockAdoptTool) Name() string             { return m.name }
func (m *mockAdoptTool) Detect() bool             { return true }
func (m *mockAdoptTool) Uninstall(_ string) error { return nil }
func (m *mockAdoptTool) Install(skillName, _ string) ([]string, error) {
	if m.installErr != nil {
		return nil, m.installErr
	}
	p := filepath.Join(m.name, skillName)
	m.installed = append(m.installed, p)
	return []string{p}, nil
}

func (m *mockAdoptTool) SkillPath(skillName string) (string, error) {
	return filepath.Join(m.name, skillName), nil
}

func (m *mockAdoptTool) CanonicalTarget(_ string) (string, bool) { return "", false }

var _ tools.Tool = (*mockAdoptTool)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTestSkill(t *testing.T, parentDir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(parentDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func freshState(t *testing.T, home string) *state.State {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".scribe"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &state.State{
		SchemaVersion: 4,
		Installed:     make(map[string]state.InstalledSkill),
	}
}

func setupScribeStore(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".scribe", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestStepAdopt_ModeOff(t *testing.T) {
	rec := &adoptRecorder{}
	b := &workflow.Bag{
		Config:    &config.Config{Adoption: config.AdoptionConfig{Mode: "off"}},
		State:     &state.State{Installed: make(map[string]state.InstalledSkill)},
		Formatter: rec,
	}

	if err := workflow.StepAdopt(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rec.skipped) != 0 || len(rec.started) != 0 || len(rec.adopted) != 0 {
		t.Errorf("expected no events; got skipped=%v started=%v adopted=%v",
			rec.skipped, rec.started, rec.adopted)
	}
	if len(b.State.Installed) != 0 {
		t.Errorf("state must not be mutated; got %v", b.State.Installed)
	}
}

func TestStepAdopt_ModeAutoNoCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec := &adoptRecorder{}
	b := &workflow.Bag{
		Config:    &config.Config{Adoption: config.AdoptionConfig{Mode: "auto"}},
		State:     freshState(t, home),
		Formatter: rec,
	}

	if err := workflow.StepAdopt(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rec.started) != 0 {
		t.Errorf("expected no OnAdoptionStarted (zero candidates), got %v", rec.started)
	}
	if len(rec.complete) != 0 {
		t.Errorf("expected no OnAdoptionComplete (zero candidates), got %v", rec.complete)
	}
}

func TestStepAdopt_ModeAutoAdoptsClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupScribeStore(t, home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeTestSkill(t, claudeSkills, "my-skill", "# my-skill\nsome content")

	tool := &mockAdoptTool{name: "claude"}
	rec := &adoptRecorder{}
	b := &workflow.Bag{
		Config:    &config.Config{Adoption: config.AdoptionConfig{Mode: "auto"}},
		State:     freshState(t, home),
		Tools:     []tools.Tool{tool},
		Formatter: rec,
	}

	if err := workflow.StepAdopt(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rec.started) == 0 {
		t.Fatal("expected OnAdoptionStarted to be called")
	}
	if len(rec.adopted) != 1 || rec.adopted[0].name != "my-skill" {
		t.Errorf("expected adopted=[my-skill], got %v", rec.adopted)
	}
	if _, ok := b.State.Installed["my-skill"]; !ok {
		t.Error("expected my-skill in state.Installed after adoption")
	}
	if b.State.Installed["my-skill"].Origin != state.OriginLocal {
		t.Errorf("expected origin=local, got %v", b.State.Installed["my-skill"].Origin)
	}
}

func TestStepAdopt_PromptOnNonTTY_Degrades(t *testing.T) {
	rec := &adoptRecorder{}
	b := &workflow.Bag{
		Config:    &config.Config{Adoption: config.AdoptionConfig{Mode: "prompt"}},
		State:     &state.State{Installed: make(map[string]state.InstalledSkill)},
		JSONFlag:  true, // forces non-TTY path
		Formatter: rec,
	}

	if err := workflow.StepAdopt(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rec.skipped) == 0 {
		t.Fatal("expected OnAdoptionSkipped to be called")
	}
	if rec.skipped[0] == "" {
		t.Error("expected non-empty reason in OnAdoptionSkipped")
	}
	if len(rec.started) != 0 {
		t.Errorf("expected no adoption activity, got started=%v", rec.started)
	}
}

func TestStepAdopt_ErrorDoesNotAbortSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupScribeStore(t, home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeTestSkill(t, claudeSkills, "broken-skill", "# broken")

	failTool := &mockAdoptTool{
		name:       "claude",
		installErr: errors.New("disk full"),
	}

	rec := &adoptRecorder{}
	b := &workflow.Bag{
		Config:    &config.Config{Adoption: config.AdoptionConfig{Mode: "auto"}},
		State:     freshState(t, home),
		Tools:     []tools.Tool{failTool},
		Formatter: rec,
	}

	// Must not return error regardless of adoption failure.
	if err := workflow.StepAdopt(context.Background(), b); err != nil {
		t.Fatalf("StepAdopt must be non-fatal; got: %v", err)
	}

	// Error must be routed to the formatter, not propagated.
	if len(rec.errors) != 1 {
		t.Fatalf("expected 1 adoption error, got %d", len(rec.errors))
	}
	if rec.errors[0].name != "broken-skill" {
		t.Errorf("expected error for broken-skill, got %q", rec.errors[0].name)
	}
}

// SyncSteps must contain Adopt between ResolveTools and SyncSkills.
func TestSyncSteps_ContainsAdopt(t *testing.T) {
	steps := workflow.SyncSteps()

	idx := func(name string) int {
		for i, s := range steps {
			if s.Name == name {
				return i
			}
		}
		return -1
	}

	adoptIdx := idx("Adopt")
	rtIdx := idx("ResolveTools")
	ssIdx := idx("SyncSkills")

	if adoptIdx < 0 {
		t.Fatal("Adopt step not found in SyncSteps")
	}
	if adoptIdx <= rtIdx {
		t.Errorf("Adopt (%d) must be after ResolveTools (%d)", adoptIdx, rtIdx)
	}
	if adoptIdx >= ssIdx {
		t.Errorf("Adopt (%d) must be before SyncSkills (%d)", adoptIdx, ssIdx)
	}
}

// Adopt must NOT be in SyncTail (used by connect which shouldn't run adoption).
func TestSyncTail_NoAdopt(t *testing.T) {
	for _, s := range workflow.SyncTail() {
		if s.Name == "Adopt" {
			t.Error("Adopt step must not be in SyncTail")
		}
	}
}
