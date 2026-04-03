package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestRunGuideJSON_UnauthenticatedNoConnections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "") // prevent gh CLI from being found

	var buf bytes.Buffer
	if err := runGuideJSON(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if got := out["status"]; got != "not_connected" {
		t.Errorf("status = %q, want not_connected", got)
	}

	steps := stepsFrom(t, out)
	commands := commandsFrom(steps)

	if !contains(commands, "gh auth login") {
		t.Error("expected gh auth login step when unauthenticated")
	}
	if !contains(commands, "scribe connect <owner/repo>") {
		t.Error("expected scribe connect step when no connections")
	}
}

func TestRunGuideJSON_AuthenticatedNoConnections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("PATH", "")

	var buf bytes.Buffer
	if err := runGuideJSON(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got := out["status"]; got != "not_connected" {
		t.Errorf("status = %q, want not_connected", got)
	}

	steps := stepsFrom(t, out)
	commands := commandsFrom(steps)

	if contains(commands, "gh auth login") {
		t.Error("gh auth login step should be absent when authenticated")
	}
	if !contains(commands, "scribe connect <owner/repo>") {
		t.Error("expected scribe connect step when no connections")
	}
	if !contains(commands, "scribe sync") {
		t.Error("expected scribe sync step")
	}
}

func TestRunGuideJSON_FullyConnected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	configDir := home + "/.scribe"
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configDir+"/config.toml", []byte("team_repos = [\"Org/repo\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runGuideJSON(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got := out["status"]; got != "connected" {
		t.Errorf("status = %q, want connected", got)
	}

	steps := stepsFrom(t, out)
	commands := commandsFrom(steps)

	if contains(commands, "gh auth login") {
		t.Error("gh auth login step should be absent when authenticated")
	}
	if contains(commands, "scribe connect <owner/repo>") {
		t.Error("scribe connect step should be absent when already connected")
	}
	if !contains(commands, "scribe sync") {
		t.Error("expected scribe sync step")
	}
	if !contains(commands, "scribe list") {
		t.Error("expected scribe list step")
	}
}

func TestRunGuideJSON_OutputSchema(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "")

	var buf bytes.Buffer
	if err := runGuideJSON(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"status", "prerequisites", "steps"} {
		if _, ok := out[key]; !ok {
			t.Errorf("missing top-level key %q in JSON output", key)
		}
	}
}

// stepsFrom extracts the steps array from the decoded JSON output.
func stepsFrom(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["steps"]
	if !ok {
		t.Fatal("missing steps field")
	}
	slice, ok := raw.([]any)
	if !ok {
		t.Fatalf("steps is not an array, got %T", raw)
	}
	var steps []map[string]any
	for _, s := range slice {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("step is not an object, got %T", s)
		}
		steps = append(steps, m)
	}
	return steps
}

// commandsFrom extracts command strings from the steps list.
func commandsFrom(steps []map[string]any) []string {
	var cmds []string
	for _, s := range steps {
		if c, ok := s["command"].(string); ok {
			cmds = append(cmds, c)
		}
	}
	return cmds
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
