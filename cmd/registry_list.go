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
	return markJSONSupported(cmd)
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)

	bag := &workflow.Bag{
		JSONFlag:   jsonFlag,
		LazyGitHub: true,
		Factory:    newCommandFactory(),
	}

	if jsonFlag {
		steps := workflow.RegistryListSteps()[:2]
		if err := workflow.Run(cmd.Context(), steps, bag); err != nil {
			return err
		}
		out := workflow.BuildRegistryListJSON(bag.Config.EnabledRegistries(), bag.State)
		renderer := jsonRendererForCommand(cmd, jsonFlag)
		if err := renderer.Result(out); err != nil {
			return err
		}
		return renderer.Flush()
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
