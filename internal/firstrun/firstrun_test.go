package firstrun_test

import (
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/firstrun"
	"github.com/Naoray/scribe/internal/state"
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
	_, _ = firstrun.ApplyBuiltins(cfg)

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
	_, _ = firstrun.ApplyBuiltins(cfg)
	count := len(cfg.Registries)

	// Apply again -- should not duplicate.
	_, _ = firstrun.ApplyBuiltins(cfg)
	if len(cfg.Registries) != count {
		t.Errorf("expected %d registries after second apply, got %d", count, len(cfg.Registries))
	}
}

func TestApplyBuiltins_FirstRunAddsAllAndMarksVersion(t *testing.T) {
	cfg := &config.Config{}
	added, firstRun := firstrun.ApplyBuiltins(cfg)

	if !firstRun {
		t.Error("first run should report firstRun=true")
	}
	if len(added) != 3 {
		t.Errorf("first run should add 3 builtins, got %d: %v", len(added), added)
	}
	if cfg.FindRegistry("Naoray/scribe") != nil {
		t.Error("Naoray/scribe must not be added as a builtin (scribe-agent is binary-managed)")
	}
	if cfg.FindRegistry("mattpocock/skills") == nil {
		t.Error("mattpocock/skills should be a builtin")
	}
	if cfg.BuiltinsVersion == 0 {
		t.Error("BuiltinsVersion must be set after first ApplyBuiltins call")
	}
}

func TestApplyBuiltins_ExistingUserV3GainsMattpocock(t *testing.T) {
	// A user on v3 has Naoray/scribe + anthropics/skills + expo/skills.
	// ApplyBuiltins should add mattpocock/skills (the new v5 entry).
	// Naoray/scribe stays in config until RemoveNaorayScribeRegistry runs separately.
	cfg := &config.Config{
		BuiltinsVersion: 3,
		Registries: []config.RegistryConfig{
			{Repo: "Naoray/scribe", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "anthropics/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "expo/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
		},
	}
	added, firstRun := firstrun.ApplyBuiltins(cfg)

	if len(added) != 1 || added[0] != "mattpocock/skills" {
		t.Errorf("only mattpocock/skills should be backfilled, got %v", added)
	}
	if firstRun {
		t.Error("existing user should report firstRun=false")
	}
	if cfg.FindRegistry("mattpocock/skills") == nil {
		t.Error("mattpocock/skills not in config after backfill")
	}
}

func TestApplyBuiltins_ManuallyConnectedNotDuplicated(t *testing.T) {
	// User already ran `scribe registry connect mattpocock/skills` before it became a builtin.
	cfg := &config.Config{
		BuiltinsVersion: 3, // pre-mattpocock version
		Registries: []config.RegistryConfig{
			{Repo: "Naoray/scribe", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "anthropics/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "expo/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: true},
			{Repo: "mattpocock/skills", Enabled: true, Type: config.RegistryTypeCommunity, Builtin: false},
		},
	}
	added, _ := firstrun.ApplyBuiltins(cfg)

	if len(added) != 0 {
		t.Errorf("expected nothing added (already connected manually), got %v", added)
	}

	count := 0
	for _, r := range cfg.Registries {
		if strings.EqualFold(r.Repo, "mattpocock/skills") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("mattpocock/skills appears %d times in config, want exactly 1", count)
	}
}

func TestApplyBuiltins_DisabledBuiltinNotReEnabled(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "anthropics/skills", Enabled: false, Type: config.RegistryTypeCommunity, Builtin: true},
		},
	}
	_, _ = firstrun.ApplyBuiltins(cfg)

	r := cfg.FindRegistry("anthropics/skills")
	if r == nil {
		t.Fatal("anthropics/skills should still be present")
	}
	if r.Enabled {
		t.Error("disabled builtin must not be flipped back to enabled")
	}
}

func TestApplyBuiltinsRemove_RemovesOpenAICodexOnce(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "openai/codex-skills", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
			{Repo: "anthropics/skills", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}, Migrations: map[string]bool{}}

	removed, ran := firstrun.ApplyBuiltinsRemove(cfg, st, []string{"openai/codex-skills"})
	if len(removed) != 1 || removed[0] != "openai/codex-skills" {
		t.Fatalf("removed = %v, want [openai/codex-skills]", removed)
	}
	if !ran {
		t.Fatal("ran = false, want true")
	}
	if cfg.FindRegistry("openai/codex-skills") != nil {
		t.Fatal("openai/codex-skills should have been removed from config")
	}

	removed, ran = firstrun.ApplyBuiltinsRemove(cfg, st, []string{"openai/codex-skills"})
	if len(removed) != 0 {
		t.Fatalf("second removal should be a no-op, got %v", removed)
	}
	if ran {
		t.Fatal("second removal ran = true, want false")
	}
}

func TestApplyBuiltinsRename_ReplacesAnthropicSkillsOnce(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "anthropic/skills", Enabled: false, Builtin: true, Type: config.RegistryTypeCommunity},
			{Repo: "expo/skills", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}, Migrations: map[string]bool{}}

	renamed, ran := firstrun.ApplyBuiltinsRename(cfg, st, map[string]string{"anthropic/skills": "anthropics/skills"})
	if len(renamed) != 1 || renamed[0] != "anthropic/skills -> anthropics/skills" {
		t.Fatalf("renamed = %v, want [anthropic/skills -> anthropics/skills]", renamed)
	}
	if !ran {
		t.Fatal("ran = false, want true")
	}
	if cfg.FindRegistry("anthropic/skills") != nil {
		t.Fatal("anthropic/skills should have been removed from config")
	}
	replacement := cfg.FindRegistry("anthropics/skills")
	if replacement == nil {
		t.Fatal("anthropics/skills should have been added to config")
	}
	if replacement.Enabled {
		t.Fatal("replacement should preserve the disabled state from anthropic/skills")
	}

	renamed, ran = firstrun.ApplyBuiltinsRename(cfg, st, map[string]string{"anthropic/skills": "anthropics/skills"})
	if len(renamed) != 0 {
		t.Fatalf("second rename should be a no-op, got %v", renamed)
	}
	if ran {
		t.Fatal("second rename ran = true, want false")
	}
}

func TestRemoveNaorayScribeRegistry_RemovesBuiltinEntry(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "Naoray/scribe", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
			{Repo: "anthropics/skills", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}, Migrations: map[string]bool{}}

	pruned, ran := firstrun.RemoveNaorayScribeRegistry(cfg, st)
	if len(pruned) != 1 || pruned[0] != "Naoray/scribe" {
		t.Fatalf("pruned = %v, want [Naoray/scribe]", pruned)
	}
	if !ran {
		t.Fatal("ran = false, want true")
	}
	if cfg.FindRegistry("Naoray/scribe") != nil {
		t.Fatal("Naoray/scribe should be removed from config")
	}
	if cfg.FindRegistry("anthropics/skills") == nil {
		t.Fatal("anthropics/skills should remain")
	}
}

func TestRemoveNaorayScribeRegistry_IdempotentOnSecondCall(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "Naoray/scribe", Enabled: true, Builtin: true, Type: config.RegistryTypeCommunity},
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}, Migrations: map[string]bool{}}

	firstrun.RemoveNaorayScribeRegistry(cfg, st)

	pruned, ran := firstrun.RemoveNaorayScribeRegistry(cfg, st)
	if len(pruned) != 0 {
		t.Fatalf("second call should be a no-op, got %v", pruned)
	}
	if ran {
		t.Fatal("second call ran = true, want false")
	}
}

func TestRemoveNaorayScribeRegistry_KeepsUserAddedEntry(t *testing.T) {
	// A user who manually added Naoray/scribe (Builtin: false) should not have it removed.
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "Naoray/scribe", Enabled: true, Builtin: false, Type: config.RegistryTypeCommunity},
		},
	}
	st := &state.State{Installed: map[string]state.InstalledSkill{}, Migrations: map[string]bool{}}

	pruned, ran := firstrun.RemoveNaorayScribeRegistry(cfg, st)
	if len(pruned) != 0 {
		t.Fatalf("user-added registry should not be removed, got %v", pruned)
	}
	if !ran {
		t.Fatal("migration should still be marked as run even when nothing was pruned")
	}
	if cfg.FindRegistry("Naoray/scribe") == nil {
		t.Fatal("user-added Naoray/scribe should remain in config")
	}
}
