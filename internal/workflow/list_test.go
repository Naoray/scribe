package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestListSteps_Composition(t *testing.T) {
	steps := workflow.ListSteps()
	if len(steps) == 0 {
		t.Fatal("ListSteps() returned empty list")
	}

	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "BranchLocalOrRemote" {
		t.Errorf("expected last step BranchLocalOrRemote, got %s", steps[len(steps)-1].Name)
	}
}
