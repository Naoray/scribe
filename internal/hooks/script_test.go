package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestScribeHookScriptOutputsAdditionalContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("requires bash")
	}

	binDir := t.TempDir()
	fakeScribe := filepath.Join(binDir, "scribe")
	if err := os.WriteFile(fakeScribe, []byte(`#!/usr/bin/env bash
set -eu
case "$1 $2" in
  "list --json")
    printf '{"skills":[{"name":"alpha"},{"name":"beta"}]}\n'
    ;;
  "status --json")
    printf '{"installed_count":2,"registries":["owner/repo"],"last_sync":"2026-04-30T10:10:47Z"}\n'
    ;;
  *)
    exit 2
    ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout := runHookScript(t, binDir+string(os.PathListSeparator)+os.Getenv("PATH"), []byte(`{"hook_event_name":"PostToolUseFailure"}`))

	var envelope struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("hook stdout is not valid JSON: %v\n%s", err, stdout)
	}

	context := envelope.HookSpecificOutput.AdditionalContext
	if context == "" {
		t.Fatal("additionalContext is empty")
	}
	for _, want := range []string{
		"Scribe is available on PATH",
		"Installed skills: 2",
		"Examples: alpha, beta",
		"Registry status: 2 installed, 1 registries",
		"scribe explain <name>",
		"scribe doctor",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("additionalContext missing %q:\n%s", want, context)
		}
	}
}

func TestScribeHookScriptHandlesMissingScribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("requires bash")
	}

	stdout := runHookScript(t, t.TempDir(), nil)

	var envelope struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("hook stdout is not valid JSON without scribe: %v\n%s", err, stdout)
	}
	if !strings.Contains(envelope.HookSpecificOutput.AdditionalContext, "scribe not available") {
		t.Fatalf("additionalContext = %q, want missing-scribe message", envelope.HookSpecificOutput.AdditionalContext)
	}
}

func runHookScript(t *testing.T, pathEnv string, stdin []byte) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join("scripts", "scribe-hook.sh"))
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "PATH="+pathEnv)
	cmd.Stdin = bytes.NewReader(stdin)

	stdout, err := cmd.Output()
	if ctx.Err() != nil {
		t.Fatalf("hook script timed out: %v", ctx.Err())
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("hook script failed: %v\nstderr:\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("hook script failed: %v", err)
	}
	return stdout
}
