package workflow

import (
	"os"

	"github.com/mattn/go-isatty"
)

// ConflictMode describes how name conflicts may be resolved for the current IO
// shape and output format.
type ConflictMode int

const (
	ConflictModeInteractive ConflictMode = iota
	ConflictModeJSONEnvelope
)

var conflictModeIsTerminal = isatty.IsTerminal

// ResolveConflictMode centralizes the prompt-vs-envelope decision for name
// conflict handling across sync, install, and add.
func ResolveConflictMode(stdin, stdout *os.File, jsonForced bool) ConflictMode {
	stdinTTY := stdin != nil && conflictModeIsTerminal(stdin.Fd())
	stdoutTTY := stdout != nil && conflictModeIsTerminal(stdout.Fd())
	if stdinTTY && stdoutTTY && !jsonForced {
		return ConflictModeInteractive
	}
	return ConflictModeJSONEnvelope
}

func ConflictModeForProcess(jsonForced bool) ConflictMode {
	return ResolveConflictMode(os.Stdin, os.Stdout, jsonForced)
}

func UseJSONOutput(stdin, stdout *os.File, jsonForced bool) bool {
	return ResolveConflictMode(stdin, stdout, jsonForced) == ConflictModeJSONEnvelope
}

func UseJSONOutputForProcess(jsonForced bool) bool {
	return UseJSONOutput(os.Stdin, os.Stdout, jsonForced)
}
