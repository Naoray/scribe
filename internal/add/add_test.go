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
