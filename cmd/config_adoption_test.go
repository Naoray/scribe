package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Naoray/scribe/internal/config"
)

// helpers

func adoptionSetup(t *testing.T) (home string, cfgPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return home, filepath.Join(scribeDir, "config.yaml")
}

func runAdoptionCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := newConfigCommand()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(append([]string{"adoption"}, args...))
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func loadCfg(t *testing.T, cfgPath string) *config.Config {
	t.Helper()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return &cfg
}

// tests

func TestConfigAdoptionBareNoFile(t *testing.T) {
	adoptionSetup(t)

	stdout, _, err := runAdoptionCmd(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "mode:") {
		t.Errorf("expected mode line in output, got: %s", stdout)
	}
}

func TestConfigAdoptionModeRoundTrip(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	// Set off.
	if _, _, err := runAdoptionCmd(t, "--mode", "off"); err != nil {
		t.Fatalf("set off: %v", err)
	}
	cfg := loadCfg(t, cfgPath)
	if cfg.Adoption.Mode != "off" {
		t.Errorf("expected mode=off, got %q", cfg.Adoption.Mode)
	}

	// Set auto.
	if _, _, err := runAdoptionCmd(t, "--mode", "auto"); err != nil {
		t.Fatalf("set auto: %v", err)
	}
	cfg = loadCfg(t, cfgPath)
	if cfg.Adoption.Mode != "auto" {
		t.Errorf("expected mode=auto, got %q", cfg.Adoption.Mode)
	}
}

func TestConfigAdoptionModePrompt(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	if _, _, err := runAdoptionCmd(t, "--mode", "prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	cfg := loadCfg(t, cfgPath)
	if cfg.Adoption.Mode != "prompt" {
		t.Errorf("expected mode=prompt, got %q", cfg.Adoption.Mode)
	}
}

func TestConfigAdoptionInvalidMode(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	// Write initial state so we can verify it's unchanged.
	initial := map[string]any{"adoption": map[string]any{"mode": "off"}}
	data, _ := yaml.Marshal(initial)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runAdoptionCmd(t, "--mode", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should mention invalid value, got: %v", err)
	}

	// File must be unchanged.
	cfg := loadCfg(t, cfgPath)
	if cfg.Adoption.Mode != "off" {
		t.Errorf("config should be unchanged after error, got mode=%q", cfg.Adoption.Mode)
	}
}

func TestConfigAdoptionAddPath(t *testing.T) {
	home, cfgPath := adoptionSetup(t)

	extraPath := "~/extra/skills"
	if _, _, err := runAdoptionCmd(t, "--add-path", extraPath); err != nil {
		t.Fatalf("add-path: %v", err)
	}

	cfg := loadCfg(t, cfgPath)
	if len(cfg.Adoption.Paths) != 1 || cfg.Adoption.Paths[0] != extraPath {
		t.Errorf("expected paths=[%s], got %v", extraPath, cfg.Adoption.Paths)
	}

	// Calling AdoptionPaths should resolve and include it.
	resolved, err := cfg.AdoptionPaths()
	if err != nil {
		t.Fatalf("AdoptionPaths: %v", err)
	}
	// Resolve symlinks on home so the comparison works on macOS (/var → /private/var).
	resolvedHome := home
	if rh, err := filepath.EvalSymlinks(home); err == nil {
		resolvedHome = rh
	}
	expected := filepath.Join(resolvedHome, "extra", "skills")
	found := false
	for _, p := range resolved {
		if p == expected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in resolved paths, got %v", expected, resolved)
	}
}

func TestConfigAdoptionAddPathDuplicate(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	extraPath := "~/extra/skills"
	// Add once.
	if _, _, err := runAdoptionCmd(t, "--add-path", extraPath); err != nil {
		t.Fatalf("first add-path: %v", err)
	}
	// Add again — should no-op.
	stdout, _, err := runAdoptionCmd(t, "--add-path", extraPath)
	if err != nil {
		t.Fatalf("duplicate add-path: %v", err)
	}
	if !strings.Contains(stdout, "already present") {
		t.Errorf("expected 'already present' message, got: %s", stdout)
	}

	cfg := loadCfg(t, cfgPath)
	if len(cfg.Adoption.Paths) != 1 {
		t.Errorf("expected exactly 1 path after duplicate add, got %d", len(cfg.Adoption.Paths))
	}
}

func TestConfigAdoptionRemovePath(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	extraPath := "~/extra/skills"
	if _, _, err := runAdoptionCmd(t, "--add-path", extraPath); err != nil {
		t.Fatalf("add-path: %v", err)
	}
	if _, _, err := runAdoptionCmd(t, "--remove-path", extraPath); err != nil {
		t.Fatalf("remove-path: %v", err)
	}

	cfg := loadCfg(t, cfgPath)
	if len(cfg.Adoption.Paths) != 0 {
		t.Errorf("expected 0 paths after remove, got %v", cfg.Adoption.Paths)
	}
}

func TestConfigAdoptionRemovePathMissing(t *testing.T) {
	adoptionSetup(t)

	_, _, err := runAdoptionCmd(t, "--remove-path", "~/nonexistent/skills")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
	if !strings.Contains(err.Error(), "not in config") {
		t.Errorf("error should mention not in config, got: %v", err)
	}
}

func TestConfigAdoptionPathOutsideHome(t *testing.T) {
	adoptionSetup(t)

	// /tmp is outside the temp home dir.
	_, _, err := runAdoptionCmd(t, "--add-path", "/tmp/outside-home")
	if err == nil {
		t.Fatal("expected error for path outside home, got nil")
	}
	if !strings.Contains(err.Error(), "outside home") {
		t.Errorf("error should mention outside home, got: %v", err)
	}
}

func TestConfigAdoptionRoundTripMultipleMutations(t *testing.T) {
	_, cfgPath := adoptionSetup(t)

	// Set mode.
	if _, _, err := runAdoptionCmd(t, "--mode", "prompt"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	// Add two paths.
	if _, _, err := runAdoptionCmd(t, "--add-path", "~/skills/a"); err != nil {
		t.Fatalf("add path a: %v", err)
	}
	if _, _, err := runAdoptionCmd(t, "--add-path", "~/skills/b"); err != nil {
		t.Fatalf("add path b: %v", err)
	}
	// Remove first path.
	if _, _, err := runAdoptionCmd(t, "--remove-path", "~/skills/a"); err != nil {
		t.Fatalf("remove path a: %v", err)
	}

	cfg := loadCfg(t, cfgPath)
	if cfg.Adoption.Mode != "prompt" {
		t.Errorf("expected mode=prompt, got %q", cfg.Adoption.Mode)
	}
	if len(cfg.Adoption.Paths) != 1 || cfg.Adoption.Paths[0] != "~/skills/b" {
		t.Errorf("expected paths=[~/skills/b], got %v", cfg.Adoption.Paths)
	}
}
