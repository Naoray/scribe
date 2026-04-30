package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPackageSkillsFindsNestedSkillFiles(t *testing.T) {
	dir := t.TempDir()
	writeInitSkill(t, dir, "review", "---\nname: code-review\n---\n# Review\n")
	writeInitSkill(t, dir, "deploy", "# Deploy\n")

	skills, err := discoverPackageSkills(dir)
	if err != nil {
		t.Fatalf("discoverPackageSkills: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("skills count = %d, want 2: %#v", len(skills), skills)
	}
	if skills[0].Name != "code-review" || skills[0].Path != "review" {
		t.Fatalf("skills[0] = %#v, want code-review/review", skills[0])
	}
	if skills[1].Name != "deploy" || skills[1].Path != "deploy" {
		t.Fatalf("skills[1] = %#v, want deploy/deploy", skills[1])
	}
}

func TestDiscoverPackageSkillsEmptyDirectory(t *testing.T) {
	skills, err := discoverPackageSkills(t.TempDir())
	if err != nil {
		t.Fatalf("discoverPackageSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("skills count = %d, want 0: %#v", len(skills), skills)
	}
}

func writeInitSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}
