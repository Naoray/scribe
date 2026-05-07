package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/projectstore"
	"github.com/Naoray/scribe/internal/state"
	isync "github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func TestProjectSkillCreateMarksOriginProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newProjectSkillCreateCommand()
	cmd.SetArgs([]string{"review"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got := st.Installed["review"].Origin; got != state.OriginProject {
		t.Fatalf("Origin = %q, want project", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "skills", "review", "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
}

func TestProjectSkillClaimRefusesRegistryOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	st := stateFixture(t, home)
	st.Installed["review"] = state.InstalledSkill{
		Origin: state.OriginRegistry,
		Sources: []state.SkillSource{{
			Registry: "acme/skills",
		}},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	cmd := newProjectSkillClaimCommand()
	cmd.SetArgs([]string{"review"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("claim should refuse registry-origin skills")
	}
}

func TestProjectSyncVendorsProjectSkillAndPinsRegistrySkill(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	mustChdir(t, project)

	mustWriteProjectFile(t, filepath.Join(project, ".scribe.yaml"), "kits: [core]\nadd: [review]\n")
	mustSaveProjectKit(t, filepath.Join(home, ".scribe", "kits", "core.yaml"), &kit.Kit{Name: "core", Skills: []string{"deploy"}})
	mustWriteProjectFile(t, filepath.Join(home, ".scribe", "skills", "review", "SKILL.md"), "# review\n")
	mustWriteProjectFile(t, filepath.Join(home, ".scribe", "skills", "deploy", "SKILL.md"), "# deploy\n")

	st := stateFixture(t, home)
	st.Installed["review"] = state.InstalledSkill{Origin: state.OriginProject}
	st.Installed["deploy"] = state.InstalledSkill{
		InstalledHash: "hash",
		Sources: []state.SkillSource{{
			Registry:   "acme/skills",
			SourceRepo: "acme/source",
			Path:       "skills/deploy",
			Ref:        "main",
			LastSHA:    "abc123",
			LastSynced: time.Now(),
		}},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	cmd := newProjectSyncCommand()
	cmd.SetArgs([]string{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("project sync: %v\n%s", err, out.String())
	}
	if _, _, err := projectstore.VerifyMarker(filepath.Join(project, ".ai", "skills", "review")); err != nil {
		t.Fatalf("verify vendored marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".ai", "kits", "core.yaml")); err != nil {
		t.Fatalf("project kit missing: %v", err)
	}
	lf, err := projectstore.Project(project).LoadProjectLockfile()
	if err != nil {
		t.Fatalf("load lockfile: %v", err)
	}
	entry, ok := lf.Entry("deploy")
	if !ok {
		t.Fatal("deploy pin missing")
	}
	if entry.SourceRegistry != "acme/skills" || entry.SourceRepo != "acme/source" || entry.Path != "skills/deploy" {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestProjectSyncCheckDetectsDrift(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	mustChdir(t, project)
	mustWriteProjectFile(t, filepath.Join(project, ".scribe.yaml"), "add: [review]\n")
	mustWriteProjectFile(t, filepath.Join(home, ".scribe", "skills", "review", "SKILL.md"), "# review\n")
	st := stateFixture(t, home)
	st.Installed["review"] = state.InstalledSkill{Origin: state.OriginProject}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
	cmd := newProjectSyncCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	mustWriteProjectFile(t, filepath.Join(home, ".scribe", "skills", "review", "SKILL.md"), "# changed\n")
	check := newProjectSyncCommand()
	check.SetArgs([]string{"--check"})
	err := check.Execute()
	if err == nil || !strings.Contains(err.Error(), "project artifacts are out of date") {
		t.Fatalf("check error = %v, want drift", err)
	}
}

func TestTeamShareAuthorPublishTeammateBoostSync(t *testing.T) {
	authorHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", authorHome)
	mustChdir(t, project)
	mustWriteProjectFile(t, filepath.Join(project, "composer.json"), `{"require":{"laravel/boost":"^1.0"}}`)
	mustWriteProjectFile(t, filepath.Join(project, ".scribe.yaml"), "add: [review]\n")
	mustWriteProjectFile(t, filepath.Join(authorHome, ".scribe", "skills", "review", "SKILL.md"), "# review\n")
	st := stateFixture(t, authorHome)
	st.Installed["review"] = state.InstalledSkill{Origin: state.OriginProject}
	if err := st.Save(); err != nil {
		t.Fatalf("save author state: %v", err)
	}
	publish := newProjectSyncCommand()
	if err := publish.Execute(); err != nil {
		t.Fatalf("author project sync: %v", err)
	}

	teammateHome := t.TempDir()
	t.Setenv("HOME", teammateHome)
	claudeRealDir := filepath.Join(project, ".claude", "skills", "review")
	mustWriteProjectFile(t, filepath.Join(claudeRealDir, "SKILL.md"), "# boost copy\n")
	teammateState := &state.State{
		SchemaVersion: state.CurrentSchemaVersion,
		Installed:     map[string]state.InstalledSkill{},
		VendorState:   map[string]state.VendorState{},
	}
	projectLock, err := projectstore.Project(project).LoadProjectLockfile()
	if err != nil {
		t.Fatalf("load project lock: %v", err)
	}
	syncer := &isync.Syncer{
		Tools:       []tools.Tool{tools.ClaudeTool{}, projectTestTool{name: "codex"}},
		ProjectRoot: project,
	}
	if err := syncer.RunProject(context.Background(), teammateState, projectLock); err != nil {
		t.Fatalf("teammate RunProject: %v", err)
	}
	if _, err := os.Stat(claudeRealDir); err != nil {
		t.Fatalf("Boost Claude dir should remain: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", "review")); err != nil {
		t.Fatalf("Codex projection missing: %v", err)
	}
	if teammateState.VendorState["review"].FirstSeenAt.IsZero() {
		t.Fatal("VendorState first seen not recorded")
	}
}

func stateFixture(t *testing.T, home string) *state.State {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".scribe"), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Installed == nil {
		st.Installed = map[string]state.InstalledSkill{}
	}
	return st
}

func mustSaveProjectKit(t *testing.T, path string, k *kit.Kit) {
	t.Helper()
	if err := kit.Save(path, k); err != nil {
		t.Fatalf("save kit: %v", err)
	}
}

func mustWriteProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mustChdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

type projectTestTool struct {
	name string
}

func (t projectTestTool) Name() string           { return t.name }
func (t projectTestTool) Detect() bool           { return true }
func (t projectTestTool) Uninstall(string) error { return nil }
func (t projectTestTool) CanonicalTarget(canonicalDir string) (string, bool) {
	return canonicalDir, true
}
func (t projectTestTool) SkillPath(skillName, projectRoot string) (string, error) {
	return filepath.Join(projectRoot, "."+t.name, "skills", skillName), nil
}
func (t projectTestTool) Install(skillName, canonicalDir, projectRoot string) ([]string, error) {
	path, err := t.SkillPath(skillName, projectRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	if err := os.Symlink(canonicalDir, path); err != nil {
		return nil, err
	}
	return []string{path}, nil
}
