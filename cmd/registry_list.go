package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show connected registries",
	Args:  cobra.NoArgs,
	RunE:  runRegistryList,
}

func init() {
	registryListCmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	bag := &workflow.Bag{
		JSONFlag: jsonFlag,
	}

	return workflow.Run(cmd.Context(), workflow.RegistryListSteps(), bag)
}
