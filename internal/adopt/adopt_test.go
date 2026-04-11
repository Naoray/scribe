package adopt_test

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// blobSHA duplicates the package-internal gitBlobSHA.
// Intentional: test is black-box (package adopt_test) so can't call the unexported fn.
// Drift risk: keep in sync with candidate.go — drift documented here.
func blobSHA(data []byte) string {
	payload := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf("%x", sum)
}

// writeSkill creates a minimal skill directory with SKILL.md at dir/name/.
func writeSkill(t *testing.T, parentDir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(parentDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

// emptyState returns a fresh state with no installed skills.
func emptyState() *state.State {
	return &state.State{
		SchemaVersion: 4,
		Installed:     make(map[string]state.InstalledSkill),
	}
}

// adoptionCfg builds an AdoptionConfig pointing at an extra path.
// Builtins (~/.claude/skills, ~/.codex/skills) are always included by AdoptionPaths.
func adoptionCfg(extraPaths ...string) config.AdoptionConfig {
	return config.AdoptionConfig{
		Mode:  "auto",
		Paths: extraPaths,
	}
}

// ---------------------------------------------------------------------------
// Mock Tool
// ---------------------------------------------------------------------------

type mockTool struct {
	name         string
	installErr   error
	installedAt  []string
	uninstallErr error
}

func (m *mockTool) Name() string { return m.name }
func (m *mockTool) Detect() bool { return true }
func (m *mockTool) Install(skillName, canonicalDir string) ([]string, error) {
	if m.installErr != nil {
		return nil, m.installErr
	}
	path := filepath.Join(m.name, skillName)
	m.installedAt = append(m.installedAt, path)
	return []string{path}, nil
}
func (m *mockTool) Uninstall(skillName string) error { return m.uninstallErr }
func (m *mockTool) SkillPath(skillName string) (string, error) {
	return filepath.Join(m.name, skillName), nil
}

// ---------------------------------------------------------------------------
// TestFindCandidates
// ---------------------------------------------------------------------------

func TestFindCandidates_FromClaudeSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkill(t, claudeSkills, "my-skill", "# my-skill\nsome content")

	st := emptyState()
	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	c := candidates[0]
	if c.Name != "my-skill" {
		t.Errorf("name = %q, want %q", c.Name, "my-skill")
	}
	if len(c.Targets) != 1 || c.Targets[0] != "claude" {
		t.Errorf("targets = %v, want [claude]", c.Targets)
	}
	if c.Hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestFindCandidates_EmptyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the dir but leave it empty.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	st := emptyState()
	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 || len(conflicts) != 0 {
		t.Errorf("expected empty results, got candidates=%d conflicts=%d", len(candidates), len(conflicts))
	}
}

func TestFindCandidates_MissingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Don't create .claude/skills at all.
	st := emptyState()
	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 || len(conflicts) != 0 {
		t.Errorf("expected empty results, got candidates=%d conflicts=%d", len(candidates), len(conflicts))
	}
}

func TestFindCandidates_IdempotentSameHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	content := "# my-skill\nsome content"
	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkill(t, claudeSkills, "my-skill", content)

	hash := blobSHA([]byte(content))
	st := emptyState()
	st.Installed["my-skill"] = state.InstalledSkill{
		Revision:      1,
		InstalledHash: hash,
		Tools:         []string{"claude"},
		Origin:        state.OriginLocal,
	}

	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 || len(conflicts) != 0 {
		t.Errorf("expected no-op, got candidates=%d conflicts=%d", len(candidates), len(conflicts))
	}
}

func TestFindCandidates_ConflictDifferentHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	content := "# my-skill\nnew content"
	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeSkill(t, claudeSkills, "my-skill", content)

	oldHash := blobSHA([]byte("# my-skill\nold content"))
	st := emptyState()
	st.Installed["my-skill"] = state.InstalledSkill{
		Revision:      1,
		InstalledHash: oldHash,
		Tools:         []string{"claude"},
		Origin:        state.OriginLocal,
	}

	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Name != "my-skill" {
		t.Errorf("conflict name = %q", conflicts[0].Name)
	}
}

func TestFindCandidates_ExtraConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	extra := filepath.Join(home, "my-skills")
	writeSkill(t, extra, "custom-skill", "# custom-skill\ncontent")

	st := emptyState()
	// Pass extra path relative to home using tilde notation (resolved by AdoptionPaths).
	cfg := config.AdoptionConfig{
		Mode:  "auto",
		Paths: []string{extra},
	}

	candidates, conflicts, err := adopt.FindCandidates(st, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}

	found := false
	for _, c := range candidates {
		if c.Name == "custom-skill" {
			found = true
			// Extra user paths have no known tool mapping.
			if len(c.Targets) != 0 {
				t.Errorf("extra-path candidate should have nil targets, got %v", c.Targets)
			}
		}
	}
	if !found {
		t.Error("custom-skill not found in candidates")
	}
}

func TestFindCandidates_SkipsPackageSubSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeSkills := filepath.Join(home, ".claude", "skills")

	// Create parent package skill: gstack/browse/SKILL.md (real file).
	pkgSkillDir := filepath.Join(claudeSkills, "gstack", "browse")
	if err := os.MkdirAll(pkgSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgSkillDir, "SKILL.md"), []byte("# gstack/browse"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create sub-skill entry: browse/SKILL.md → ../gstack/browse/SKILL.md (symlink).
	subSkillDir := filepath.Join(claudeSkills, "browse")
	if err := os.MkdirAll(subSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink target is relative to the sub-skill dir.
	symlinkPath := filepath.Join(subSkillDir, "SKILL.md")
	symlinkTarget := filepath.Join("..", "gstack", "browse", "SKILL.md")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Also add a normal standalone skill to confirm it still appears.
	writeSkill(t, claudeSkills, "standalone", "# standalone\ncontent")

	st := emptyState()
	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}

	for _, c := range candidates {
		if c.Name == "browse" {
			t.Errorf("browse is a package sub-skill and must not appear in candidates")
		}
	}

	found := false
	for _, c := range candidates {
		if c.Name == "standalone" {
			found = true
		}
	}
	if !found {
		t.Error("standalone skill should still appear as a candidate")
	}
}

// ---------------------------------------------------------------------------
// TestApply — happy path
// ---------------------------------------------------------------------------

func TestApply_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Set up scribe store dir inside temp home.
	storeDir := filepath.Join(home, ".scribe", "skills")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCRIBE_STORE", storeDir) // not used by code directly; store resolves via HOME

	content := "# test-skill\nHello world"
	srcDir := filepath.Join(home, "source-skills")
	writeSkill(t, srcDir, "test-skill", content)

	hash := blobSHA([]byte(content))
	cand := adopt.Candidate{
		Name:      "test-skill",
		LocalPath: filepath.Join(srcDir, "test-skill"),
		Targets:   []string{"mock"},
		Hash:      hash,
	}

	st := emptyState()
	// Point state file to temp home.
	mockT := &mockTool{name: "mock"}
	adopter := &adopt.Adopter{
		State: st,
		Tools: []tools.Tool{mockT},
		Emit:  func(any) {},
	}

	result := adopter.Apply([]adopt.Candidate{cand})

	if len(result.Adopted) != 1 || result.Adopted[0] != "test-skill" {
		t.Errorf("adopted = %v, want [test-skill]", result.Adopted)
	}
	if len(result.Failed) != 0 {
		t.Errorf("failed = %v, want empty", result.Failed)
	}

	// Check installed in state (in-memory — Save requires real paths).
	installed, ok := st.Installed["test-skill"]
	if !ok {
		t.Fatal("test-skill not in state")
	}
	if installed.Origin != state.OriginLocal {
		t.Errorf("origin = %q, want %q", installed.Origin, state.OriginLocal)
	}
	if installed.InstalledHash != hash {
		t.Errorf("hash = %q, want %q", installed.InstalledHash, hash)
	}
	if installed.Revision != 1 {
		t.Errorf("revision = %d, want 1", installed.Revision)
	}
	if installed.Sources != nil {
		t.Errorf("sources = %v, want nil", installed.Sources)
	}
	if len(installed.Tools) != 1 || installed.Tools[0] != "mock" {
		t.Errorf("tools = %v, want [mock]", installed.Tools)
	}
}

// ---------------------------------------------------------------------------
// TestApply — failure rollback
// ---------------------------------------------------------------------------

func TestApply_FailureRollback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	content := "# fail-skill\nContent"
	srcDir := filepath.Join(home, "source-skills")
	writeSkill(t, srcDir, "fail-skill", content)

	hash := blobSHA([]byte(content))
	cand := adopt.Candidate{
		Name:      "fail-skill",
		LocalPath: filepath.Join(srcDir, "fail-skill"),
		Targets:   []string{"badtool"},
		Hash:      hash,
	}

	st := emptyState()
	badTool := &mockTool{name: "badtool", installErr: fmt.Errorf("install exploded")}
	var events []any
	adopter := &adopt.Adopter{
		State: st,
		Tools: []tools.Tool{badTool},
		Emit:  func(msg any) { events = append(events, msg) },
	}

	result := adopter.Apply([]adopt.Candidate{cand})

	// Source file must still be present.
	if _, err := os.Stat(filepath.Join(srcDir, "fail-skill", "SKILL.md")); err != nil {
		t.Error("source SKILL.md should still exist after failure")
	}

	// State must not be mutated.
	if _, ok := st.Installed["fail-skill"]; ok {
		t.Error("state should not have fail-skill after install failure")
	}

	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failure, got %d", len(result.Failed))
	}
	if len(result.Adopted) != 0 {
		t.Errorf("expected 0 adopted, got %d", len(result.Adopted))
	}

	// Canonical store copy may exist (documented: left for retry).
	// No assertion on that either way.

	// Check that AdoptErrorMsg was emitted.
	foundErr := false
	for _, ev := range events {
		if _, ok := ev.(adopt.AdoptErrorMsg); ok {
			foundErr = true
		}
	}
	if !foundErr {
		t.Error("expected AdoptErrorMsg event")
	}
}

// ---------------------------------------------------------------------------
// TestApply — partial batch
// ---------------------------------------------------------------------------

func TestApply_PartialBatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srcDir := filepath.Join(home, "source-skills")

	makeCandidate := func(name, toolName string, installErr error) (adopt.Candidate, *mockTool) {
		content := "# " + name
		writeSkill(t, srcDir, name, content)
		mt := &mockTool{name: toolName, installErr: installErr}
		return adopt.Candidate{
			Name:      name,
			LocalPath: filepath.Join(srcDir, name),
			Targets:   []string{toolName},
			Hash:      blobSHA([]byte(content)),
		}, mt
	}

	cand1, tool1 := makeCandidate("skill-a", "tool1", nil)
	cand2, tool2 := makeCandidate("skill-b", "tool2", fmt.Errorf("boom"))
	cand3, tool3 := makeCandidate("skill-c", "tool3", nil)

	st := emptyState()
	adopter := &adopt.Adopter{
		State: st,
		Tools: []tools.Tool{tool1, tool2, tool3},
		Emit:  func(any) {},
	}

	result := adopter.Apply([]adopt.Candidate{cand1, cand2, cand3})

	if len(result.Adopted) != 2 {
		t.Errorf("adopted = %v, want 2", result.Adopted)
	}
	if len(result.Failed) != 1 {
		t.Errorf("failed count = %d, want 1", len(result.Failed))
	}
	if _, ok := result.Failed["skill-b"]; !ok {
		t.Error("skill-b should be in Failed")
	}
}

// ---------------------------------------------------------------------------
// TestResolve
// ---------------------------------------------------------------------------

func TestResolve_UnresolvedConflictsFiltered(t *testing.T) {
	plan := adopt.Plan{
		Conflicts: []adopt.Conflict{
			{Name: "conflict-skill", Unmanaged: adopt.Candidate{Name: "conflict-skill"}},
		},
	}
	out := adopt.Resolve(plan, nil)
	if len(out) != 0 {
		t.Errorf("expected 0 candidates after filtering unresolved, got %d", len(out))
	}
}

func TestResolve_DecisionOverwriteManaged(t *testing.T) {
	unmanaged := adopt.Candidate{Name: "foo", LocalPath: "/tmp/foo", Hash: "abc"}
	plan := adopt.Plan{
		Conflicts: []adopt.Conflict{
			{Name: "foo", Unmanaged: unmanaged},
		},
	}
	out := adopt.Resolve(plan, map[string]adopt.Decision{
		"foo": adopt.DecisionOverwriteManaged,
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].Name != "foo" {
		t.Errorf("name = %q", out[0].Name)
	}
}

func TestResolve_DecisionReplaceUnmanaged(t *testing.T) {
	unmanaged := adopt.Candidate{Name: "bar", LocalPath: "/tmp/bar", Hash: "def"}
	plan := adopt.Plan{
		Conflicts: []adopt.Conflict{
			{Name: "bar", Unmanaged: unmanaged},
		},
	}
	out := adopt.Resolve(plan, map[string]adopt.Decision{
		"bar": adopt.DecisionReplaceUnmanaged,
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	// We can't inspect reLinkOnly from the test package (unexported), but
	// we can verify the candidate is present and named correctly.
	if out[0].Name != "bar" {
		t.Errorf("name = %q", out[0].Name)
	}
}

func TestResolve_DecisionSkip(t *testing.T) {
	plan := adopt.Plan{
		Adopt: []adopt.Candidate{{Name: "clean"}},
		Conflicts: []adopt.Conflict{
			{Name: "conflict-skill"},
		},
	}
	out := adopt.Resolve(plan, map[string]adopt.Decision{
		"conflict-skill": adopt.DecisionSkip,
	})
	if len(out) != 1 || out[0].Name != "clean" {
		t.Errorf("expected only clean candidate, got %v", out)
	}
}

func TestResolve_CleanCandidatesPassThrough(t *testing.T) {
	plan := adopt.Plan{
		Adopt: []adopt.Candidate{
			{Name: "a"},
			{Name: "b"},
		},
	}
	out := adopt.Resolve(plan, nil)
	if len(out) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Integration test: adopting a non-empty real directory
// ---------------------------------------------------------------------------

// TestApply_RealDirectoryAdoption verifies that applyOne can convert a real
// (non-empty) skill directory at ~/.claude/skills/<name>/ into a symlink.
// This exercises the pre-remove step that fixes the ENOTEMPTY bug.
func TestApply_RealDirectoryAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a real (non-empty) skill directory at the Claude-facing path.
	claudeSkillsDir := filepath.Join(home, ".claude", "skills")
	skillDir := filepath.Join(claudeSkillsDir, "commit")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMDContent := "# commit\nUse conventional commits."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMDContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// Add a second file to prove non-empty directories work.
	if err := os.WriteFile(filepath.Join(skillDir, "notes.md"), []byte("extra notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build adopter with the real ClaudeTool.
	st := emptyState()
	adopter := &adopt.Adopter{
		State: st,
		Tools: []tools.Tool{tools.ClaudeTool{}},
		Emit:  func(any) {},
	}

	// Find candidates — expect exactly 1.
	candidates, conflicts, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}

	// Apply adoption.
	result := adopter.Apply(candidates)

	if len(result.Failed) != 0 {
		t.Fatalf("expected no failures, got: %v", result.Failed)
	}
	if len(result.Adopted) != 1 || result.Adopted[0] != "commit" {
		t.Errorf("adopted = %v, want [commit]", result.Adopted)
	}

	// The Claude-facing path must now be a symlink.
	claudeLink := filepath.Join(claudeSkillsDir, "commit")
	info, err := os.Lstat(claudeLink)
	if err != nil {
		t.Fatalf("lstat claude link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink at %s, got mode %v", claudeLink, info.Mode())
	}

	// The symlink must point into the canonical store.
	target, err := os.Readlink(claudeLink)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	storeSkillDir := filepath.Join(home, ".scribe", "skills", "commit")
	wantTarget := filepath.Join(storeSkillDir, "SKILL.md")
	if target != wantTarget {
		t.Errorf("symlink target = %q, want %q", target, wantTarget)
	}

	// Canonical store must have SKILL.md.
	if _, err := os.Stat(filepath.Join(storeSkillDir, "SKILL.md")); err != nil {
		t.Errorf("canonical SKILL.md missing: %v", err)
	}

	// State must record the adoption.
	installed, ok := st.Installed["commit"]
	if !ok {
		t.Fatal("commit not in state")
	}
	if installed.Origin != state.OriginLocal {
		t.Errorf("origin = %q, want %q", installed.Origin, state.OriginLocal)
	}
	if len(installed.Paths) == 0 {
		t.Error("Paths should be non-empty")
	}

	// Idempotency: second FindCandidates should return 0 candidates (already managed).
	candidates2, conflicts2, err := adopt.FindCandidates(st, adoptionCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates2) != 0 || len(conflicts2) != 0 {
		t.Errorf("second run: expected no candidates/conflicts, got candidates=%d conflicts=%d", len(candidates2), len(conflicts2))
	}
}
