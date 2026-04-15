package cmd

import "github.com/Naoray/scribe/internal/workflow"

func saveWorkflowState(bag *workflow.Bag) error {
	if bag == nil || !bag.StateDirty || bag.State == nil {
		return nil
	}
	return bag.State.Save()
}
