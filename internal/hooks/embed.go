package hooks

import _ "embed"

//go:embed scripts/scribe-hook.sh
var script []byte

// Script returns the embedded Claude Code hook script.
func Script() []byte {
	out := make([]byte, len(script))
	copy(out, script)
	return out
}
