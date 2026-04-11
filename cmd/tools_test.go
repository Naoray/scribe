package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/tools"
)

func TestFormatToolsList(t *testing.T) {
	statuses := []tools.Status{
		{Name: "claude", Type: "builtin", Enabled: true, Detected: true, DetectKnown: true, Source: "auto"},
		{Name: "aider", Type: "custom", Enabled: false, Detected: false, DetectKnown: false, Source: "manual"},
	}

	out := formatToolsList(statuses)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	for _, want := range []string{"claude", "aider", "builtin", "custom", "enabled", "disabled", "auto", "manual"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q, got: %s", want, out)
		}
	}
}

func TestFormatToolsListJSON(t *testing.T) {
	statuses := []tools.Status{
		{Name: "claude", Type: "builtin", Enabled: true, Detected: true, DetectKnown: true, Source: "auto"},
		{Name: "aider", Type: "custom", Enabled: true, Detected: false, DetectKnown: false, Source: "manual"},
	}

	out, err := formatToolsListJSON(statuses)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []tools.Status
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(parsed))
	}
	if parsed[0].Name != "claude" || !parsed[0].Enabled {
		t.Errorf("unexpected first tool: %+v", parsed[0])
	}
	if parsed[1].Name != "aider" || !parsed[1].Enabled {
		t.Errorf("unexpected second tool: %+v", parsed[1])
	}
}

func TestFormatToolsListEmpty(t *testing.T) {
	out := formatToolsList(nil)
	if !strings.Contains(out, "No tools detected or configured") {
		t.Errorf("expected empty-state message, got: %s", out)
	}
}
