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
	PendingUpdates int      `json:"pending_updates"`
	StaleStatus    bool     `json:"stale_status"`
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
	if !got.StaleStatus {
		t.Error("stale_status: got false, want true")
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

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"minutes", 5 * time.Minute, "5 minutes ago"},
		{"one hour", 1 * time.Hour, "1 hour ago"},
		{"hours", 3 * time.Hour, "3 hours ago"},
		{"one day", 25 * time.Hour, "1 day ago"},
		{"days", 72 * time.Hour, "3 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Now().Add(-tt.ago)
			got := formatRelativeTime(ts)
			if got != tt.want {
				t.Errorf("formatRelativeTime(%v ago): got %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}

func TestFormatRelativeTimeZero(t *testing.T) {
	got := formatRelativeTime(time.Time{})
	if got != "never" {
		t.Errorf("formatRelativeTime(zero): got %q, want %q", got, "never")
	}
}
