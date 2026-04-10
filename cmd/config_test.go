package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigSetEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the .scribe directory and an initial config.yaml.
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(scribeDir, "config.yaml")
	initial := map[string]any{"editor": "vim"}
	data, _ := yaml.Marshal(initial)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newConfigCommand()
	cmd.SetArgs([]string{"set", "editor", "cursor"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back and verify.
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := yaml.Unmarshal(got, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg["editor"] != "cursor" {
		t.Errorf("expected editor=cursor, got %v", cfg["editor"])
	}
}

func TestConfigSetEditorNewConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newConfigCommand()
	cmd.SetArgs([]string{"set", "editor", "vim"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfgPath := filepath.Join(home, ".scribe", "config.yaml")
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := yaml.Unmarshal(got, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg["editor"] != "vim" {
		t.Errorf("expected editor=vim, got %v", cfg["editor"])
	}
}
