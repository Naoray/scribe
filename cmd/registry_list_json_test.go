package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryListJSONEnvelopeIncludesVisibility(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `registries:
  - repo: acme/public
    enabled: true
    type: community
    visibility: public
  - repo: acme/private
    enabled: true
    type: team
    visibility: private
adoption:
  mode: off
scribe_agent:
  enabled: false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"registry", "list", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}

	var env struct {
		Data struct {
			Registries []struct {
				Registry   string `json:"registry"`
				Visibility string `json:"visibility"`
			} `json:"registries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s", err, stdout)
	}
	got := map[string]string{}
	for _, registry := range env.Data.Registries {
		got[registry.Registry] = registry.Visibility
	}
	if got["acme/public"] != "public" {
		t.Errorf("acme/public visibility = %q, want public", got["acme/public"])
	}
	if got["acme/private"] != "private" {
		t.Errorf("acme/private visibility = %q, want private", got["acme/private"])
	}
}
