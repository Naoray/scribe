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
