package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/registryindex"
)

func newRegistryIndexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Show cached public registry index",
		Args:  cobra.NoArgs,
		RunE:  runRegistryIndex,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runRegistryIndex(cmd *cobra.Command, args []string) error {
	path, err := registryindex.Path()
	if err != nil {
		return err
	}
	idx, err := registryindex.Load(path)
	if err != nil {
		return err
	}

	jsonFlag := jsonFlagPassed(cmd)
	if jsonFlag {
		renderer := jsonRendererForCommand(cmd, jsonFlag)
		if err := renderer.Result(idx); err != nil {
			return err
		}
		return renderer.Flush()
	}

	if len(idx.Registries) == 0 {
		fmt.Fprintln(os.Stdout, "No public registries indexed.")
		return nil
	}
	for _, entry := range idx.Registries {
		fmt.Fprintf(os.Stdout, "%s (%d skills, %d kits)\n", entry.Repo, entry.SkillCount, entry.KitCount)
	}
	return nil
}
