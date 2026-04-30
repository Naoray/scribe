package cmd

import (
	"context"
	"encoding/json"
	"strings"
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

func TestWrapRunERendererEmitsTiming(t *testing.T) {
	stdout, stderr, code := runScribeHelper(t, []string{"list", "--json"}, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Meta struct {
			DurationMS  int64 `json:"duration_ms"`
			BootstrapMS int64 `json:"bootstrap_ms"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if env.Meta.DurationMS <= 0 {
		t.Fatalf("duration_ms = %d, want > 0\nenvelope=%s", env.Meta.DurationMS, stdout)
	}
	if env.Meta.BootstrapMS <= 0 {
		t.Fatalf("bootstrap_ms = %d, want > 0\nenvelope=%s", env.Meta.BootstrapMS, stdout)
	}
}
