package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

type hubStatus struct {
	Version        string   `json:"version"`
	Registries     []string `json:"registries"`
	InstalledCount int      `json:"installed_count"`
	LastSync       string   `json:"last_sync,omitempty"`
}

func TestHubJSONOutput(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true},
			{Repo: "disabled/repo", Enabled: false},
		},
	}
	st := &state.State{
		LastSync: time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
		Installed: map[string]state.InstalledSkill{
			"deploy": {},
			"lint":   {},
		},
	}

	var buf bytes.Buffer
	err := writeHubJSON(&buf, "1.0.0", cfg, st)
	if err != nil {
		t.Fatalf("writeHubJSON error: %v", err)
	}

	var got hubStatus
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if got.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", got.Version, "1.0.0")
	}
	if len(got.Registries) != 1 {
		t.Errorf("registries: got %d, want 1 (only enabled)", len(got.Registries))
	}
	if got.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("registry: got %q, want %q", got.Registries[0], "ArtistfyHQ/team-skills")
	}
	if got.InstalledCount != 2 {
		t.Errorf("installed_count: got %d, want 2", got.InstalledCount)
	}
	if got.LastSync != "2026-04-06T10:00:00Z" {
		t.Errorf("last_sync: got %q, want %q", got.LastSync, "2026-04-06T10:00:00Z")
	}
}

func TestHubJSONNoState(t *testing.T) {
	cfg := &config.Config{}
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := writeHubJSON(&buf, "dev", cfg, st)
	if err != nil {
		t.Fatalf("writeHubJSON error: %v", err)
	}

	var got hubStatus
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got.Version != "dev" {
		t.Errorf("version: got %q, want %q", got.Version, "dev")
	}
	if len(got.Registries) != 0 {
		t.Errorf("registries: got %d, want 0", len(got.Registries))
	}
	if got.InstalledCount != 0 {
		t.Errorf("installed_count: got %d, want 0", got.InstalledCount)
	}
}
