package cmd

import (
	"encoding/json"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestFormatToolsList(t *testing.T) {
	tools := []config.ToolConfig{
		{Name: "claude", Enabled: true},
		{Name: "cursor", Enabled: false},
	}

	out := formatToolsList(tools)

	if out == "" {
		t.Fatal("expected non-empty output")
	}
	// Should contain tool names.
	if !containsStr(out, "claude") {
		t.Errorf("output should contain 'claude', got: %s", out)
	}
	if !containsStr(out, "cursor") {
		t.Errorf("output should contain 'cursor', got: %s", out)
	}
	// Should contain status indicators.
	if !containsStr(out, "enabled") {
		t.Errorf("output should contain 'enabled', got: %s", out)
	}
	if !containsStr(out, "disabled") {
		t.Errorf("output should contain 'disabled', got: %s", out)
	}
}

func TestFormatToolsListJSON(t *testing.T) {
	tools := []config.ToolConfig{
		{Name: "claude", Enabled: true},
		{Name: "cursor", Enabled: false},
	}

	out, err := formatToolsListJSON(tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON.
	var parsed []config.ToolConfig
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(parsed))
	}
	if parsed[0].Name != "claude" || !parsed[0].Enabled {
		t.Errorf("unexpected first tool: %+v", parsed[0])
	}
	if parsed[1].Name != "cursor" || parsed[1].Enabled {
		t.Errorf("unexpected second tool: %+v", parsed[1])
	}
}

func TestFormatToolsListEmpty(t *testing.T) {
	out := formatToolsList(nil)
	if !containsStr(out, "No tools detected") {
		t.Errorf("expected 'No tools detected' message, got: %s", out)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
