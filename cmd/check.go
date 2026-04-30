package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	isync "github.com/Naoray/scribe/internal/sync"
)

func newCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check registries for lockfile updates without modifying anything",
		Args:  cobra.NoArgs,
		RunE:  runCheck,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	client, err := factory.Client()
	if err != nil {
		return err
	}
	provider, err := factory.Provider()
	if err != nil {
		return err
	}
	st, err := factory.State()
	if err != nil {
		return err
	}
	repos := cfg.TeamRepos()
	out, _, err := buildLockPlan(cmd.Context(), repos, isync.WrapGitHubClient(client), provider, st)
	if err != nil {
		return err
	}
	if jsonFlag {
		return renderMutatorEnvelope(cmd, out, envelope.StatusOK)
	}
	if len(out.Updates) == 0 {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "No lockfile updates available.")
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
