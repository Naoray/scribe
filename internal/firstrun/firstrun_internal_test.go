package firstrun

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestApplyBuiltins_AlreadyCurrentIsNoop(t *testing.T) {
	cfg := &config.Config{BuiltinsVersion: parsedDefaults.Version}
	added, firstRun := ApplyBuiltins(cfg)

	if len(added) != 0 {
		t.Errorf("no-op expected, got %v", added)
	}
	if firstRun {
		t.Error("already-current config should not report firstRun")
	}
	if len(cfg.Registries) != 0 {
		t.Errorf("no registries should be appended when already current; got %v", cfg.Registries)
	}
}
