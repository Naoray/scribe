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

// Run executes steps sequentially. No retry, no rollback — Scribe operations
// are idempotent.
func Run(ctx context.Context, steps []Step, bag *Bag) error {
	for _, step := range steps {
		if err := step.Fn(ctx, bag); err != nil {
			if errors.Is(err, errSkip) {
				return nil
			}
			return err
		}
	}
	if bag.Formatter != nil {
		return bag.Formatter.Flush()
	}
	return nil
}
