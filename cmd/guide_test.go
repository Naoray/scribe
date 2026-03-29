package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestGuideJSON_NotConnected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	t.Setenv("PATH", os.Getenv("PATH"))

	var buf bytes.Buffer
	err := runGuideJSON(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "not_connected" {
		t.Errorf("expected status not_connected, got %v", result["status"])
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Error("expected non-empty steps array")
	}
}

func TestGuideJSON_AlreadyConnected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	configDir := home + "/.scribe"
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(configDir+"/config.toml", []byte("team_repos = [\"Org/repo\"]\n"), 0o644)

	var buf bytes.Buffer
	err := runGuideJSON(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "connected" {
		t.Errorf("expected status connected, got %v", result["status"])
	}
}
