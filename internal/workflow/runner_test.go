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

	// First step should be LoadConfig, last should be the final system reconcile.
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "ReconcileSystem" {
		t.Errorf("expected last step ReconcileSystem, got %s", steps[len(steps)-1].Name)
	}
}

func TestSyncTail_Composition(t *testing.T) {
	tail := workflow.SyncTail()

	if len(tail) == 0 {
		t.Fatal("SyncTail() returned empty list")
	}

	// SyncTail is the shared sync suffix reused by connect and create-registry.
	// It must end with ReconcileSystem and must NOT contain Adopt (adoption is a
	// sync-only prelude; connect flows should not trigger adoption).
	last := tail[len(tail)-1]
	if last.Name != "ReconcileSystem" {
		t.Errorf("expected SyncTail to end with ReconcileSystem, got %s", last.Name)
	}

	for _, s := range tail {
		if s.Name == "Adopt" {
			t.Error("Adopt must not appear in SyncTail")
		}
	}
}
