package errors

import (
	stderrors "errors"
	"testing"
)

func TestWrapPreservesErrorChainAndAs(t *testing.T) {
	base := stderrors.New("boom")
	err := Wrap(base, "NETWORK", ExitNetwork, WithRetryable(true), WithRemediation("retry"), WithRendered(true))

	var ce *Error
	if !stderrors.As(err, &ce) {
		t.Fatal("errors.As did not find *Error")
	}
	if ce.Code != "NETWORK" || ce.Exit != ExitNetwork || !ce.Retryable || ce.Remediation != "retry" || !ce.Rendered {
		t.Fatalf("wrapped error = %+v", ce)
	}
	if !stderrors.Is(err, base) {
		t.Fatal("wrapped error does not preserve source chain")
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: ExitOK},
		{name: "cli error", err: &Error{Exit: ExitNotFound}, want: ExitNotFound},
		{name: "plain error", err: stderrors.New("plain"), want: ExitGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Fatalf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}
