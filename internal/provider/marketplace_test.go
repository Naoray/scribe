package provider_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/provider"
)

func TestParseMarketplace(t *testing.T) {
	raw := []byte(`{
		"name": "acme-plugins",
		"plugins": [
			{
				"name": "deploy-tools",
				"source": "./plugins/deploy-tools",
				"skills": ["skills/deploy", "skills/rollback"]
			},
			{
				"name": "testing",
				"source": "./plugins/testing",
				"skills": ["skills/unit-test"]
			}
		]
	}`)

	entries, err := provider.ParseMarketplace(raw, "acme", "plugins-repo")
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	// First plugin's skills.
	if entries[0].Name != "deploy" {
		t.Errorf("entry[0].Name: got %q, want deploy", entries[0].Name)
	}
	if entries[0].Group != "deploy-tools" {
		t.Errorf("entry[0].Group: got %q, want deploy-tools", entries[0].Group)
	}
	if entries[0].Source != "github:acme/plugins-repo@HEAD" {
		t.Errorf("entry[0].Source: got %q", entries[0].Source)
	}
	if entries[0].Path != "plugins/deploy-tools/skills/deploy" {
		t.Errorf("entry[0].Path: got %q", entries[0].Path)
	}

	if entries[1].Name != "rollback" {
		t.Errorf("entry[1].Name: got %q", entries[1].Name)
	}

	if entries[2].Name != "unit-test" {
		t.Errorf("entry[2].Name: got %q", entries[2].Name)
	}
	if entries[2].Group != "testing" {
		t.Errorf("entry[2].Group: got %q", entries[2].Group)
	}
}

func TestParseMarketplaceEmptyPlugins(t *testing.T) {
	raw := []byte(`{"name": "empty", "plugins": []}`)
	entries, err := provider.ParseMarketplace(raw, "acme", "repo")
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseMarketplaceInvalidJSON(t *testing.T) {
	_, err := provider.ParseMarketplace([]byte("{invalid"), "acme", "repo")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
