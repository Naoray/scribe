package cmd

import (
	"os"
	"time"

	clienv "github.com/Naoray/scribe/internal/cli/env"
	"github.com/Naoray/scribe/internal/cli/envelope"
	"github.com/Naoray/scribe/internal/cli/output"

	"github.com/spf13/cobra"
)

func jsonRendererForCommand(cmd *cobra.Command, jsonFlag bool) output.Renderer {
	mode := clienv.Detect(os.Stdout, os.Stdin, jsonFlag)
	r := output.New(mode, cmd.OutOrStdout(), cmd.ErrOrStderr())
	r.SetMeta("command", cmd.CommandPath())
	if version, ok := cmd.Context().Value(envelope.ScribeVersionKey).(string); ok {
		r.SetMeta("scribe_version", version)
	}
	if start, ok := cmd.Context().Value(envelope.RunEStartKey).(time.Time); ok {
		duration := time.Since(start).Milliseconds()
		if duration < 1 {
			duration = 1
		}
		r.SetMeta("duration_ms", duration)
	}
	if bootstrapStart, ok := cmd.Context().Value(envelope.BootstrapStartKey).(time.Time); ok {
		if runStart, ok := cmd.Context().Value(envelope.RunEStartKey).(time.Time); ok {
			bootstrap := runStart.Sub(bootstrapStart).Milliseconds()
			if bootstrap < 0 {
				bootstrap = 0
			}
			r.SetMeta("bootstrap_ms", bootstrap)
		}
	}
	return r
}
