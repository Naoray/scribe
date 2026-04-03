package workflow

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
)

func TestListSteps_Composition(t *testing.T) {
	steps := ListSteps()
	if len(steps) == 0 {
		t.Fatal("ListSteps() returned empty list")
	}

	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "BranchLocalOrRemote" {
		t.Errorf("expected last step BranchLocalOrRemote, got %s", steps[len(steps)-1].Name)
	}
}

func TestPrintLocalJSON(t *testing.T) {
	type outputSkill struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Version     string   `json:"version"`
		ContentHash string   `json:"content_hash,omitempty"`
		Source      string   `json:"source"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"managed-skill": {Version: "v1.0"},
		},
	}

	skills := []discovery.Skill{
		{
			Name:        "managed-skill",
			Description: "A managed skill",
			Version:     "v1.0",
			ContentHash: "abc123",
			Source:      "github.com/example/repo",
			Targets:     []string{"claude"},
			LocalPath:   "/home/user/.claude/skills/managed-skill",
		},
		{
			Name:        "unmanaged-skill",
			Description: "An unmanaged skill",
			Version:     "v2.0",
			ContentHash: "def456",
			Source:      "",
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
	if got[0].Version != "v1.0" {
		t.Errorf("managed-skill: version = %q, want %q", got[0].Version, "v1.0")
	}

	// Unmanaged skill
	if got[1].Managed {
		t.Error("unmanaged-skill: expected managed=false")
	}
	if got[1].ContentHash != "def456" {
		t.Errorf("unmanaged-skill: content_hash = %q, want %q", got[1].ContentHash, "def456")
	}
	if got[1].Version != "v2.0" {
		t.Errorf("unmanaged-skill: version = %q, want %q", got[1].Version, "v2.0")
	}

	// Targets should be [] not null for nil input
	if got[1].Targets == nil {
		t.Error("unmanaged-skill: targets should be [] not null")
	}
	if len(got[1].Targets) != 0 {
		t.Errorf("unmanaged-skill: targets = %v, want empty array", got[1].Targets)
	}
}
