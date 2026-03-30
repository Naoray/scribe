package workflow

import "context"

// Step is a named unit of work in a workflow.
type Step struct {
	Name string
	Fn   func(ctx context.Context, b *Bag) error
}

// Run executes steps sequentially, notifying the Formatter at each boundary.
// No retry, no rollback — Scribe operations are idempotent.
func Run(ctx context.Context, steps []Step, bag *Bag) error {
	for i, step := range steps {
		if bag.Formatter != nil {
			bag.Formatter.OnStepStarted(step.Name, i, len(steps))
		}
		if err := step.Fn(ctx, bag); err != nil {
			return err
		}
		if bag.Formatter != nil {
			bag.Formatter.OnStepCompleted(step.Name, i, len(steps))
		}
	}
	if bag.Formatter != nil {
		return bag.Formatter.Flush()
	}
	return nil
}
