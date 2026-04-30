package hooks

import (
	"bytes"
	"testing"
)

func TestScriptEmbedsHook(t *testing.T) {
	t.Parallel()

	got := Script()
	if len(got) < 100 {
		t.Fatalf("Script() length = %d, want nontrivial embedded script", len(got))
	}
	if !bytes.HasPrefix(got, []byte("#!/usr/bin/env bash\n")) {
		t.Fatalf("Script() does not start with bash shebang: %q", got[:min(len(got), 32)])
	}
	if !bytes.Contains(got, []byte("hookSpecificOutput")) {
		t.Fatal("Script() does not contain Claude Code hook output key")
	}
}

func TestScriptReturnsCopy(t *testing.T) {
	t.Parallel()

	first := Script()
	first[0] = 'x'

	second := Script()
	if second[0] != '#' {
		t.Fatal("Script() returned mutable embedded backing storage")
	}
}
