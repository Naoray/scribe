package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
)

func renderMutatorEnvelope(cmd *cobra.Command, data any, status envelope.Status) error {
	renderer := jsonRendererForCommand(cmd, jsonFlagPassed(cmd))
	renderer.SetStatus(status)
	if version, ok := cmd.Context().Value(envelope.ScribeVersionKey).(string); ok {
		renderer.SetMeta("scribe_version", version)
	}
	if command, ok := cmd.Context().Value(envelope.CommandPathKey).(string); ok {
		renderer.SetMeta("command", command)
	}
	if start, ok := cmd.Context().Value(envelope.RunEStartKey).(time.Time); ok {
		renderer.SetMeta("duration_ms", positiveDuration(time.Since(start).Milliseconds()))
	}
	if bootstrap, ok := cmd.Context().Value(envelope.BootstrapMSKey).(int64); ok {
		renderer.SetMeta("bootstrap_ms", positiveDuration(bootstrap))
	}
	if err := renderer.Result(data); err != nil {
		return err
	}
	return renderer.Flush()
}
