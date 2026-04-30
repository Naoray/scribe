package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

func newRegistryListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show connected registries",
		Args:  cobra.NoArgs,
		RunE:  runRegistryList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	bag := &workflow.Bag{
		JSONFlag: jsonFlag,
	}

	if err := workflow.Run(cmd.Context(), workflow.RegistryListSteps(), bag); err != nil {
		return err
	}

	// Render results populated by the workflow step.
	if bag.RegistryRepos != nil {
		if len(bag.RegistryRepos) == 0 {
			printRegistryEmpty(os.Stdout)
			return nil
		}
		return printRegistryTable(os.Stdout, bag.RegistryRepos, bag.RegistryCounts, bag.State)
	}
	return nil
}
