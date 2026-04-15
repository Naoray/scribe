package firstrun_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
)

func TestBuiltinRegistries(t *testing.T) {
	registries := firstrun.BuiltinRegistries()
	if len(registries) == 0 {
		t.Fatal("expected at least one built-in registry")
	}

	for _, r := range registries {
		if r.Repo == "" {
			t.Error("builtin registry has empty repo")
		}
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
		if !r.Enabled {
			t.Errorf("%s: expected Enabled=true", r.Repo)
		}
	}
}

func TestIsFirstRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if !firstrun.IsFirstRun() {
		t.Error("expected first run when no config exists")
	}
}

func TestApplyBuiltins(t *testing.T) {
	cfg := &config.Config{}
	firstrun.ApplyBuiltins(cfg)

	if len(cfg.Registries) == 0 {
		t.Fatal("expected registries to be populated")
	}

	for _, r := range cfg.Registries {
		if !r.Builtin {
			t.Errorf("%s: expected Builtin=true", r.Repo)
		}
		if !r.Enabled {
			t.Errorf("%s: expected enabled", r.Repo)
		}
	}
}

func TestApplyBuiltinsIdempotent(t *testing.T) {
	cfg := &config.Config{}
	firstrun.ApplyBuiltins(cfg)
	count := len(cfg.Registries)

	// Apply again -- should not duplicate.
	firstrun.ApplyBuiltins(cfg)
	if len(cfg.Registries) != count {
		t.Errorf("expected %d registries after second apply, got %d", count, len(cfg.Registries))
	}
}

func TestApplyBuiltins_FirstRunAddsAllAndMarksVersion(t *testing.T) {
	cfg := &config.Config{}
	added := firstrun.ApplyBuiltins(cfg)

	if len(added) != 4 {
		t.Errorf("first run should add 4 builtins, got %d: %v", len(added), added)
	}
	if added[0] != "Naoray/scribe" {
		t.Errorf("Naoray/scribe must be first in builtin order, got %q", added[0])
	}
	if cfg.BuiltinsVersion == 0 {
		t.Error("BuiltinsVersion must be set after first ApplyBuiltins call")
	}
}

func TestApplyBuiltins_ExistingUserGetsNaorayScribeBackfilled(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "anthropic/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "openai/codex-skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "expo/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
		},
	}
	added := firstrun.ApplyBuiltins(cfg)

	if len(added) != 1 || added[0] != "Naoray/scribe" {
		t.Errorf("only Naoray/scribe should be backfilled, got %v", added)
	}
	if cfg.FindRegistry("Naoray/scribe") == nil {
		t.Error("Naoray/scribe not in config after backfill")
	}
}

func TestApplyBuiltins_DisabledBuiltinNotReEnabled(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "anthropic/skills", Enabled: false, Type: config.RegistryTypeCommunity, Builtin: true},
		},
	}
	_ = firstrun.ApplyBuiltins(cfg)

	r := cfg.FindRegistry("anthropic/skills")
	if r == nil {
		t.Fatal("anthropic/skills should still be present")
	}
	if r.Enabled {
		t.Error("disabled builtin must not be flipped back to enabled")
	}
}
