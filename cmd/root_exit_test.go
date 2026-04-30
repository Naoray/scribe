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
		wantErr    string
		wantStdout string
		stdoutJSON bool
		badHome    bool
	}{
		{name: "unknown command json", args: []string{"--json", "nope"}, wantCode: 2, wantJSON: true, wantErr: "USAGE"},
		{name: "bad flag json", args: []string{"--json", "--bad"}, wantCode: 2, wantJSON: true, wantErr: "USAGE"},
		{name: "help", args: []string{"--help"}, wantCode: 0, wantStdout: "Scribe manages local AI coding agent skills"},
		{name: "version", args: []string{"--version"}, wantCode: 0, wantStdout: "scribe version"},
		{name: "operational pre-run failure json", args: []string{"--json", "list"}, wantCode: 1, wantJSON: true, wantErr: "GENERAL", badHome: true},
		{name: "json unsupported config adoption", args: []string{"--json", "config", "adoption"}, wantCode: 2, wantJSON: true, wantErr: "JSON_NOT_SUPPORTED"},
		{name: "json unsupported resolve", args: []string{"--json", "resolve", "recap"}, wantCode: 2, wantJSON: true, wantErr: "JSON_NOT_SUPPORTED"},
		{name: "json supported schema list", args: []string{"--json", "schema", "list"}, wantCode: 0, stdoutJSON: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runScribeHelper(t, tt.args, tt.badHome)
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
				if tt.wantErr != "" {
					errObj, ok := env["error"].(map[string]any)
					if !ok {
						t.Fatalf("envelope missing error object: %#v", env)
					}
					if got := errObj["code"]; got != tt.wantErr {
						t.Fatalf("error code = %v, want %s\nenvelope=%#v", got, tt.wantErr, env)
					}
				}
				return
			}
			if tt.stdoutJSON {
				var env map[string]any
				if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
					t.Fatalf("stdout is not JSON: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
				}
				if _, ok := env["input_schema"]; !ok {
					t.Fatalf("schema JSON missing input_schema: %#v", env)
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
	home := t.TempDir()
	cmd.Env = append(os.Environ(), "GO_WANT_SCRIBE_HELPER_PROCESS=1", "HOME="+home)
	if badHome {
		homeFile := filepath.Join(t.TempDir(), "home-file")
		if err := os.WriteFile(homeFile, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("write home file: %v", err)
		}
		cmd.Env = append(cmd.Env, "HOME="+homeFile)
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
			os.Exit(0)
			return
		}
	}
	os.Exit(2)
}
