package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootExitSubprocessMatrix(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantJSON   bool
		wantStdout string
	}{
		{name: "unknown command json", args: []string{"--json", "nope"}, wantCode: 2, wantJSON: true},
		{name: "bad flag json", args: []string{"--json", "--bad"}, wantCode: 2, wantJSON: true},
		{name: "help", args: []string{"--help"}, wantCode: 0, wantStdout: "Scribe manages local AI coding agent skills"},
		{name: "version", args: []string{"--version"}, wantCode: 0, wantStdout: "scribe version"},
		{name: "pre-run failure json", args: []string{"--json", "list"}, wantCode: 2, wantJSON: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runScribeHelper(t, tt.args, tt.name == "pre-run failure json")
			if code != tt.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout=%s\nstderr=%s", code, tt.wantCode, stdout, stderr)
			}
			if tt.wantJSON {
				var env map[string]any
				if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
					t.Fatalf("stderr is not JSON envelope: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
				}
				if env["status"] != "error" || env["format_version"] != "1" {
					t.Fatalf("unexpected envelope: %#v", env)
				}
				return
			}
			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Fatalf("stdout = %q, want contains %q", stdout, tt.wantStdout)
			}
		})
	}
}

func runScribeHelper(t *testing.T, args []string, badHome bool) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestScribeHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_SCRIBE_HELPER_PROCESS=1")
	if badHome {
		home := filepath.Join(t.TempDir(), "home-file")
		if err := os.WriteFile(home, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("write home file: %v", err)
		}
		cmd.Env = append(cmd.Env, "HOME="+home)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper run: %v", err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

func TestScribeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SCRIBE_HELPER_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"scribe"}, os.Args[i+1:]...)
			Execute()
			return
		}
	}
	os.Exit(2)
}
