package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/state"
)

func TestReadSkillMetadata_FullFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: browse\nversion: 1.1.0\ndescription: Fast headless browser for QA testing.\n---\n\n# Browse\n\nContent here.\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Name != "browse" {
		t.Errorf("Name: got %q, want %q", meta.Name, "browse")
	}
	if meta.Version != "1.1.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "1.1.0")
	}
	if meta.Description != "Fast headless browser for QA testing." {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_NoVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: ascii\ndescription: ASCII diagram generator\n---\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "" {
		t.Errorf("Version should be empty, got %q", meta.Version)
	}
	if meta.Description != "ASCII diagram generator" {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# My Skill\n\nThis is the first paragraph.\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "" {
		t.Errorf("Version should be empty, got %q", meta.Version)
	}
	if meta.Description != "This is the first paragraph." {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_QuotedVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\nversion: \"3.0.0\"\ndescription: Quoted version test.\n---\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "3.0.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "3.0.0")
	}
}

func TestReadSkillMetadata_NoSkillMD(t *testing.T) {
	dir := t.TempDir()

	meta := readSkillMetadata(dir)

	if meta.Name != "" || meta.Version != "" || meta.Description != "" {
		t.Errorf("expected empty meta, got %+v", meta)
	}
}

func TestReadSkillMetadata_MultilineDescription(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\nversion: \"2.0.0\"\ndescription: |\n  A multiline description here.\n---\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "2.0.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "2.0.0")
	}
	if meta.Description != "A multiline description here." {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetaAuthor(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "deploy")
	os.MkdirAll(skill, 0o755)
	content := "---\nname: deploy\ndescription: Deploy to production\nmetadata:\n  version: \"2.0.0\"\n  author: krishan\n---\n\n# Deploy\n"
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644)

	meta := readSkillMetadata(skill)
	if meta.Author != "krishan" {
		t.Errorf("Author: got %q, want krishan", meta.Author)
	}
	if meta.Version != "2.0.0" {
		t.Errorf("Version: got %q, want 2.0.0", meta.Version)
	}
}

func TestReadSkillMetaTopLevelAuthor(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "review")
	os.MkdirAll(skill, 0o755)
	content := "---\nname: review\ndescription: Code review\nversion: \"1.0.0\"\nauthor: obra\n---\n"
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644)

	meta := readSkillMetadata(skill)
	if meta.Author != "obra" {
		t.Errorf("Author: got %q, want obra", meta.Author)
	}
}

func TestReadSkillMetaMetadataOverridesTopLevel(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "test")
	os.MkdirAll(skill, 0o755)
	content := "---\nname: test\nversion: \"1.0.0\"\nauthor: old-author\nmetadata:\n  version: \"2.0.0\"\n  author: new-author\n---\n"
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644)

	meta := readSkillMetadata(skill)
	if meta.Author != "new-author" {
		t.Errorf("Author: got %q, want new-author", meta.Author)
	}
	if meta.Version != "2.0.0" {
		t.Errorf("Version: got %q, want 2.0.0", meta.Version)
	}
}

func TestOnDiskSkipsReservedNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scribeSkills := filepath.Join(home, ".scribe", "skills")

	// Create reserved-name dirs with SKILL.md inside.
	for _, name := range []string{"versions", ".git"} {
		dir := filepath.Join(scribeSkills, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Reserved\n"), 0o644)
	}

	// Create a legit skill.
	legit := filepath.Join(scribeSkills, "legit-skill")
	os.MkdirAll(legit, 0o755)
	os.WriteFile(filepath.Join(legit, "SKILL.md"), []byte("# Legit\n"), 0o644)

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	skills, err := OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "legit-skill" {
		t.Errorf("expected legit-skill, got %q", skills[0].Name)
	}
}

func TestOnDiskModifiedDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scribeSkills := filepath.Join(home, ".scribe", "skills")
	skillDir := filepath.Join(scribeSkills, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill\n\nModified content.\n"), 0o644)

	// Compute the actual hash of the file on disk.
	actualHash, err := skillFileHash(skillDir)
	if err != nil {
		t.Fatal(err)
	}

	// Set a DIFFERENT installed_hash in state — should flag as modified.
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"my-skill": {
				InstalledHash: "different",
				Tools:         []string{"claude"},
				Revision:      1,
			},
		},
	}

	skills, err := OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if !skills[0].Modified {
		t.Error("expected Modified=true when installed_hash differs")
	}

	// Now set matching hash — should NOT be modified.
	st.Installed["my-skill"] = state.InstalledSkill{
		InstalledHash: actualHash,
		Tools:         []string{"claude"},
		Revision:      2,
	}

	skills, err = OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Modified {
		t.Error("expected Modified=false when installed_hash matches")
	}
	if skills[0].Revision != 2 {
		t.Errorf("expected Revision=2, got %d", skills[0].Revision)
	}
}

func TestOnDiskConflictDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scribeSkills := filepath.Join(home, ".scribe", "skills")
	skillDir := filepath.Join(scribeSkills, "conflict-skill")
	os.MkdirAll(skillDir, 0o755)

	content := "# Conflict Skill\n\n<<<<<<< local\nmy version\n=======\ntheir version\n>>>>>>> remote\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	skills, err := OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if !skills[0].Conflicted {
		t.Error("expected Conflicted=true for SKILL.md with merge conflict markers")
	}
}

func TestOnDiskFileSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create skill in scribe store (not already scanned — we only scan claude dir).
	scribeSkills := filepath.Join(home, ".scribe", "skills")
	skillDir := filepath.Join(scribeSkills, "symlinked-skill")
	os.MkdirAll(skillDir, 0o755)
	skillMD := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillMD, []byte("# Symlinked Skill\n\nDiscovered via file symlink.\n"), 0o644)

	// Create file symlink in claude skills dir pointing to SKILL.md (not the dir).
	claudeSkills := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(claudeSkills, 0o755)
	os.Symlink(skillMD, filepath.Join(claudeSkills, "symlinked-skill"))

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	skills, err := OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	// Should find exactly 1 skill (deduplicated: scribe store + claude symlink).
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "symlinked-skill" {
		t.Errorf("expected symlinked-skill, got %q", skills[0].Name)
	}
	if skills[0].LocalPath != skillDir {
		t.Errorf("expected LocalPath=%q, got %q", skillDir, skills[0].LocalPath)
	}
}

func TestHasConflictMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"with markers", "<<<<<<< local\nfoo\n=======\nbar\n>>>>>>> remote", true},
		{"without markers", "# Clean file\nno conflicts here", false},
		{"partial marker", "<<<<<< almost", false},
		{"marker in text", "See <<<<<<< HEAD for details", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasConflictMarkers([]byte(tt.content))
			if got != tt.want {
				t.Errorf("hasConflictMarkers(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestSkillFileHash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Test\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("ignored"), 0o644)

	hash, err := skillFileHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q", hash)
	}

	// Changing extra.txt should NOT change the skill file hash.
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("changed"), 0o644)
	hash2, err := skillFileHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if hash != hash2 {
		t.Errorf("skillFileHash should only hash SKILL.md: got %q then %q", hash, hash2)
	}

	// Changing SKILL.md SHOULD change the hash.
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Modified\n"), 0o644)
	hash3, err := skillFileHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if hash == hash3 {
		t.Error("skillFileHash should change when SKILL.md changes")
	}
}
