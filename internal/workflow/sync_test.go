package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
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
