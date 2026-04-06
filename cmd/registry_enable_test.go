package cmd

import (
	"testing"
)

func TestRegistryEnableDisableArgs(t *testing.T) {
	// enableCmd and disableCmd should require exactly one arg.
	if registryEnableCmd.Args == nil {
		t.Error("enableCmd should have arg validation")
	}
	if registryDisableCmd.Args == nil {
		t.Error("disableCmd should have arg validation")
	}
}
