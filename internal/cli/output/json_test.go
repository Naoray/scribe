package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Naoray/scribe/internal/cli/env"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

func TestJSONRendererRoundTrip(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(env.Mode{Format: env.FormatJSON}, &stdout, &stderr)
	r.Progress("loading")
	r.SetMeta("command", "scribe list")
	r.SetStatus(envelope.StatusOK)
	if err := r.Result(map[string]any{"name": "recap"}); err != nil {
		t.Fatalf("Result: %v", err)
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}

	if stderr.String() != "loading\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	lines := bytes.Count(stdout.Bytes(), []byte("\n"))
	if lines != 1 {
		t.Fatalf("Flush emitted %d lines, want 1: %q", lines, stdout.String())
	}

	var got envelope.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != envelope.StatusOK || got.FormatVersion != envelope.FormatVersion {
		t.Fatalf("envelope = %+v", got)
	}
	if got.Meta.Command != "scribe list" {
		t.Fatalf("command meta = %q", got.Meta.Command)
	}
}

func TestJSONRendererError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(env.Mode{Format: env.FormatJSON}, &stdout, &stderr)
	err := &clierrors.Error{Code: "BAD", Message: "bad", Exit: clierrors.ExitUsage}
	if writeErr := r.Error(err); writeErr != nil {
		t.Fatalf("Error: %v", writeErr)
	}

	var got envelope.Envelope
	if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
		t.Fatalf("unmarshal: %v", decodeErr)
	}
	if got.Status != envelope.StatusError || got.Error.Code != "BAD" {
		t.Fatalf("envelope = %+v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
