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

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"1.0.0"`, "1.0.0"},
		{`'browse'`, "browse"},
		{"no-quotes", "no-quotes"},
		{`""`, ""},
		{`"mismatched'`, `"mismatched'`},
		{"a", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripQuotes(tt.input)
			if got != tt.want {
				t.Errorf("stripQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\nversion: 2.0.0\ndescription: |\n  A multiline description here.\n---\n"), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "2.0.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "2.0.0")
	}
	if meta.Description != "A multiline description here." {
		t.Errorf("Description: got %q", meta.Description)
	}
}
