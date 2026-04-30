package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
