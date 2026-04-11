package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestLoadMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if s.Installed == nil {
		t.Error("expected non-nil Installed map")
	}
	if len(s.Installed) != 0 {
		t.Errorf("expected empty Installed, got %d entries", len(s.Installed))
	}
	if s.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion=3, got %d", s.SchemaVersion)
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordSync()
	s.RecordInstall("gstack", state.InstalledSkill{
		Revision:      1,
		InstalledHash: "abc123def456",
		Sources: []state.SkillSource{{
			Registry: "garrytan/gstack",
			Ref:      "v0.12.9.0",
		}},
		Tools: []string{"claude"},
		Paths: []string{"/Users/krishan/.claude/skills/gstack/"},
	})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	if loaded.LastSync.IsZero() {
		t.Error("expected LastSync to be set")
	}
	if loaded.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion=3, got %d", loaded.SchemaVersion)
	}

	skill, ok := loaded.Installed["gstack"]
	if !ok {
		t.Fatal("gstack not found in Installed")
	}
	if skill.Revision != 1 {
		t.Errorf("revision: got %d", skill.Revision)
	}
	if skill.InstalledHash != "abc123def456" {
		t.Errorf("installed_hash: got %q", skill.InstalledHash)
	}
	if len(skill.Sources) != 1 || skill.Sources[0].Registry != "garrytan/gstack" {
		t.Errorf("sources: got %v", skill.Sources)
	}
	if skill.InstalledAt.IsZero() {
		t.Error("expected InstalledAt to be set")
	}
}

func TestAtomicWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, _ := state.Load()
	s.RecordSync()
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmpPath := filepath.Join(home, ".scribe", "state.json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after save")
	}
}

func TestDisplayVersion(t *testing.T) {
	tests := []struct {
		name     string
		skill    state.InstalledSkill
		expected string
	}{
		{
			name:     "revision 1",
			skill:    state.InstalledSkill{Revision: 1},
			expected: "rev 1",
		},
		{
			name:     "revision 5",
			skill:    state.InstalledSkill{Revision: 5},
			expected: "rev 5",
		},
		{
			name:     "revision 0",
			skill:    state.InstalledSkill{Revision: 0},
			expected: "rev 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.skill.DisplayVersion()
			if got != tt.expected {
				t.Errorf("DisplayVersion(): got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestToolsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("deploy", state.InstalledSkill{
		Revision: 1,
		Sources: []state.SkillSource{{
			Registry: "org/deploy",
			Ref:      "v1.0.0",
		}},
		Tools: []string{"claude"},
	})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _ := state.Load()
	skill := loaded.Installed["deploy"]
	if len(skill.Tools) != 1 || skill.Tools[0] != "claude" {
		t.Errorf("Tools: got %v", skill.Tools)
	}
}

func TestPackageFieldsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	st.RecordInstall("gstack", state.InstalledSkill{
		Revision:      3,
		InstalledHash: "deadbeefcafe",
		Sources: []state.SkillSource{{
			Registry: "garrytan/gstack",
			Ref:      "main",
			LastSHA:  "abc123",
		}},
		Tools:      []string{"claude"},
		Paths:      []string{"/home/user/.claude/skills/gstack"},
		Type:       "package",
		InstallCmd: "git clone ...",
		UpdateCmd:  "cd ~/.claude/skills/gstack && git pull",
		CmdHash:    "deadbeef",
		Approval:   "approved",
	})

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	st2, err := state.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	got := st2.Installed["gstack"]
	if got.Type != "package" {
		t.Errorf("Type: got %q, want package", got.Type)
	}
	if got.InstallCmd != "git clone ..." {
		t.Errorf("InstallCmd: got %q", got.InstallCmd)
	}
	if got.UpdateCmd != "cd ~/.claude/skills/gstack && git pull" {
		t.Errorf("UpdateCmd: got %q", got.UpdateCmd)
	}
	if got.CmdHash != "deadbeef" {
		t.Errorf("CmdHash: got %q", got.CmdHash)
	}
	if got.Approval != "approved" {
		t.Errorf("Approval: got %q", got.Approval)
	}
	if got.Revision != 3 {
		t.Errorf("Revision: got %d, want 3", got.Revision)
	}
	if got.InstalledHash != "deadbeefcafe" {
		t.Errorf("InstalledHash: got %q", got.InstalledHash)
	}
}

func TestRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("gstack", state.InstalledSkill{
		Revision:    1,
		InstalledAt: time.Now(),
	})
	s.Remove("gstack")

	if _, ok := s.Installed["gstack"]; ok {
		t.Error("expected gstack to be removed")
	}
}

// --- Migration tests ---

func TestMigrationNamespacesKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a legacy state.json with bare keys and Registries field (pre-v2).
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"gstack": {
				"version": "v0.12.9.0",
				"source": "github:garrytan/gstack@v0.12.9.0",
				"installed_at": "2026-01-01T00:00:00Z",
				"targets": ["claude"],
				"paths": ["/Users/test/.claude/skills/gstack/"],
				"registries": ["ArtistfyHQ/team-skills"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Migration 4 should convert qualified keys to bare names.
	// "gstack" → namespaced to "ArtistfyHQ-team-skills/gstack" by migration 3 → bare "gstack" by migration 4.
	if _, ok := s.Installed["gstack"]; !ok {
		t.Errorf("expected bare key 'gstack', got keys: %v", installedKeys(s))
	}

	skill := s.Installed["gstack"]
	if len(skill.Sources) != 1 {
		t.Fatalf("expected sources to be preserved by v3 migration, got %v", skill.Sources)
	}
	if skill.Sources[0].Registry != "garrytan/gstack" || skill.Sources[0].Ref != "v0.12.9.0" {
		t.Errorf("unexpected migrated sources: %v", skill.Sources)
	}

	// Targets should be migrated to Tools.
	if len(skill.Tools) != 1 || skill.Tools[0] != "claude" {
		t.Errorf("expected Tools=[claude], got %v", skill.Tools)
	}

	// Revision should be set to 1.
	if skill.Revision != 1 {
		t.Errorf("expected Revision=1, got %d", skill.Revision)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion=3, got %d", s.SchemaVersion)
	}
}

func TestStateMigrateTargetsToTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write legacy state with "targets" field.
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude", "cursor"],
				"paths": ["/Users/test/.claude/skills/deploy"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Bare key "deploy" → "local/deploy" (migration 3) → "deploy" (migration 4 strips prefix)
	skill := s.Installed["deploy"]
	if len(skill.Tools) != 2 {
		t.Fatalf("expected 2 tools after migration, got %d", len(skill.Tools))
	}
	if skill.Tools[0] != "claude" || skill.Tools[1] != "cursor" {
		t.Errorf("tools: got %v", skill.Tools)
	}
}

func TestStatePromoteLastSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.LastSync.IsZero() {
		t.Error("expected LastSync to be promoted from team.last_sync")
	}
	expected := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	if !s.LastSync.Equal(expected) {
		t.Errorf("LastSync: got %v, want %v", s.LastSync, expected)
	}
}

func TestStateNamespaceKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {"last_sync": "2026-03-15T10:00:00Z"},
		"installed": {
			"deploy": {
				"version": "v1.0.0",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			},
			"recap": {
				"version": "v2.0.0",
				"source": "github:ArtistfyHQ/team-skills@v2.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": [],
				"registries": ["ArtistfyHQ/team-skills"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Migration 4 strips namespace prefixes to bare keys.
	if _, ok := s.Installed["deploy"]; !ok {
		t.Errorf("expected bare key 'deploy', got keys: %v", installedKeys(s))
	}
	if _, ok := s.Installed["recap"]; !ok {
		t.Errorf("expected bare key 'recap', got keys: %v", installedKeys(s))
	}

	// Old namespaced keys should be gone.
	if _, ok := s.Installed["ArtistfyHQ-team-skills/deploy"]; ok {
		t.Error("namespaced key should have been converted to bare key")
	}
}

func TestStateNamespaceKeysNoRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"my-local-skill": {
				"version": "v1.0.0",
				"source": "",
				"installed_at": "2026-03-10T12:00:00Z",
				"targets": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Migration 4 strips "local/" prefix to bare key.
	if _, ok := s.Installed["my-local-skill"]; !ok {
		t.Errorf("expected bare key 'my-local-skill', got keys: %v", installedKeys(s))
	}
}

// TestStateMigrateV2ToV3 verifies that a v2 state (bare keys, populated
// Sources) gets upgraded to v3 with both sources and non-source fields
// preserved.
func TestStateMigrateV2ToV3(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"schema_version": 2,
		"last_sync": "2026-03-15T10:00:00Z",
		"installed": {
			"deploy": {
				"revision": 3,
				"installed_hash": "abc123",
				"sources": [{"registry": "ArtistfyHQ/team-skills", "ref": "v1.0.0", "last_sha": "def456", "last_synced": "2026-03-15T10:00:00Z"}],
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["claude"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, ok := s.Installed["deploy"]; !ok {
		t.Errorf("expected key 'deploy', got keys: %v", installedKeys(s))
	}
	if len(s.Installed) != 1 {
		t.Errorf("expected 1 installed skill, got %d", len(s.Installed))
	}

	skill := s.Installed["deploy"]
	if skill.Revision != 3 {
		t.Errorf("expected Revision=3, got %d", skill.Revision)
	}
	if skill.InstalledHash != "abc123" {
		t.Errorf("expected InstalledHash=abc123, got %q", skill.InstalledHash)
	}
	if len(skill.Sources) != 1 {
		t.Fatalf("expected sources preserved by v3 migration, got %v", skill.Sources)
	}
	if skill.Sources[0].Registry != "ArtistfyHQ/team-skills" || skill.Sources[0].LastSHA != "def456" {
		t.Errorf("unexpected migrated sources: %v", skill.Sources)
	}
	if s.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion=3, got %d", s.SchemaVersion)
	}
}

func TestMigrationSchemaV2(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	// Full v1 state with qualified keys, old fields.
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"last_sync": "2026-03-15T10:00:00Z",
		"installed": {
			"ArtistfyHQ-team-skills/deploy": {
				"version": "v1.0.0",
				"commit_sha": "abc123def456789",
				"source": "github:ArtistfyHQ/team-skills@v1.0.0",
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["claude"],
				"paths": ["/test/path"]
			},
			"local/my-tool": {
				"version": "v2.0.0",
				"source": "",
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["cursor"],
				"paths": []
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion=3, got %d", s.SchemaVersion)
	}

	// Qualified key should become bare.
	if _, ok := s.Installed["deploy"]; !ok {
		t.Errorf("expected bare key 'deploy', got keys: %v", installedKeys(s))
	}
	if _, ok := s.Installed["ArtistfyHQ-team-skills/deploy"]; ok {
		t.Error("qualified key should have been converted to bare")
	}

	// "local/my-tool" should become "my-tool"
	if _, ok := s.Installed["my-tool"]; !ok {
		t.Errorf("expected bare key 'my-tool', got keys: %v", installedKeys(s))
	}

	// v3 migration preserves sources gathered during migration.
	deploy := s.Installed["deploy"]
	if len(deploy.Sources) != 1 {
		t.Fatalf("expected deploy sources preserved, got %v", deploy.Sources)
	}
	if deploy.Sources[0].Registry != "ArtistfyHQ/team-skills" || deploy.Sources[0].Ref != "v1.0.0" {
		t.Errorf("unexpected deploy sources: %v", deploy.Sources)
	}
	if deploy.Revision != 1 {
		t.Errorf("expected Revision=1, got %d", deploy.Revision)
	}

	myTool := s.Installed["my-tool"]
	if len(myTool.Sources) != 0 {
		t.Errorf("expected my-tool sources empty, got %v", myTool.Sources)
	}
	if myTool.Revision != 1 {
		t.Errorf("expected Revision=1, got %d", myTool.Revision)
	}
}

// TestMigrationPreservesRevisionAndHash verifies v2→v3 migration preserves
// both source metadata and non-source fields.
func TestMigrationPreservesRevisionAndHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"schema_version": 2,
		"last_sync": "2026-04-01T00:00:00Z",
		"installed": {
			"gstack": {
				"revision": 5,
				"installed_hash": "sha256hash",
				"sources": [{"registry": "garrytan/gstack", "ref": "main", "last_sha": "commit123", "last_synced": "2026-04-01T00:00:00Z"}],
				"installed_at": "2026-03-01T00:00:00Z",
				"tools": ["claude"],
				"paths": ["/test"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("SchemaVersion: got %d, want 3", s.SchemaVersion)
	}
	skill := s.Installed["gstack"]
	if skill.Revision != 5 {
		t.Errorf("Revision: got %d, want 5", skill.Revision)
	}
	if skill.InstalledHash != "sha256hash" {
		t.Errorf("InstalledHash: got %q", skill.InstalledHash)
	}
	if len(skill.Sources) != 1 {
		t.Fatalf("Sources: expected preserve on v3 migration, got %v", skill.Sources)
	}
	if skill.Sources[0].Registry != "garrytan/gstack" || skill.Sources[0].LastSHA != "commit123" {
		t.Errorf("unexpected preserved sources: %v", skill.Sources)
	}
}

func TestParseSourceString(t *testing.T) {
	// parseSourceString is unexported, so test via migration behavior.
	// We test the actual function indirectly through the migration.
	// For direct testing, we'd need to export it or use a test helper.
	// Instead, verify via a migration that parses the source string.
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	tests := []struct {
		name         string
		source       string
		wantRegistry string
		wantRef      string
	}{
		{
			name:         "standard github source",
			source:       "github:owner/repo@main",
			wantRegistry: "owner/repo",
			wantRef:      "main",
		},
		{
			name:         "tag ref",
			source:       "github:ArtistfyHQ/team-skills@v1.2.0",
			wantRegistry: "ArtistfyHQ/team-skills",
			wantRef:      "v1.2.0",
		},
		{
			name:         "empty source",
			source:       "",
			wantRegistry: "",
			wantRef:      "",
		},
		{
			name:         "no github prefix",
			source:       "owner/repo@main",
			wantRegistry: "",
			wantRef:      "",
		},
		{
			name:         "no ref",
			source:       "github:owner/repo",
			wantRegistry: "",
			wantRef:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write a state file with this source and verify parsed sources
			// survive migration when the source string is valid.
			_ = tt
			os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
				"installed": {
					"test-skill": {
						"version": "v1.0.0",
						"source": "`+tt.source+`",
						"installed_at": "2026-01-01T00:00:00Z",
						"tools": ["claude"],
						"paths": []
					}
				}
			}`), 0o644)

			s, err := state.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			skill, ok := s.Installed["test-skill"]
			if !ok {
				t.Fatal("test-skill not found after migration")
			}
			if tt.wantRegistry == "" {
				if len(skill.Sources) != 0 {
					t.Errorf("expected no sources for %q, got %v", tt.source, skill.Sources)
				}
				return
			}
			if len(skill.Sources) != 1 {
				t.Fatalf("expected one source for %q, got %v", tt.source, skill.Sources)
			}
			if skill.Sources[0].Registry != tt.wantRegistry || skill.Sources[0].Ref != tt.wantRef {
				t.Errorf("source mismatch: got %v, want registry=%q ref=%q", skill.Sources, tt.wantRegistry, tt.wantRef)
			}
		})
	}
}

func TestStateToolsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("deploy", state.InstalledSkill{
		Revision: 1,
		Sources: []state.SkillSource{{
			Registry: "ArtistfyHQ/team-skills",
			Ref:      "v1.0.0",
		}},
		Tools: []string{"claude", "cursor"},
		Paths: []string{"/test"},
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := loaded.Installed["deploy"]
	if len(skill.Tools) != 2 || skill.Tools[0] != "claude" {
		t.Errorf("Tools round-trip: got %v", skill.Tools)
	}
}

func TestMigrationBareKeyCollisionCollapses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)

	// Two qualified keys collapse to the same bare name "deploy".
	// Schema v3 preserves merged sources while the newer entry wins as base.
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"installed": {
			"org-a/deploy": {
				"version": "v2.0.0",
				"source": "github:org-a/skills@v2.0.0",
				"installed_at": "2026-04-01T00:00:00Z",
				"tools": ["claude"],
				"paths": ["/test/a"],
				"registries": ["org-a/skills"]
			},
			"org-b/deploy": {
				"version": "v1.0.0",
				"source": "github:org-b/tools@v1.0.0",
				"installed_at": "2026-03-01T00:00:00Z",
				"tools": ["cursor"],
				"paths": ["/test/b"],
				"registries": ["org-b/tools"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("expected SchemaVersion 3, got %d", s.SchemaVersion)
	}

	// Should have exactly one "deploy" key, not two.
	if len(s.Installed) != 1 {
		t.Fatalf("expected 1 installed skill, got %d: %v", len(s.Installed), installedKeys(s))
	}

	skill, ok := s.Installed["deploy"]
	if !ok {
		t.Fatalf("expected bare key 'deploy', got keys: %v", installedKeys(s))
	}

	if len(skill.Sources) != 2 {
		t.Fatalf("expected merged sources preserved in v3, got %d: %v", len(skill.Sources), skill.Sources)
	}

	// Newer entry (org-a, 2026-04-01) should win as the base.
	if len(skill.Paths) == 0 || skill.Paths[0] != "/test/a" {
		t.Errorf("expected newer entry (org-a) to win, got paths %v", skill.Paths)
	}
}

// installedKeys is a test helper that returns the keys of the Installed map.
func installedKeys(s *state.State) []string {
	keys := make([]string, 0, len(s.Installed))
	for k := range s.Installed {
		keys = append(keys, k)
	}
	return keys
}

// TestMigrationSchemaV3PreservesSources verifies that loading a v2 state keeps
// SkillSource entries intact so the first sync after upgrade can refresh
// metadata in place instead of forcing reinstalls.
func TestMigrationSchemaV3PreservesSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"schema_version": 2,
		"last_sync": "2026-04-01T00:00:00Z",
		"installed": {
			"xray": {
				"revision": 3,
				"installed_hash": "contentabc",
				"sources": [{"registry": "Artistfy/hq", "ref": "main", "last_sha": "commit-xyz", "last_synced": "2026-04-01T00:00:00Z"}],
				"installed_at": "2026-03-10T12:00:00Z",
				"tools": ["claude"],
				"paths": ["/test/xray"]
			},
			"caveman": {
				"revision": 1,
				"sources": [{"registry": "JuliusBrussee/caveman", "ref": "main", "last_sha": "commit-pkg", "last_synced": "2026-03-20T12:00:00Z"}],
				"installed_at": "2026-03-20T12:00:00Z",
				"tools": ["claude"],
				"type": "package",
				"install_cmd": "claude plugin install caveman",
				"approval": "approved"
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("SchemaVersion: got %d, want 3", s.SchemaVersion)
	}

	xray, ok := s.Installed["xray"]
	if !ok {
		t.Fatal("xray not in Installed after migration")
	}
	if len(xray.Sources) != 1 {
		t.Fatalf("xray sources: got %v, want preserved source", xray.Sources)
	}
	if xray.Sources[0].Registry != "Artistfy/hq" || xray.Sources[0].LastSHA != "commit-xyz" {
		t.Errorf("xray sources: got %v", xray.Sources)
	}
	// Non-source fields must be preserved so the skill is still recognised.
	if xray.Revision != 3 {
		t.Errorf("xray revision: got %d, want 3", xray.Revision)
	}
	if xray.InstalledHash != "contentabc" {
		t.Errorf("xray installed_hash: got %q, want contentabc", xray.InstalledHash)
	}
	if len(xray.Tools) != 1 || xray.Tools[0] != "claude" {
		t.Errorf("xray tools: got %v", xray.Tools)
	}

	caveman, ok := s.Installed["caveman"]
	if !ok {
		t.Fatal("caveman not in Installed after migration")
	}
	if len(caveman.Sources) != 1 {
		t.Fatalf("caveman sources: got %v, want preserved source", caveman.Sources)
	}
	if caveman.Sources[0].Registry != "JuliusBrussee/caveman" || caveman.Sources[0].LastSHA != "commit-pkg" {
		t.Errorf("caveman sources: got %v", caveman.Sources)
	}
	// Package-specific fields preserved.
	if caveman.Type != "package" {
		t.Errorf("caveman type: got %q, want package", caveman.Type)
	}
	if caveman.Approval != "approved" {
		t.Errorf("caveman approval: got %q, want approved", caveman.Approval)
	}
}

// TestMigrationSchemaV3Idempotent verifies that a v3 state passes through
// unchanged — sources are preserved, schema stays 3.
func TestMigrationSchemaV3Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"schema_version": 3,
		"last_sync": "2026-04-11T00:00:00Z",
		"installed": {
			"xray": {
				"revision": 4,
				"installed_hash": "contentdef",
				"sources": [{"registry": "Artistfy/hq", "ref": "main", "last_sha": "blob-abc", "last_synced": "2026-04-11T00:00:00Z"}],
				"installed_at": "2026-04-11T00:00:00Z",
				"tools": ["claude"],
				"paths": ["/test/xray"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.SchemaVersion != 3 {
		t.Errorf("SchemaVersion: got %d, want 3", s.SchemaVersion)
	}
	xray := s.Installed["xray"]
	if len(xray.Sources) != 1 {
		t.Fatalf("xray sources: got %d, want 1 (preserved on v3 passthrough)", len(xray.Sources))
	}
	if xray.Sources[0].LastSHA != "blob-abc" {
		t.Errorf("xray LastSHA: got %q, want blob-abc", xray.Sources[0].LastSHA)
	}
}
