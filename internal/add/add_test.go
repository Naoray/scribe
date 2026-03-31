package add_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

func TestCandidateUploadFlag(t *testing.T) {
	cases := []struct {
		name   string
		c      add.Candidate
		upload bool
	}{
		{
			name:   "local with source",
			c:      add.Candidate{Name: "deploy", Source: "github:owner/repo@v1.0.0", LocalPath: "/home/user/.scribe/skills/deploy"},
			upload: false,
		},
		{
			name:   "local without source",
			c:      add.Candidate{Name: "cleanup", LocalPath: "/home/user/.claude/skills/cleanup"},
			upload: true,
		},
		{
			name:   "remote only",
			c:      add.Candidate{Name: "nextjs", Source: "github:vercel/skills@v2.0.0", Origin: "registry:vercel/skills"},
			upload: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.c.NeedsUpload() != tc.upload {
				t.Errorf("NeedsUpload() = %v, want %v", tc.c.NeedsUpload(), tc.upload)
			}
		})
	}
}

func TestAdderEmitNilSafe(t *testing.T) {
	a := &add.Adder{}
	// Should not panic with nil Emit callback.
	a.Emit = nil
	// No public method yet to test emit, but verifying struct creates without panic.
	if a.Client != nil {
		t.Error("expected nil client on zero-value Adder")
	}
}

func TestDiscoverLocalSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a local-only skill in ~/.claude/skills/cleanup/
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "cleanup")
	if err := os.MkdirAll(claudeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# Cleanup"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a scribe-managed skill in ~/.scribe/skills/deploy/
	scribeSkillDir := filepath.Join(home, ".scribe", "skills", "deploy")
	if err := os.MkdirAll(scribeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scribeSkillDir, "SKILL.md"), []byte("# Deploy"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"deploy": {Source: "github:owner/repo@v1.0.0", Version: "v1.0.0"},
		},
	}

	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	// Find each by name.
	byName := map[string]add.Candidate{}
	for _, c := range candidates {
		byName[c.Name] = c
	}

	cleanup, ok := byName["cleanup"]
	if !ok {
		t.Fatal("missing candidate: cleanup")
	}
	if cleanup.Source != "" {
		t.Errorf("cleanup source: got %q, want empty", cleanup.Source)
	}
	if !cleanup.NeedsUpload() {
		t.Error("cleanup should need upload")
	}
	if cleanup.Origin != "local" {
		t.Errorf("cleanup origin: got %q, want local", cleanup.Origin)
	}

	deploy, ok := byName["deploy"]
	if !ok {
		t.Fatal("missing candidate: deploy")
	}
	if deploy.Source != "github:owner/repo@v1.0.0" {
		t.Errorf("deploy source: got %q", deploy.Source)
	}
	if deploy.NeedsUpload() {
		t.Error("deploy should not need upload")
	}
}

func TestDiscoverLocalSkipsEmptyDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create an empty directory — should not be discovered.
	emptyDir := filepath.Join(home, ".claude", "skills", "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for empty dirs, got %d", len(candidates))
	}
}

func TestDiscoverLocalDeduplicates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Same skill name in both ~/.claude/skills/ and ~/.scribe/skills/ — local (claude) wins.
	for _, dir := range []string{
		filepath.Join(home, ".claude", "skills", "deploy"),
		filepath.Join(home, ".scribe", "skills", "deploy"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Deploy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 (deduplicated), got %d", len(candidates))
	}
	// Should be the claude path (scanned first, wins).
	if !strings.Contains(candidates[0].LocalPath, ".claude") {
		t.Errorf("expected claude path, got %q", candidates[0].LocalPath)
	}
}

func TestAddBuildsPushFilesForReference(t *testing.T) {
	// Test that Add correctly modifies scribe.toml for a source-reference candidate.
	// We can't easily test the full GitHub flow without a mock, so we test the
	// manifest modification logic instead.

	original := `[team]
name = "my-team"

[skills]
deploy = {source = "github:owner/repo@v1.0.0"}
`
	m, err := manifest.Parse([]byte(original))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate adding a new source-reference skill.
	m.Skills["gstack"] = manifest.Skill{Source: "github:garrytan/gstack@v0.12.9.0"}

	encoded, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Verify round-trip.
	m2, err := manifest.Parse(encoded)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if len(m2.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(m2.Skills))
	}
	gstack, ok := m2.Skills["gstack"]
	if !ok {
		t.Fatal("gstack not found after round-trip")
	}
	if gstack.Source != "github:garrytan/gstack@v0.12.9.0" {
		t.Errorf("gstack source: got %q", gstack.Source)
	}
}

func TestAddBuildsPushFilesForUpload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a local skill with files.
	skillDir := filepath.Join(home, ".claude", "skills", "cleanup")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Cleanup\nDo cleanup."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.md"), []byte("# Helper"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidate := add.Candidate{
		Name:      "cleanup",
		Origin:    "local",
		LocalPath: skillDir,
	}

	files, err := add.ReadLocalSkillFiles(candidate)
	if err != nil {
		t.Fatalf("ReadLocalSkillFiles: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Files should be keyed as skills/<name>/<filename>.
	for path := range files {
		if !strings.HasPrefix(path, "skills/cleanup/") {
			t.Errorf("unexpected file path: %q", path)
		}
	}
}

func TestDiscoverRemoteSkills(t *testing.T) {
	// DiscoverRemote takes parsed manifests rather than calling GitHub directly.
	// The cmd layer fetches manifests; the core just filters and converts.

	targetManifest := &manifest.Manifest{
		Team:   &manifest.Team{Name: "my-team"},
		Skills: map[string]manifest.Skill{
			"deploy": {Source: "github:owner/repo@v1.0.0"},
		},
	}

	otherManifests := map[string]*manifest.Manifest{
		"vercel/skills": {
			Team: &manifest.Team{Name: "vercel"},
			Skills: map[string]manifest.Skill{
				"nextjs":  {Source: "github:vercel/nextjs-skill@v2.0.0"},
				"deploy":  {Source: "github:vercel/deploy@v1.0.0"}, // already in target — should be filtered
			},
		},
	}

	adder := &add.Adder{}
	candidates := adder.DiscoverRemote(targetManifest, otherManifests)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 remote candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "nextjs" {
		t.Errorf("expected nextjs, got %q", candidates[0].Name)
	}
	if candidates[0].Origin != "registry:vercel/skills" {
		t.Errorf("origin: got %q", candidates[0].Origin)
	}
	if candidates[0].Source != "github:vercel/nextjs-skill@v2.0.0" {
		t.Errorf("source: got %q", candidates[0].Source)
	}
	if candidates[0].NeedsUpload() {
		t.Error("remote candidate should not need upload")
	}
}

func TestFilterAlreadyInTarget(t *testing.T) {
	targetManifest := &manifest.Manifest{
		Team: &manifest.Team{Name: "test"},
		Skills: map[string]manifest.Skill{
			"deploy": {Source: "github:owner/repo@v1.0.0"},
		},
	}

	candidates := []add.Candidate{
		{Name: "deploy", Source: "github:owner/repo@v1.0.0"},
		{Name: "cleanup", LocalPath: "/path/to/cleanup"},
	}

	// filterAlreadyInTarget is in cmd/, so test the equivalent logic here.
	var filtered []add.Candidate
	for _, c := range candidates {
		if _, exists := targetManifest.Skills[c.Name]; !exists {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 after filter, got %d", len(filtered))
	}
	if filtered[0].Name != "cleanup" {
		t.Errorf("expected cleanup, got %q", filtered[0].Name)
	}
}

func TestReadLocalSkillFilesNested(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, "skill")
	subDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "run.sh"), []byte("#!/bin/sh"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := add.Candidate{Name: "myskill", LocalPath: skillDir}
	files, err := add.ReadLocalSkillFiles(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if _, ok := files["skills/myskill/SKILL.md"]; !ok {
		t.Error("missing skills/myskill/SKILL.md")
	}
	if _, ok := files["skills/myskill/scripts/run.sh"]; !ok {
		t.Error("missing skills/myskill/scripts/run.sh")
	}
}
