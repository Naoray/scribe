package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
)

func TestWrapRunEStampsTiming(t *testing.T) {
	cmd := &cobra.Command{
		Use: "noop",
		RunE: wrapRunE(func(cmd *cobra.Command, args []string) error {
			time.Sleep(time.Millisecond)
			return nil
		}),
	}
	cmd.SetContext(context.WithValue(context.Background(), envelope.BootstrapStartKey, time.Now()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	duration, ok := cmd.Context().Value(envelope.DurationMSKey).(int64)
	if !ok || duration <= 0 {
		t.Fatalf("duration_ms = %v, want > 0", cmd.Context().Value(envelope.DurationMSKey))
	}
	bootstrap, ok := cmd.Context().Value(envelope.BootstrapMSKey).(int64)
	if !ok || bootstrap < 0 {
		t.Fatalf("bootstrap_ms = %v, want >= 0", cmd.Context().Value(envelope.BootstrapMSKey))
	}
}
