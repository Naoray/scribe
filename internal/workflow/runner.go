package workflow

import (
	"context"
	"errors"
)

// Step is a named unit of work in a workflow.
type Step struct {
	Name string
	Fn   func(ctx context.Context, b *Bag) error
}

// errSkip is a sentinel error returned by steps that want to skip all
// remaining steps (e.g. DedupCheck finds the repo is already connected).
var errSkip = errors.New("skip remaining steps")

// Run executes steps sequentially, notifying the Formatter at each boundary.
// No retry, no rollback — Scribe operations are idempotent.
func Run(ctx context.Context, steps []Step, bag *Bag) error {
	for i, step := range steps {
		if bag.Formatter != nil {
			bag.Formatter.OnStepStarted(step.Name, i, len(steps))
		}
		if err := step.Fn(ctx, bag); err != nil {
			if errors.Is(err, errSkip) {
				return nil
			}
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
