package discovery

import (
	"os"
	"path/filepath"
	"testing"
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
