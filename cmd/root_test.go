package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRoot_JSONFlag_StdoutIsCleanJSON_EvenDuringBuiltinsBackfill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := newRootCmd()
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	root.SetArgs([]string{"list", "--json"})

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()

	execErr := root.Execute()
	w.Close()

	if execErr != nil {
		var stderr bytes.Buffer
		stderr.ReadFrom(r)
		t.Fatalf("execute: %v (stderr=%s stdout=%s)", execErr, errBuf.String(), stderr.String())
	}

	var outBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	out := strings.TrimSpace(outBuf.String())
	var anyJSON interface{}
	if err := json.Unmarshal([]byte(out), &anyJSON); err != nil {
		t.Errorf("stdout not clean JSON: %v\nstdout: %q", err, out)
	}

	stdout := outBuf.String()
	if strings.Contains(stdout, "Welcome to Scribe") || strings.Contains(stdout, "new built-in") {
		t.Errorf("banner leaked into stdout: %q", stdout)
	}

	stderr := errBuf.String()
	if !strings.Contains(stderr, "Welcome to Scribe") {
		t.Errorf("expected first-run banner on stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, "Naoray/scribe") {
		t.Errorf("expected Naoray/scribe to appear in banner, got: %q", stderr)
	}
}
