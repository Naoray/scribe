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
		Origin      string   `json:"origin,omitempty"`
		Path        string   `json:"path,omitempty"`
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	storeDir := home + "/.scribe/skills"

	t.Run("unmanaged tool-facing skill", func(t *testing.T) {
		// Path outside ~/.scribe/skills/ → Managed=false in discovery.Skill.
		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills := []discovery.Skill{
			{
				Name:        "tool-skill",
				Description: "A tool-facing skill",
				ContentHash: "abc123",
				Targets:     []string{"claude"},
				LocalPath:   home + "/.claude/skills/tool-skill",
				Managed:     false,
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
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if got[0].Managed {
			t.Error("tool-skill: expected managed=false")
		}
		if got[0].Origin != "" {
			t.Errorf("tool-skill: expected no origin, got %q", got[0].Origin)
		}
	})

	t.Run("adopted local-origin skill", func(t *testing.T) {
		// State has Origin=OriginLocal + path inside store → managed=true, origin="local".
		st := &state.State{
			Installed: map[string]state.InstalledSkill{
				"adopted-skill": {Revision: 1, Origin: state.OriginLocal},
			},
		}
		skills := []discovery.Skill{
			{
				Name:        "adopted-skill",
				Description: "An adopted skill",
				ContentHash: "def456",
				Targets:     []string{"claude"},
				LocalPath:   storeDir + "/adopted-skill",
				Managed:     true,
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
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if !got[0].Managed {
			t.Error("adopted-skill: expected managed=true")
		}
		if got[0].Origin != "local" {
			t.Errorf("adopted-skill: expected origin=%q, got %q", "local", got[0].Origin)
		}
	})

	t.Run("registry-sourced skill", func(t *testing.T) {
		// State has empty Origin (OriginRegistry) → managed=true, no origin in JSON.
		st := &state.State{
			Installed: map[string]state.InstalledSkill{
				"registry-skill": {Revision: 3, Origin: state.OriginRegistry},
			},
		}
		skills := []discovery.Skill{
			{
				Name:        "registry-skill",
				Description: "A registry skill",
				Revision:    3,
				ContentHash: "ghi789",
				Targets:     []string{"claude"},
				LocalPath:   storeDir + "/registry-skill",
				Managed:     true,
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
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if !got[0].Managed {
			t.Error("registry-skill: expected managed=true")
		}
		if got[0].Origin != "" {
			t.Errorf("registry-skill: expected no origin (omitempty), got %q", got[0].Origin)
		}
		if got[0].ContentHash != "ghi789" {
			t.Errorf("registry-skill: content_hash = %q, want %q", got[0].ContentHash, "ghi789")
		}
		if got[0].Revision != 3 {
			t.Errorf("registry-skill: revision = %d, want 3", got[0].Revision)
		}
	})

	t.Run("nil targets become empty array", func(t *testing.T) {
		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills := []discovery.Skill{
			{Name: "bare", ContentHash: "x", Targets: nil, Managed: false},
		}
		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON error: %v", err)
		}
		var got []outputSkill
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
		}
		if got[0].Targets == nil {
			t.Error("targets should be [] not null")
		}
		if len(got[0].Targets) != 0 {
			t.Errorf("targets = %v, want empty array", got[0].Targets)
		}
	})
}
