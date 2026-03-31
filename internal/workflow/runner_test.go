package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestRun_ExecutesStepsInOrder(t *testing.T) {
	var order []string

	steps := []workflow.Step{
		{"A", func(_ context.Context, _ *workflow.Bag) error {
			order = append(order, "A")
			return nil
		}},
		{"B", func(_ context.Context, _ *workflow.Bag) error {
			order = append(order, "B")
			return nil
		}},
		{"C", func(_ context.Context, _ *workflow.Bag) error {
			order = append(order, "C")
			return nil
		}},
	}

	bag := &workflow.Bag{}
	err := workflow.Run(context.Background(), steps, bag)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(order) != 3 || order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Errorf("expected [A B C], got %v", order)
	}
}

func TestRun_StopsOnError(t *testing.T) {
	boom := errors.New("boom")
	var order []string

	steps := []workflow.Step{
		{"A", func(_ context.Context, _ *workflow.Bag) error {
			order = append(order, "A")
			return nil
		}},
		{"B", func(_ context.Context, _ *workflow.Bag) error {
			return boom
		}},
		{"C", func(_ context.Context, _ *workflow.Bag) error {
			order = append(order, "C")
			return nil
		}},
	}

	bag := &workflow.Bag{}
	err := workflow.Run(context.Background(), steps, bag)
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got: %v", err)
	}
	if len(order) != 1 || order[0] != "A" {
		t.Errorf("expected only [A] to have run, got %v", order)
	}
}

func TestRun_EmptySteps(t *testing.T) {
	bag := &workflow.Bag{}
	if err := workflow.Run(context.Background(), nil, bag); err != nil {
		t.Fatalf("Run(nil steps) error: %v", err)
	}
}

func TestSyncSteps_Composition(t *testing.T) {
	steps := workflow.SyncSteps()
	if len(steps) == 0 {
		t.Fatal("SyncSteps() returned empty list")
	}

	// First step should be LoadConfig, last should be SyncSkills
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "SyncSkills" {
		t.Errorf("expected last step SyncSkills, got %s", steps[len(steps)-1].Name)
	}
}

func TestSyncTail_IsSubsetOfSyncSteps(t *testing.T) {
	full := workflow.SyncSteps()
	tail := workflow.SyncTail()

	if len(tail) == 0 {
		t.Fatal("SyncTail() returned empty list")
	}

	// The tail should match the last N steps of full sync.
	offset := len(full) - len(tail)
	if offset < 0 {
		t.Fatal("SyncTail is longer than SyncSteps")
	}

	for i, step := range tail {
		if full[offset+i].Name != step.Name {
			t.Errorf("tail[%d] name %q != full[%d] name %q", i, step.Name, offset+i, full[offset+i].Name)
		}
	}
}
