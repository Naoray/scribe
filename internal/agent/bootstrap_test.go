package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

func TestEnsureScribeAgentInstallsWhenMissing(t *testing.T) {
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
	if !bytes.Equal(got, EmbeddedSkillMD) {
		t.Fatal("embedded skill bytes were not written")
	}
	if st.Installed["scribe-agent"].Origin != state.OriginBootstrap {
		t.Fatalf("origin = %q, want %q", st.Installed["scribe-agent"].Origin, state.OriginBootstrap)
	}
}

func TestEnsureScribeAgentNoOpWhenPresent(t *testing.T) {
	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), EmbeddedSkillMD, 0o644); err != nil {
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
	if changed {
		t.Fatal("EnsureScribeAgent() changed = true, want false")
	}
}

func TestEnsureScribeAgentReinstallsOnVersionMismatch(t *testing.T) {
	store := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(store, "scribe-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
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

func TestInstallScribeAgentValidatesFrontmatter(t *testing.T) {
	store := filepath.Join(t.TempDir(), "skills")
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	if _, err := InstallScribeAgent(store, st, []byte("---\nname: wrong\n---\n"), "v1.2.3"); err == nil {
		t.Fatal("InstallScribeAgent() error = nil, want validation error")
	}
}
