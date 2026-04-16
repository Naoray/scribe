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

func TestOnDiskCodexSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scribeSkills := filepath.Join(home, ".scribe", "skills")
	skillDir := filepath.Join(scribeSkills, "codex-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Codex Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	codexSkills := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(codexSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillDir, filepath.Join(codexSkills, "codex-skill")); err != nil {
		t.Fatal(err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	skills, err := OnDisk(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "codex-skill" {
		t.Errorf("expected codex-skill, got %q", skills[0].Name)
	}
}

func TestOnDiskManagedField(t *testing.T) {
	t.Run("canonical store entry state has it", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		skillDir := filepath.Join(home, ".scribe", "skills", "foo")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Foo\n"), 0o644)

		st := &state.State{Installed: map[string]state.InstalledSkill{
			"foo": {Tools: []string{"claude"}, Revision: 1},
		}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		if len(skills) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(skills))
		}
		if !skills[0].Managed {
			t.Error("expected Managed=true for canonical store entry tracked in state")
		}
	})

	t.Run("dedup prefers canonical store over tool-facing symlink", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		storeDir := filepath.Join(home, ".scribe", "skills", "bar")
		os.MkdirAll(storeDir, 0o755)
		os.WriteFile(filepath.Join(storeDir, "SKILL.md"), []byte("# Bar\n"), 0o644)

		claudeSkills := filepath.Join(home, ".claude", "skills")
		os.MkdirAll(claudeSkills, 0o755)
		os.Symlink(storeDir, filepath.Join(claudeSkills, "bar"))

		st := &state.State{Installed: map[string]state.InstalledSkill{
			"bar": {Tools: []string{"claude"}, Revision: 1},
		}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		// Store scan finds "bar" first; claude-dir symlink pass skips it as a duplicate.
			// Result: exactly one entry, Managed=true because state tracks it.
		var found *Skill
		for i := range skills {
			if skills[i].Name == "bar" {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			t.Fatal("bar not found in results")
		}
		if !found.Managed {
			t.Error("expected Managed=true for symlink into store tracked in state")
		}
	})

	t.Run("unmanaged tool-facing entry not in state", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		claudeSkills := filepath.Join(home, ".claude", "skills", "baz")
		os.MkdirAll(claudeSkills, 0o755)
		os.WriteFile(filepath.Join(claudeSkills, "SKILL.md"), []byte("# Baz\n"), 0o644)

		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		if len(skills) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(skills))
		}
		if skills[0].Managed {
			t.Error("expected Managed=false for plain tool-dir entry not in state")
		}
	})

	t.Run("state orphan no LocalPath", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		st := &state.State{Installed: map[string]state.InstalledSkill{
			"ghost": {Tools: []string{"claude"}, Revision: 3},
		}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		var found *Skill
		for i := range skills {
			if skills[i].Name == "ghost" {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			t.Fatal("ghost orphan not found in results")
		}
		if found.LocalPath != "" {
			t.Errorf("expected empty LocalPath for orphan, got %q", found.LocalPath)
		}
		if !found.Managed {
			t.Error("expected Managed=true for state-orphan entry")
		}
	})
}

func TestOnDiskPluginCache(t *testing.T) {
	t.Run("discovers skill from claude plugin cache", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		// Mimic the Claude Code plugin layout for caveman:
		// ~/.claude/plugins/cache/<plugin>/<name>/<hash>/skills/<skill>/SKILL.md
		pluginSkill := filepath.Join(home, ".claude", "plugins", "cache", "caveman", "caveman", "abc123", "skills", "caveman")
		if err := os.MkdirAll(pluginSkill, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: caveman\ndescription: Ultra-compressed mode\n---\n\n# Caveman\n"
		if err := os.WriteFile(filepath.Join(pluginSkill, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		// State tracks caveman as a plugin-installed package with no symlinks.
		st := &state.State{Installed: map[string]state.InstalledSkill{
			"caveman": {Type: "package", Revision: 1},
		}}

		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}

		var found *Skill
		for i := range skills {
			if skills[i].Name == "caveman" {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("caveman not discovered from plugin cache; got %+v", skills)
		}
		if found.LocalPath != pluginSkill {
			t.Errorf("LocalPath: got %q, want %q", found.LocalPath, pluginSkill)
		}
	})

	t.Run("dedups by frontmatter name across alternate plugin layouts", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		base := filepath.Join(home, ".claude", "plugins", "cache", "caveman", "caveman", "abc123")
		// Three real layouts caveman ships at the same time.
		for _, sub := range []string{
			filepath.Join("caveman"),
			filepath.Join("skills", "caveman"),
			filepath.Join("plugins", "caveman", "skills", "caveman"),
		} {
			dir := filepath.Join(base, sub)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			content := "---\nname: caveman\ndescription: dup\n---\n"
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}

		count := 0
		for _, sk := range skills {
			if sk.Name == "caveman" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 1 caveman entry, got %d", count)
		}
	})

	t.Run("scribe store wins over plugin cache", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		storeDir := filepath.Join(home, ".scribe", "skills", "caveman")
		os.MkdirAll(storeDir, 0o755)
		os.WriteFile(filepath.Join(storeDir, "SKILL.md"), []byte("---\nname: caveman\n---\n# from store\n"), 0o644)

		pluginDir := filepath.Join(home, ".claude", "plugins", "cache", "caveman", "caveman", "abc", "skills", "caveman")
		os.MkdirAll(pluginDir, 0o755)
		os.WriteFile(filepath.Join(pluginDir, "SKILL.md"), []byte("---\nname: caveman\n---\n# from plugin\n"), 0o644)

		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		var found *Skill
		for i := range skills {
			if skills[i].Name == "caveman" {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			t.Fatal("caveman not found")
		}
		if found.LocalPath != storeDir {
			t.Errorf("expected scribe store to win; LocalPath=%q want=%q", found.LocalPath, storeDir)
		}
	})

	t.Run("skips temp_git staging dirs", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		stale := filepath.Join(home, ".claude", "plugins", "cache", "temp_git_999", "skills", "ghost")
		os.MkdirAll(stale, 0o755)
		os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("---\nname: ghost\n---\n"), 0o644)

		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills, err := OnDisk(st)
		if err != nil {
			t.Fatal(err)
		}
		for _, sk := range skills {
			if sk.Name == "ghost" {
				t.Fatalf("ghost from temp_git_* dir should be skipped, got %+v", sk)
			}
		}
	})
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
