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
	return &timingRenderer{Renderer: r, cmd: cmd}
}

type timingRenderer struct {
	output.Renderer
	cmd *cobra.Command
}

func (r *timingRenderer) Flush() error {
	ctx := r.cmd.Context()
	if duration, ok := ctx.Value(envelope.DurationMSKey).(int64); ok {
		r.SetMeta("duration_ms", positiveDuration(duration))
	} else if start, ok := ctx.Value(envelope.RunEStartKey).(time.Time); ok {
		r.SetMeta("duration_ms", positiveDuration(time.Since(start).Milliseconds()))
	}

	if bootstrap, ok := ctx.Value(envelope.BootstrapMSKey).(int64); ok {
		r.SetMeta("bootstrap_ms", positiveDuration(bootstrap))
	} else if bootstrapStart, ok := ctx.Value(envelope.BootstrapStartKey).(time.Time); ok {
		if runStart, ok := ctx.Value(envelope.RunEStartKey).(time.Time); ok {
			r.SetMeta("bootstrap_ms", positiveDuration(runStart.Sub(bootstrapStart).Milliseconds()))
		}
	}

	return r.Renderer.Flush()
}

func positiveDuration(ms int64) int64 {
	if ms < 1 {
		return 1
	}
	return ms
}
