package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

func withFakeScribeBinary(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	path := filepath.Join(binDir, "scribe")
	script := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(path, script, 0o755); err != nil {
		t.Fatalf("write fake scribe: %v", err)
	}
	t.Setenv("PATH", binDir)
	return path
}

func TestEnsureScribeAgentInstallsWhenMissing(t *testing.T) {
	withFakeScribeBinary(t)

	store := filepath.Join(t.TempDir(), "skills")
	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureScribeAgent() changed = false, want true")
	}

	got, err := os.ReadFile(filepath.Join(store, "scribe-agent", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(got), "===setup-start===") {
		t.Fatal("bootstrap section missing from rendered skill")
	}
	if strings.Contains(string(got), "## Keep `scribe` current") {
		t.Fatal("daily upgrade prompt should not appear during bootstrap")
	}
	if st.Installed["scribe-agent"].Origin != state.OriginBootstrap {
		t.Fatalf("origin = %q, want %q", st.Installed["scribe-agent"].Origin, state.OriginBootstrap)
	}
}

func TestEnsureScribeAgentNoOpWhenPresent(t *testing.T) {
	withFakeScribeBinary(t)

	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	steadyState, err := renderScribeAgentMarkdown(scribeAgentRenderContext{
		HasScribeBinary:         true,
		HasScribeAgentInstalled: true,
		NeedsBootstrap:          false,
		ShowDailyUpgradePrompt:  true,
	})
	if err != nil {
		t.Fatalf("render steady state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), steadyState, 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), steadyState, 0o644); err != nil {
		t.Fatalf("write .scribe-base.md: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"scribe-agent": {
			Revision: 1,
			Origin:   state.OriginBootstrap,
			Sources:  []state.SkillSource{{Ref: EmbeddedVersion}},
		},
	}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureScribeAgent() changed = true, want false")
	}
}

func TestEnsureScribeAgentRepairsMissingBaseFile(t *testing.T) {
	withFakeScribeBinary(t)

	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	steadyState, err := renderScribeAgentMarkdown(scribeAgentRenderContext{
		HasScribeBinary:         true,
		HasScribeAgentInstalled: true,
		NeedsBootstrap:          false,
		ShowDailyUpgradePrompt:  true,
	})
	if err != nil {
		t.Fatalf("render steady state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), steadyState, 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"scribe-agent": {
			Revision: 1,
			Origin:   state.OriginBootstrap,
			Sources:  []state.SkillSource{{Ref: EmbeddedVersion}},
		},
	}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureScribeAgent() changed = false, want true when .scribe-base.md is missing")
	}
	if st.Installed["scribe-agent"].Revision != 1 {
		t.Fatalf("revision = %d, want 1 for same-version repair", st.Installed["scribe-agent"].Revision)
	}

	baseContent, err := os.ReadFile(filepath.Join(skillDir, ".scribe-base.md"))
	if err != nil {
		t.Fatalf("read .scribe-base.md: %v", err)
	}
	if !strings.Contains(string(baseContent), "## Keep `scribe` current") {
		t.Fatal(".scribe-base.md was not repaired with the rendered steady-state content")
	}
}

func TestEnsureScribeAgentRepairsStaleBaseFile(t *testing.T) {
	withFakeScribeBinary(t)

	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	steadyState, err := renderScribeAgentMarkdown(scribeAgentRenderContext{
		HasScribeBinary:         true,
		HasScribeAgentInstalled: true,
		NeedsBootstrap:          false,
		ShowDailyUpgradePrompt:  true,
	})
	if err != nil {
		t.Fatalf("render steady state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), steadyState, 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write .scribe-base.md: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"scribe-agent": {
			Revision: 1,
			Origin:   state.OriginBootstrap,
			Sources:  []state.SkillSource{{Ref: EmbeddedVersion}},
		},
	}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureScribeAgent() changed = false, want true when .scribe-base.md is stale")
	}
	if st.Installed["scribe-agent"].Revision != 1 {
		t.Fatalf("revision = %d, want 1 for same-version repair", st.Installed["scribe-agent"].Revision)
	}

	baseContent, err := os.ReadFile(filepath.Join(skillDir, ".scribe-base.md"))
	if err != nil {
		t.Fatalf("read .scribe-base.md: %v", err)
	}
	if !strings.Contains(string(baseContent), "## Keep `scribe` current") {
		t.Fatal(".scribe-base.md was not refreshed with the rendered steady-state content")
	}
}

func TestEnsureScribeAgentReinstallsOnVersionMismatch(t *testing.T) {
	withFakeScribeBinary(t)

	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write .scribe-base.md: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{
		"scribe-agent": {Revision: 2, Origin: state.OriginBootstrap},
	}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureScribeAgent() changed = false, want true")
	}
	if st.Installed["scribe-agent"].Revision != 3 {
		t.Fatalf("revision = %d, want 3", st.Installed["scribe-agent"].Revision)
	}

	got, err := os.ReadFile(filepath.Join(store, "scribe-agent", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(got), "## Keep `scribe` current") {
		t.Fatal("steady-state upgrade prompt missing from rendered skill")
	}
	if strings.Contains(string(got), "===setup-start===") {
		t.Fatal("bootstrap section should not be present in steady state")
	}
}

func TestEnsureScribeAgentRespectsOptOut(t *testing.T) {
	store := filepath.Join(t.TempDir(), "skills")
	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = false

	changed, err := EnsureScribeAgent(store, st, cfg)
	if err != nil {
		t.Fatalf("EnsureScribeAgent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureScribeAgent() changed = true, want false")
	}
}

func TestEnsureScribeAgentSurvivesReadOnlyStore(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "skills")
	if err := os.WriteFile(blocker, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	cfg := &config.Config{}
	cfg.ScribeAgent.Enabled = true

	if _, err := EnsureScribeAgent(blocker, st, cfg); err == nil {
		t.Fatal("EnsureScribeAgent() error = nil, want error")
	}
}

func TestRenderScribeAgentBootstrapWhenBinaryMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(&state.State{}, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if !strings.Contains(string(rendered), "===setup-start===") {
		t.Fatal("bootstrap section missing when binary is absent")
	}
	if strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("daily prompt should not render during bootstrap")
	}
}

func TestRenderScribeAgentSteadyStateWhenInstalled(t *testing.T) {
	withFakeScribeBinary(t)

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(&state.State{
		Installed: map[string]state.InstalledSkill{
			"scribe-agent": {},
		},
	}, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if strings.Contains(string(rendered), "===setup-start===") {
		t.Fatal("bootstrap section should not render in steady state")
	}
	if !strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("steady-state prompt missing")
	}
}

func TestRenderScribeAgentBootstrapWhenSkillMissing(t *testing.T) {
	withFakeScribeBinary(t)

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(&state.State{}, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if !strings.Contains(string(rendered), "===setup-start===") {
		t.Fatal("bootstrap section missing when skill is absent")
	}
	if strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("steady-state prompt should not render during bootstrap")
	}
}

func TestRenderScribeAgentOmitUpgradePromptWhenCooldownFresh(t *testing.T) {
	withFakeScribeBinary(t)

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"scribe-agent": {},
		},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	st.RecordScribeBinaryUpdateSuccessAt(now.Add(-23 * time.Hour))

	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(st, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("steady-state prompt should be omitted while cooldown is fresh")
	}
}

func TestRenderScribeAgentShowUpgradePromptWhenCooldownExpired(t *testing.T) {
	withFakeScribeBinary(t)

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"scribe-agent": {},
		},
		BinaryUpdateChecks: map[string]state.BinaryUpdateCheck{},
	}
	st.RecordScribeBinaryUpdateSuccessAt(now.Add(-24 * time.Hour))

	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(st, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if !strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("steady-state prompt should render when cooldown expires")
	}
}

func TestRenderScribeAgentShowUpgradePromptWhenCooldownMissing(t *testing.T) {
	withFakeScribeBinary(t)

	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	rendered, err := renderScribeAgentMarkdown(buildScribeAgentRenderContextAt(&state.State{
		Installed: map[string]state.InstalledSkill{
			"scribe-agent": {},
		},
	}, now))
	if err != nil {
		t.Fatalf("renderScribeAgentMarkdown() error = %v", err)
	}
	if !strings.Contains(string(rendered), "## Keep `scribe` current") {
		t.Fatal("steady-state prompt should render when no cooldown entry exists")
	}
}

func TestInstallScribeAgentValidatesFrontmatter(t *testing.T) {
	store := filepath.Join(t.TempDir(), "skills")
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	if _, err := InstallScribeAgent(store, st, []byte("---\nname: wrong\n---\n"), "v1.2.3"); err == nil {
		t.Fatal("InstallScribeAgent() error = nil, want validation error")
	}
}
