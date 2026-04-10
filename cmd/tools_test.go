package cmd

import (
	"encoding/json"
	"strings"
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
	if !strings.Contains(out, "claude") {
		t.Errorf("output should contain 'claude', got: %s", out)
	}
	if !strings.Contains(out, "cursor") {
		t.Errorf("output should contain 'cursor', got: %s", out)
	}
	// Should contain status indicators.
	if !strings.Contains(out, "enabled") {
		t.Errorf("output should contain 'enabled', got: %s", out)
	}
	if !strings.Contains(out, "disabled") {
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
	if !strings.Contains(out, "No tools detected") {
		t.Errorf("expected 'No tools detected' message, got: %s", out)
	}
}

