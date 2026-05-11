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
	fakeJQ := filepath.Join(binDir, "jq")
	if err := os.WriteFile(fakeJQ, []byte(`#!/usr/bin/env bash
set -eu

json_quote() {
  local value=${1//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '"%s"' "$value"
}

if [ "${1:-}" = "-n" ]; then
  context=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--arg" ] && [ "${2:-}" = "context" ]; then
      context=$3
      shift 3
    else
      shift
    fi
  done
  printf '{"hookSpecificOutput":{"additionalContext":'
  json_quote "$context"
  printf '}}\n'
  exit 0
fi

filter="${2:-${1:-}}"
case "$filter" in
  *installed_count*)
    printf '2 installed, 1 registries, last sync 2026-04-30T10:10:47Z\n'
    ;;
  *"def names:"*)
    printf 'alpha, beta\n'
    ;;
  *"then length"*)
    printf '2\n'
    ;;
  *)
    exit 1
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

	ctx := context.Background()
	cancel := func() {}
	if deadline, ok := t.Deadline(); ok {
		if time.Until(deadline) > time.Second {
			deadline = deadline.Add(-time.Second)
		}
		ctx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join("scripts", "scribe-hook.sh"))
	cmd.Dir = "."
	cmd.Env = envWithPath(pathEnv)
	cmd.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("hook script timed out before test deadline: %v\nstderr:\n%s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("hook script failed: %v\nstderr:\n%s", err, stderr.String())
	}
	return stdout.Bytes()
}

func envWithPath(pathEnv string) []string {
	env := os.Environ()
	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			env[i] = "PATH=" + pathEnv
			return env
		}
	}
	return append(env, "PATH="+pathEnv)
}
