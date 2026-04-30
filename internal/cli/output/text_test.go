package output

import (
	"bytes"
	"testing"

	"github.com/Naoray/scribe/internal/cli/env"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

func TestTextRendererShape(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(env.Mode{Format: env.FormatText}, &stdout, &stderr)
	if err := r.Result("hello"); err != nil {
		t.Fatalf("Result: %v", err)
	}
	if err := r.Error(&clierrors.Error{Code: "BAD", Message: "bad", Remediation: "fix it"}); err != nil {
		t.Fatalf("Error: %v", err)
	}

	if stdout.String() != "hello\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	wantErr := "error[BAD]: bad\nremediation: fix it\n"
	if stderr.String() != wantErr {
		t.Fatalf("stderr = %q, want %q", stderr.String(), wantErr)
	}
}
