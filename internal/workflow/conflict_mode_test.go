package workflow

import (
	"os"
	"testing"
)

func TestResolveConflictModeMatrix(t *testing.T) {
	oldIsTerminal := conflictModeIsTerminal
	t.Cleanup(func() { conflictModeIsTerminal = oldIsTerminal })

	stdin := os.NewFile(1, "stdin")
	stdout := os.NewFile(2, "stdout")

	tests := []struct {
		name       string
		stdinTTY   bool
		stdoutTTY  bool
		jsonForced bool
		want       ConflictMode
	}{
		{name: "both tty text", stdinTTY: true, stdoutTTY: true, want: ConflictModeInteractive},
		{name: "tty stdin non tty stdout", stdinTTY: true, want: ConflictModeJSONEnvelope},
		{name: "non tty stdin tty stdout", stdoutTTY: true, want: ConflictModeJSONEnvelope},
		{name: "both non tty", want: ConflictModeJSONEnvelope},
		{name: "json forced", stdinTTY: true, stdoutTTY: true, jsonForced: true, want: ConflictModeJSONEnvelope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflictModeIsTerminal = func(fd uintptr) bool {
				switch fd {
				case stdin.Fd():
					return tt.stdinTTY
				case stdout.Fd():
					return tt.stdoutTTY
				default:
					return false
				}
			}

			if got := ResolveConflictMode(stdin, stdout, tt.jsonForced); got != tt.want {
				t.Fatalf("mode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUseJSONOutputUsesConflictModeMatrix(t *testing.T) {
	oldIsTerminal := conflictModeIsTerminal
	t.Cleanup(func() { conflictModeIsTerminal = oldIsTerminal })

	stdin := os.NewFile(3, "stdin")
	stdout := os.NewFile(4, "stdout")

	tests := []struct {
		name       string
		stdinTTY   bool
		stdoutTTY  bool
		jsonForced bool
		want       bool
	}{
		{name: "both tty text", stdinTTY: true, stdoutTTY: true, want: false},
		{name: "both tty json forced", stdinTTY: true, stdoutTTY: true, jsonForced: true, want: true},
		{name: "non tty stdout", stdinTTY: true, want: true},
		{name: "non tty stdin", stdoutTTY: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflictModeIsTerminal = func(fd uintptr) bool {
				switch fd {
				case stdin.Fd():
					return tt.stdinTTY
				case stdout.Fd():
					return tt.stdoutTTY
				default:
					return false
				}
			}

			if got := UseJSONOutput(stdin, stdout, tt.jsonForced); got != tt.want {
				t.Fatalf("useJSON = %v, want %v", got, tt.want)
			}
		})
	}
}
