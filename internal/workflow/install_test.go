package workflow_test

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestInstallSteps_Composition(t *testing.T) {
	steps := workflow.InstallSteps()
	if len(steps) == 0 {
		t.Fatal("InstallSteps() returned empty list")
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	last := steps[len(steps)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected last step SyncSkills, got %s (install must end with sync)", last.Name)
	}
}

func TestInstallSteps_ContainsSelectSkills(t *testing.T) {
	for _, s := range workflow.InstallSteps() {
		if s.Name == "SelectSkills" {
			return
		}
	}
	t.Error("InstallSteps missing SelectSkills step")
}

func TestStepSelectSkills_NamedArgs(t *testing.T) {
	b := &workflow.Bag{
		Args: []string{"tdd", "commit"},
	}
	if err := workflow.StepSelectSkills(context.Background(), b); err != nil {
		t.Fatalf("StepSelectSkills: %v", err)
	}
	if len(b.SkillFilter) != 2 {
		t.Fatalf("expected SkillFilter len 2, got %d: %v", len(b.SkillFilter), b.SkillFilter)
	}
	if b.SkillFilter[0] != "tdd" || b.SkillFilter[1] != "commit" {
		t.Errorf("SkillFilter = %v, want [tdd commit]", b.SkillFilter)
	}
}

func TestStepSelectSkills_InstallAllFlag(t *testing.T) {
	b := &workflow.Bag{
		InstallAllFlag: true,
	}
	if err := workflow.StepSelectSkills(context.Background(), b); err != nil {
		t.Fatalf("StepSelectSkills with --all: %v", err)
	}
	// --all sets no filter; syncer installs everything
	if b.SkillFilter != nil {
		t.Errorf("expected nil SkillFilter for --all, got %v", b.SkillFilter)
	}
}
