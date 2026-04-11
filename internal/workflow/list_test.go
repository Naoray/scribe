package workflow

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
)

func TestListLoadSteps_Composition(t *testing.T) {
	steps := ListLoadSteps()
	if len(steps) != 3 {
		t.Fatalf("ListLoadSteps() = %d steps, want 3", len(steps))
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("step[0] = %s, want LoadConfig", steps[0].Name)
	}
	if steps[1].Name != "LoadState" {
		t.Errorf("step[1] = %s, want LoadState", steps[1].Name)
	}
	if steps[2].Name != "ResolveTools" {
		t.Errorf("step[2] = %s, want ResolveTools", steps[2].Name)
	}
}

func TestListJSONSteps_Composition(t *testing.T) {
	steps := ListJSONSteps()
	if len(steps) == 0 {
		t.Fatal("ListJSONSteps() returned empty list")
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("first step = %s, want LoadConfig", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "WriteListJSON" {
		t.Errorf("last step = %s, want WriteListJSON", steps[len(steps)-1].Name)
	}
}

func TestPrintLocalJSON(t *testing.T) {
	type outputSkill struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Revision    int      `json:"revision,omitempty"`
		ContentHash string   `json:"content_hash,omitempty"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"managed-skill": {Revision: 3},
		},
	}

	skills := []discovery.Skill{
		{
			Name:        "managed-skill",
			Description: "A managed skill",
			Revision:    3,
			ContentHash: "abc123",
			Targets:     []string{"claude"},
			LocalPath:   "/home/user/.claude/skills/managed-skill",
		},
		{
			Name:        "unmanaged-skill",
			Description: "An unmanaged skill",
			ContentHash: "def456",
			Targets:     nil, // should become [] in JSON
			LocalPath:   "/home/user/.claude/skills/unmanaged-skill",
		},
	}

	var buf bytes.Buffer
	if err := printLocalJSON(&buf, skills, st); err != nil {
		t.Fatalf("printLocalJSON error: %v", err)
	}

	var got []outputSkill
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(got))
	}

	// Managed skill
	if !got[0].Managed {
		t.Error("managed-skill: expected managed=true")
	}
	if got[0].ContentHash != "abc123" {
		t.Errorf("managed-skill: content_hash = %q, want %q", got[0].ContentHash, "abc123")
	}
	if got[0].Revision != 3 {
		t.Errorf("managed-skill: revision = %d, want 3", got[0].Revision)
	}

	// Unmanaged skill
	if got[1].Managed {
		t.Error("unmanaged-skill: expected managed=false")
	}
	if got[1].ContentHash != "def456" {
		t.Errorf("unmanaged-skill: content_hash = %q, want %q", got[1].ContentHash, "def456")
	}

	// Targets should be [] not null for nil input
	if got[1].Targets == nil {
		t.Error("unmanaged-skill: targets should be [] not null")
	}
	if len(got[1].Targets) != 0 {
		t.Errorf("unmanaged-skill: targets = %v, want empty array", got[1].Targets)
	}
}
