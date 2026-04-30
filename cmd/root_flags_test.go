package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRootFlags(t *testing.T) {
	root := newRootCmd()
	walkCommands(t, root, func(cmd *cobra.Command) {
		if cmd.Root().PersistentFlags().Lookup("json") == nil {
			t.Fatalf("%s: root missing persistent --json", cmd.CommandPath())
		}
		if cmd.CommandPath() == "scribe list" {
			if cmd.Flags().Lookup("fields") == nil {
				t.Fatalf("%s: missing --fields opt-in", cmd.CommandPath())
			}
			return
		}
		if cmd.Flags().Lookup("fields") != nil {
			t.Fatalf("%s: unexpected --fields; use output.AttachFieldsFlag for opt-in commands", cmd.CommandPath())
		}
	})
}

func walkCommands(t *testing.T, cmd *cobra.Command, visit func(*cobra.Command)) {
	t.Helper()
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(t, child, visit)
	}
}
