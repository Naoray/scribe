package cmd

import (
	"testing"
)

func TestRegistryEnableDisableArgs(t *testing.T) {
	// enableCmd and disableCmd should require exactly one arg.
	enableCmd := newRegistryEnableCommand()
	if enableCmd.Args == nil {
		t.Error("enableCmd should have arg validation")
	}
	disableCmd := newRegistryDisableCommand()
	if disableCmd.Args == nil {
		t.Error("disableCmd should have arg validation")
	}
}
