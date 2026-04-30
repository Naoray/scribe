package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSemanticExitCodesSubprocessMatrix(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
		badHome  bool
	}{
		{name: "cobra parse usage", args: []string{"--json", "nonsense-cmd"}, wantCode: 2, wantErr: "USAGE"},
		{name: "schema command not found", args: []string{"--json", "schema", "nonsense-cmd"}, wantCode: 3, wantErr: "SCHEMA_COMMAND_NOT_FOUND"},
		{name: "skill not found", args: []string{"--json", "explain", "nonsense-skill"}, wantCode: 3, wantErr: "SKILL_NOT_FOUND"},
		{name: "auth failure", args: []string{"--json", "add", "Naoray/scribe:scribe-agent"}, wantCode: 4, wantErr: "GH_AUTH_FAILED"},
		{name: "usage validation", args: []string{"--json", "connect"}, wantCode: 2, wantErr: "USAGE_CONNECT_REPO_REQUIRED"},
		{name: "plain operational error", args: []string{"--json", "list"}, wantCode: 1, wantErr: "GENERAL", badHome: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runScribeHelper(t, tt.args, tt.badHome)
			if code != tt.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout=%s\nstderr=%s", code, tt.wantCode, stdout, stderr)
			}
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
				t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if env.Error.Code != tt.wantErr {
				t.Fatalf("error code = %q, want %q\nstdout=%s", env.Error.Code, tt.wantErr, stdout)
			}
		})
	}
}
