package cmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
)

func TestInstallNameConflictJSONEnvelope(t *testing.T) {
	home := setupNameConflictHome(t)
	realSkillPath := filepath.Join(home, ".claude", "skills", "good-skill")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir conflict dir: %v", err)
	}
	writeNameConflictState(t, home)

	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"sync", "--json", "--registry", "acme/team"})
	if code != 5 {
		t.Fatalf("exit = %d, want 5\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Status string `json:"status"`
		Data   struct {
			Resolution nameConflictResolutionPayload `json:"resolution"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
			Exit int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if env.Status != "error" || env.Error.Code != "SYNC_NAME_CONFLICT" || env.Error.Exit != 5 {
		t.Fatalf("unexpected envelope: %+v\nstdout=%s", env, stdout)
	}
	if env.Data.Resolution.Skill != "good-skill" ||
		env.Data.Resolution.Action != "unresolved" ||
		env.Data.Resolution.Path != realSkillPath {
		t.Fatalf("resolution = %+v, want unresolved good-skill at %s", env.Data.Resolution, realSkillPath)
	}
}

func TestSyncNameConflictEnvelopeWhenStdoutNonTTYAndStdinTTY(t *testing.T) {
	home := setupNameConflictHome(t)
	realSkillPath := filepath.Join(home, ".claude", "skills", "good-skill")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir conflict dir: %v", err)
	}
	writeNameConflictState(t, home)

	stdout, stderr, code := runScribeHelperWithTTYStdin(t, home, []string{"sync", "--registry", "acme/team"})
	assertNameConflictEnvelope(t, stdout, stderr, code, realSkillPath)
}

func TestSyncNameConflictEnvelopeWhenStdinNonTTYAndStdoutTTY(t *testing.T) {
	home := setupNameConflictHome(t)
	realSkillPath := filepath.Join(home, ".claude", "skills", "good-skill")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir conflict dir: %v", err)
	}
	writeNameConflictState(t, home)

	stdout, stderr, code := runScribeHelperWithTTYStdout(t, home, []string{"sync", "--registry", "acme/team"})
	assertNameConflictEnvelope(t, stdout, stderr, code, realSkillPath)
}

func TestSyncNameConflictEnvelopeWhenBothNonTTY(t *testing.T) {
	home := setupNameConflictHome(t)
	realSkillPath := filepath.Join(home, ".claude", "skills", "good-skill")
	if err := os.MkdirAll(realSkillPath, 0o755); err != nil {
		t.Fatalf("mkdir conflict dir: %v", err)
	}
	writeNameConflictState(t, home)

	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"sync", "--registry", "acme/team"})
	assertNameConflictEnvelope(t, stdout, stderr, code, realSkillPath)
}

func assertNameConflictEnvelope(t *testing.T, stdout, stderr string, code int, realSkillPath string) {
	t.Helper()
	if code != 5 {
		t.Fatalf("exit = %d, want 5\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	var env struct {
		Status string `json:"status"`
		Data   struct {
			Resolution nameConflictResolutionPayload `json:"resolution"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
			Exit int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if env.Status != "error" || env.Error.Code != "SYNC_NAME_CONFLICT" || env.Error.Exit != 5 {
		t.Fatalf("unexpected envelope: %+v\nstdout=%s", env, stdout)
	}
	if env.Data.Resolution.Skill != "good-skill" ||
		env.Data.Resolution.Action != "unresolved" ||
		env.Data.Resolution.Path != realSkillPath {
		t.Fatalf("resolution = %+v, want unresolved good-skill at %s", env.Data.Resolution, realSkillPath)
	}
}

func runScribeHelperWithTTYStdin(t *testing.T, home string, args []string) (string, string, int) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })

	cmd := scribeHelperCommand(t, home, args)
	cmd.Stdin = tty
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Start()
	_ = tty.Close()
	if err != nil {
		t.Fatalf("helper start: %v", err)
	}
	code := waitExitCode(t, cmd)
	return stdout.String(), stderr.String(), code
}

func runScribeHelperWithTTYStdout(t *testing.T, home string, args []string) (string, string, int) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })

	cmd := scribeHelperCommand(t, home, args)
	cmd.Stdin = strings.NewReader("piped input\n")
	cmd.Stdout = tty
	var stderr strings.Builder
	cmd.Stderr = &stderr

	err = cmd.Start()
	_ = tty.Close()
	if err != nil {
		t.Fatalf("helper start: %v", err)
	}
	out, readErr := io.ReadAll(ptmx)
	if readErr != nil && !strings.Contains(readErr.Error(), "input/output error") {
		t.Fatalf("read pty: %v", readErr)
	}
	code := waitExitCode(t, cmd)
	return strings.ReplaceAll(string(out), "\r\n", "\n"), stderr.String(), code
}

func scribeHelperCommand(t *testing.T, home string, args []string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestScribeHelperProcess", "--"}, args...)...)
	cmd.Dir = home
	cmd.Env = withoutEnv(os.Environ(), "HOME", "PWD")
	cmd.Env = append(cmd.Env, "GO_WANT_SCRIBE_HELPER_PROCESS=1", "HOME="+home, "PWD="+home, partialFixtureEnv+"=1")
	return cmd
}

func withoutEnv(env []string, keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key] = true
	}
	out := env[:0]
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && blocked[key] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func waitExitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper wait: %v", err)
	}
	return exitErr.ExitCode()
}

func writeNameConflictState(t *testing.T, home string) {
	t.Helper()
	raw := []byte(`{
  "schema_version": 5,
  "installed": {
    "good-skill": {
      "revision": 1,
      "sources": [
        {
          "registry": "acme/team",
          "ref": "v0.9.0"
        }
      ],
      "tools": [
        "claude"
      ],
      "tools_mode": "inherit"
    }
  },
  "migrations": {
    "store-v2-flat-skills": true
  }
}`)
	if err := os.WriteFile(filepath.Join(home, ".scribe", "state.json"), raw, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func setupNameConflictHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatalf("mkdir .scribe: %v", err)
	}
	cfg := `registries:
  - repo: acme/team
    enabled: true
    type: team
adoption:
  mode: off
tools:
  - name: claude
    enabled: true
  - name: codex
    enabled: false
  - name: cursor
    enabled: false
  - name: gemini
    enabled: false
scribe_agent:
  enabled: false
`
	if err := os.WriteFile(filepath.Join(scribeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return home
}
