package workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func TestStepFilterRegistries_OnlyEnabled(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: true},
			{Repo: "acme/disabled-repo", Enabled: false},
			{Repo: "acme/default-repo", Enabled: true},
		},
	}

	b := &Bag{Config: cfg}

	if err := StepFilterRegistries(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(b.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(b.Repos), b.Repos)
	}

	if b.Repos[0] != "acme/team-skills" {
		t.Errorf("repos[0]: got %q, want %q", b.Repos[0], "acme/team-skills")
	}
	if b.Repos[1] != "acme/default-repo" {
		t.Errorf("repos[1]: got %q, want %q", b.Repos[1], "acme/default-repo")
	}
}

func TestStepFilterRegistries_WithFilterFunc(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: true},
			{Repo: "acme/other-repo", Enabled: true},
		},
	}

	b := &Bag{
		Config:   cfg,
		RepoFlag: "acme/team-skills",
		FilterRegistries: func(flag string, repos []string) ([]string, error) {
			for _, r := range repos {
				if r == flag {
					return []string{r}, nil
				}
			}
			return repos, nil
		},
	}

	if err := StepFilterRegistries(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(b.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(b.Repos), b.Repos)
	}
	if b.Repos[0] != "acme/team-skills" {
		t.Errorf("repos[0]: got %q, want %q", b.Repos[0], "acme/team-skills")
	}
}

func TestStepResolveKitFilter_WithProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create project dir with .scribe.yaml referencing test-kit
	projectDir := t.TempDir()
	pfContent := "kits:\n  - test-kit\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	// Create ~/.scribe/kits/test-kit.yaml with skills: [recap]
	kitsDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatalf("mkdir kits: %v", err)
	}
	kitContent := "name: test-kit\nskills:\n  - recap\n"
	if err := os.WriteFile(filepath.Join(kitsDir, "test-kit.yaml"), []byte(kitContent), 0o644); err != nil {
		t.Fatalf("write kit file: %v", err)
	}

	// Change into the project dir so projectfile.Find(wd) finds .scribe.yaml
	t.Chdir(projectDir)

	b := &Bag{
		ProjectRoot: projectDir,
		State: &state.State{Installed: map[string]state.InstalledSkill{
			"recap":    {},
			"debugger": {},
		}},
	}

	if err := StepResolveKitFilter(context.Background(), b); err != nil {
		t.Fatalf("StepResolveKitFilter: %v", err)
	}

	if len(b.KitFilter) != 1 || b.KitFilter[0] != "recap" {
		t.Fatalf("KitFilter = %v, want [recap]", b.KitFilter)
	}
	if !b.KitFilterEnabled {
		t.Fatal("KitFilterEnabled should be true after kit resolution")
	}
}

func TestWorkflowSkipMissingInstallsProjectKitSkills(t *testing.T) {
	if workflowSkipMissing(&Bag{KitFilterEnabled: true}) {
		t.Fatal("project kit sync should not skip missing skills")
	}
	if !workflowSkipMissing(&Bag{}) {
		t.Fatal("legacy sync should skip missing skills")
	}
	if workflowSkipMissing(&Bag{SkillFilter: []string{"recap"}}) {
		t.Fatal("explicit skill filter should not skip missing skills")
	}
	if workflowSkipMissing(&Bag{InstallAllFlag: true}) {
		t.Fatal("--all should not skip missing skills")
	}
}

func TestStepResolveKitFilter_EmptyKitResolvesZeroSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	// .scribe.yaml lists a kit, but that kit has no skills
	pfContent := "kits:\n  - empty-kit\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	kitsDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatalf("mkdir kits: %v", err)
	}
	// Kit exists but has no skills
	if err := os.WriteFile(filepath.Join(kitsDir, "empty-kit.yaml"), []byte("name: empty-kit\nskills: []\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	t.Chdir(projectDir)

	b := &Bag{
		ProjectRoot: projectDir,
		State: &state.State{Installed: map[string]state.InstalledSkill{
			"recap": {},
		}},
	}
	if err := StepResolveKitFilter(context.Background(), b); err != nil {
		t.Fatalf("StepResolveKitFilter: %v", err)
	}
	if !b.KitFilterEnabled {
		t.Fatal("KitFilterEnabled should be true when project file exists")
	}
	if len(b.KitFilter) != 0 {
		t.Fatalf("KitFilter = %v, want [] (empty kit)", b.KitFilter)
	}
}

func TestStepResolveKitFilter_NoProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	emptyDir := t.TempDir()
	t.Chdir(emptyDir)

	b := &Bag{
		ProjectRoot: "", // no project scope
		State: &state.State{Installed: map[string]state.InstalledSkill{
			"recap": {},
		}},
	}

	if err := StepResolveKitFilter(context.Background(), b); err != nil {
		t.Fatalf("StepResolveKitFilter: %v", err)
	}

	if b.KitFilter != nil {
		t.Fatalf("KitFilter = %v, want nil (no project scope)", b.KitFilter)
	}
}

func TestStepResolveMCPServers_WithProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	pfContent := "kits:\n  - runtime-kit\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	kitsDir := filepath.Join(home, ".scribe", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatalf("mkdir kits: %v", err)
	}
	kitContent := "name: runtime-kit\nskills:\n  - recap\nmcp_servers:\n  - playwright\n  - mempalace\n"
	if err := os.WriteFile(filepath.Join(kitsDir, "runtime-kit.yaml"), []byte(kitContent), 0o644); err != nil {
		t.Fatalf("write kit file: %v", err)
	}

	t.Chdir(projectDir)

	b := &Bag{ProjectRoot: projectDir}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers: %v", err)
	}

	want := []string{"mempalace", "playwright"}
	if len(b.ProjectMCPServers) != len(want) {
		t.Fatalf("ProjectMCPServers = %v, want %v", b.ProjectMCPServers, want)
	}
	for i := range want {
		if b.ProjectMCPServers[i] != want[i] {
			t.Fatalf("ProjectMCPServers = %v, want %v", b.ProjectMCPServers, want)
		}
	}
	if !b.ProjectMCPServersEnabled {
		t.Fatal("ProjectMCPServersEnabled should be true after MCP server resolution")
	}
}

func TestStepResolveMCPServers_WithDirectProjectMCP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	pfContent := "mcp:\n  - mempalace\nmcp_servers:\n  - playwright\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	t.Chdir(projectDir)

	b := &Bag{ProjectRoot: projectDir}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers: %v", err)
	}

	want := []string{"mempalace", "playwright"}
	if len(b.ProjectMCPServers) != len(want) {
		t.Fatalf("ProjectMCPServers = %v, want %v", b.ProjectMCPServers, want)
	}
	for i := range want {
		if b.ProjectMCPServers[i] != want[i] {
			t.Fatalf("ProjectMCPServers = %v, want %v", b.ProjectMCPServers, want)
		}
	}
}

func TestStepProjectSnippets_WritesTargetsAndState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	pfContent := "snippets:\n  - commit-discipline\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetContent := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude, codex, cursor]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(filepath.Join(snippetDir, "commit-discipline.md"), []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}

	b := &Bag{
		ProjectRoot: projectDir,
		State: &state.State{
			Snippets: map[string]state.InstalledSnippet{},
		},
		Tools: []tools.Tool{tools.ClaudeTool{}, tools.CodexTool{}, tools.CursorTool{}},
	}
	if err := StepProjectSnippets(context.Background(), b); err != nil {
		t.Fatalf("StepProjectSnippets: %v", err)
	}

	for _, rel := range []string{"CLAUDE.md", "AGENTS.md", filepath.Join(".cursor", "rules", "commit-discipline.mdc")} {
		data, err := os.ReadFile(filepath.Join(projectDir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "Agent Commit Discipline") {
			t.Fatalf("%s missing snippet body:\n%s", rel, data)
		}
	}
	if _, ok := b.State.Snippets["commit-discipline"]; !ok {
		t.Fatalf("state snippet missing: %#v", b.State.Snippets)
	}
	if !b.StateDirty {
		t.Fatal("StateDirty = false, want true")
	}
}

func TestStepProjectSnippets_MarksStateDirtyWhenProjectionUnchangedButStateMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	pfContent := "snippets:\n  - commit-discipline\n"
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte(pfContent), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetContent := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(filepath.Join(snippetDir, "commit-discipline.md"), []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}

	first := &Bag{
		ProjectRoot: projectDir,
		State:       &state.State{Snippets: map[string]state.InstalledSnippet{}},
		Tools:       []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepProjectSnippets(context.Background(), first); err != nil {
		t.Fatalf("StepProjectSnippets initial: %v", err)
	}

	second := &Bag{
		ProjectRoot: projectDir,
		State:       &state.State{Snippets: map[string]state.InstalledSnippet{}},
		Tools:       []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepProjectSnippets(context.Background(), second); err != nil {
		t.Fatalf("StepProjectSnippets second: %v", err)
	}
	if !second.StateDirty {
		t.Fatal("StateDirty = false, want true for missing snippet state")
	}
	if _, ok := second.State.Snippets["commit-discipline"]; !ok {
		t.Fatalf("state snippet missing: %#v", second.State.Snippets)
	}
}

func TestStepProjectSnippets_RemovesStaleBlocksWhenProjectFileClearsSnippets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, projectfile.Filename)
	if err := os.WriteFile(projectPath, []byte("snippets:\n  - commit-discipline\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetContent := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(filepath.Join(snippetDir, "commit-discipline.md"), []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	b := &Bag{
		ProjectRoot: projectDir,
		State:       &state.State{Snippets: map[string]state.InstalledSnippet{}},
		Tools:       []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepProjectSnippets(context.Background(), b); err != nil {
		t.Fatalf("StepProjectSnippets initial: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte("snippets: []\n"), 0o644); err != nil {
		t.Fatalf("clear project snippets: %v", err)
	}
	if err := StepProjectSnippets(context.Background(), b); err != nil {
		t.Fatalf("StepProjectSnippets clear: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if strings.Contains(string(data), "Agent Commit Discipline") {
		t.Fatalf("stale snippet remains projected:\n%s", data)
	}
}

func TestStepProjectSnippets_RemovesLegacyCursorRuleForStaleSnippet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, projectfile.Filename)
	if err := os.WriteFile(projectPath, []byte("snippets: []\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	snippetDir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	snippetPath := filepath.Join(snippetDir, "commit-discipline.md")
	snippetContent := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [cursor]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(snippetPath, []byte(snippetContent), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	cursorRulesDir := filepath.Join(projectDir, ".cursor", "rules")
	if err := os.MkdirAll(cursorRulesDir, 0o755); err != nil {
		t.Fatalf("mkdir cursor rules: %v", err)
	}
	legacyRule := filepath.Join(cursorRulesDir, "commit-discipline.mdc")
	if err := os.WriteFile(legacyRule, []byte("---\ndescription: Commit rules\nalwaysApply: false\n---\n# Agent Commit Discipline\n"), 0o644); err != nil {
		t.Fatalf("write legacy cursor rule: %v", err)
	}
	userRule := filepath.Join(cursorRulesDir, "user-rule.mdc")
	if err := os.WriteFile(userRule, []byte("---\ndescription: user rule\nalwaysApply: false\n---\n# User Rule\n"), 0o644); err != nil {
		t.Fatalf("write user cursor rule: %v", err)
	}
	b := &Bag{
		ProjectRoot: projectDir,
		State: &state.State{Snippets: map[string]state.InstalledSnippet{
			"commit-discipline": {Source: snippetPath, Targets: []string{"cursor"}},
		}},
		Tools: []tools.Tool{tools.CursorTool{}},
	}

	if err := StepProjectSnippets(context.Background(), b); err != nil {
		t.Fatalf("StepProjectSnippets: %v", err)
	}
	if _, err := os.Stat(legacyRule); !os.IsNotExist(err) {
		t.Fatalf("legacy cursor rule still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(userRule); err != nil {
		t.Fatalf("user cursor rule removed: %v", err)
	}
	if !b.StateDirty {
		t.Fatal("StateDirty = false, want true")
	}
}

func TestStepResolveMCPServers_NoProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	emptyDir := t.TempDir()
	t.Chdir(emptyDir)

	b := &Bag{ProjectRoot: ""}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers: %v", err)
	}
	if b.ProjectMCPServers != nil {
		t.Fatalf("ProjectMCPServers = %v, want nil (no project scope)", b.ProjectMCPServers)
	}
	if b.ProjectMCPServersEnabled {
		t.Fatal("ProjectMCPServersEnabled should be false without project scope")
	}
}

func TestStepResolveMCPServers_MalformedProjectFileNonFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, projectfile.Filename), []byte("kits:\n  - [broken\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	t.Chdir(projectDir)

	b := &Bag{ProjectRoot: projectDir}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers: %v", err)
	}
	if b.ProjectMCPServers != nil {
		t.Fatalf("ProjectMCPServers = %v, want nil after malformed project file", b.ProjectMCPServers)
	}
	if b.ProjectMCPServersEnabled {
		t.Fatal("ProjectMCPServersEnabled should be false after malformed project file")
	}
}

func TestStepProjectClaudeMCPServers_WritesProjectSettings(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")
	existing := []byte(`{
  "permissions": {
    "allow": ["Bash(go test ./...)"]
  },
  "enableAllProjectMcpServers": true,
  "enabledMcpjsonServers": ["old-server"]
}
`)
	if err := os.WriteFile(settingsPath, existing, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"playwright", "mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepProjectClaudeMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepProjectClaudeMCPServers: %v", err)
	}

	var got map[string]any
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if got["enableAllProjectMcpServers"] != false {
		t.Fatalf("enableAllProjectMcpServers = %v, want false", got["enableAllProjectMcpServers"])
	}
	wantServers := []any{"mempalace", "playwright"}
	servers, ok := got["enabledMcpjsonServers"].([]any)
	if !ok {
		t.Fatalf("enabledMcpjsonServers = %T, want array", got["enabledMcpjsonServers"])
	}
	if len(servers) != len(wantServers) {
		t.Fatalf("enabledMcpjsonServers = %v, want %v", servers, wantServers)
	}
	for i := range wantServers {
		if servers[i] != wantServers[i] {
			t.Fatalf("enabledMcpjsonServers = %v, want %v", servers, wantServers)
		}
	}
	permissions, ok := got["permissions"].(map[string]any)
	if !ok || permissions["allow"] == nil {
		t.Fatalf("permissions not preserved: %v", got["permissions"])
	}
}

func TestProjectClaudeMCPServers_UpdatesWhenProjectMCPChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, projectfile.Filename)
	if err := os.WriteFile(projectPath, []byte("mcp:\n  - mempalace\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	t.Chdir(projectDir)

	b := &Bag{
		ProjectRoot: projectDir,
		Tools:       []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers initial: %v", err)
	}
	if err := StepProjectClaudeMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepProjectClaudeMCPServers initial: %v", err)
	}

	if err := os.WriteFile(projectPath, []byte("mcp: []\n"), 0o644); err != nil {
		t.Fatalf("clear project mcp: %v", err)
	}
	if err := StepResolveMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepResolveMCPServers cleared: %v", err)
	}
	if err := StepProjectClaudeMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepProjectClaudeMCPServers cleared: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	servers, ok := got["enabledMcpjsonServers"].([]any)
	if !ok {
		t.Fatalf("enabledMcpjsonServers = %T, want array", got["enabledMcpjsonServers"])
	}
	if len(servers) != 0 {
		t.Fatalf("enabledMcpjsonServers = %v, want empty after clearing mcp", servers)
	}
}

func TestStepProjectClaudeMCPServers_SkipsWhenClaudeInactive(t *testing.T) {
	projectDir := t.TempDir()
	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    nil,
	}
	if err := StepProjectClaudeMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepProjectClaudeMCPServers: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings file was written or stat failed: %v", err)
	}
}

func TestStepProjectClaudeMCPServers_MalformedSettingsFailsWithoutOverwrite(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")
	original := []byte(`{"permissions":`)
	if err := os.WriteFile(settingsPath, original, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    []tools.Tool{tools.ClaudeTool{}},
	}
	if err := StepProjectClaudeMCPServers(context.Background(), b); err == nil {
		t.Fatal("StepProjectClaudeMCPServers error = nil, want malformed settings error")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("settings overwritten: %q, want %q", data, original)
	}
}

func TestStepProjectMCPServers_ProjectsCodexAndCursorFromMCPJSON(t *testing.T) {
	projectDir := t.TempDir()
	mcpJSON := `{
  "mcpServers": {
    "mempalace": {
      "command": "mempalace",
      "args": ["serve"],
      "env": {"TOKEN": "abc"}
    },
    "figma": {
      "url": "https://mcp.figma.com/mcp",
      "headers": {"X-Figma-Region": "us-east-1"}
    },
    "unused": {
      "command": "unused"
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(projectDir, ".mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cursorDir := filepath.Join(projectDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("mkdir cursor dir: %v", err)
	}
	existingCursor := `{
  "mcpServers": {
    "manual": {"command": "manual"},
    "old-managed": {"command": "old"}
  }
}
`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(existingCursor), 0o644); err != nil {
		t.Fatalf("write cursor mcp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "scribe-mcp.json"), []byte(`{"servers":["old-managed"]}`), 0o644); err != nil {
		t.Fatalf("write cursor sidecar: %v", err)
	}

	codexDir := filepath.Join(projectDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	existingCodex := `[mcp_servers.manual]
command = "manual"

[mcp_servers.old-managed]
command = "old"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existingCodex), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "scribe-mcp.json"), []byte(`{"servers":["old-managed"]}`), 0o644); err != nil {
		t.Fatalf("write codex sidecar: %v", err)
	}

	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"figma", "mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    []tools.Tool{tools.CodexTool{}, tools.CursorTool{}},
	}
	if err := StepProjectMCPServers(context.Background(), b); err != nil {
		t.Fatalf("StepProjectMCPServers: %v", err)
	}

	cursorData, err := os.ReadFile(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatalf("read cursor mcp: %v", err)
	}
	var cursorConfig struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(cursorData, &cursorConfig); err != nil {
		t.Fatalf("parse cursor mcp: %v", err)
	}
	if _, ok := cursorConfig.MCPServers["manual"]; !ok {
		t.Fatalf("cursor manual server not preserved: %#v", cursorConfig.MCPServers)
	}
	if _, ok := cursorConfig.MCPServers["old-managed"]; ok {
		t.Fatalf("cursor old managed server not removed: %#v", cursorConfig.MCPServers)
	}
	if got := cursorConfig.MCPServers["figma"]["url"]; got != "https://mcp.figma.com/mcp" {
		t.Fatalf("cursor figma url = %v", got)
	}
	if got := cursorConfig.MCPServers["mempalace"]["command"]; got != "mempalace" {
		t.Fatalf("cursor mempalace command = %v", got)
	}

	codexData, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	var codexConfig struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(codexData, &codexConfig); err != nil {
		t.Fatalf("parse codex config: %v", err)
	}
	if _, ok := codexConfig.MCPServers["manual"]; !ok {
		t.Fatalf("codex manual server not preserved: %#v", codexConfig.MCPServers)
	}
	if _, ok := codexConfig.MCPServers["old-managed"]; ok {
		t.Fatalf("codex old managed server not removed: %#v", codexConfig.MCPServers)
	}
	if got := codexConfig.MCPServers["figma"]["url"]; got != "https://mcp.figma.com/mcp" {
		t.Fatalf("codex figma url = %v", got)
	}
	if _, ok := codexConfig.MCPServers["figma"]["headers"]; ok {
		t.Fatalf("codex headers key should be converted: %#v", codexConfig.MCPServers["figma"])
	}
	if _, ok := codexConfig.MCPServers["figma"]["http_headers"]; !ok {
		t.Fatalf("codex http_headers missing: %#v", codexConfig.MCPServers["figma"])
	}
	if got := codexConfig.MCPServers["mempalace"]["command"]; got != "mempalace" {
		t.Fatalf("codex mempalace command = %v", got)
	}
}

func TestStepProjectMCPServers_RequiresDefinitionsForCodexCursor(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".mcp.json"), []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}
	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    []tools.Tool{tools.CodexTool{}},
	}
	err := StepProjectMCPServers(context.Background(), b)
	if err == nil || !strings.Contains(err.Error(), "missing from .mcp.json") {
		t.Fatalf("StepProjectMCPServers error = %v, want missing definition error", err)
	}
}

func TestStepProjectMCPServers_ValidatesDefinitionsBeforeClaudeWrite(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".mcp.json"), []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}
	b := &Bag{
		ProjectRoot:              projectDir,
		ProjectMCPServers:        []string{"mempalace"},
		ProjectMCPServersEnabled: true,
		Tools:                    []tools.Tool{tools.ClaudeTool{}, tools.CodexTool{}},
	}
	err := StepProjectMCPServers(context.Background(), b)
	if err == nil || !strings.Contains(err.Error(), "missing from .mcp.json") {
		t.Fatalf("StepProjectMCPServers error = %v, want missing definition error", err)
	}
	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("claude settings written before validation failed: %v", err)
	}
}
