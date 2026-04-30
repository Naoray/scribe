package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
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

func TestDefaultInitPackageMetaUsesCwdNameAndGitAuthor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-skills-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGitForInitTest(t, dir, "init")
	runGitForInitTest(t, dir, "config", "user.name", "Test Author")

	meta := defaultInitPackageMeta(dir)

	if meta.Name != "my-skills-repo" {
		t.Fatalf("Name = %q, want my-skills-repo", meta.Name)
	}
	if meta.Author != "Test Author" {
		t.Fatalf("Author = %q, want Test Author", meta.Author)
	}
}

func TestNewInitCommandHasForceFlag(t *testing.T) {
	cmd := newInitCommand()
	if cmd.Use != "init" {
		t.Fatalf("Use = %q, want init", cmd.Use)
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Fatal("init command missing --force flag")
	}
}

func TestRunInitNonTTYWritesJSONEnvelope(t *testing.T) {
	dir := t.TempDir()
	writeInitSkill(t, dir, "review", "---\nname: review\n---\n# Review\n")
	withInitWorkingDir(t, dir)

	cmd := newInitCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute init: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if env.Status != "ok" {
		t.Fatalf("status = %q, want ok", env.Status)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	pkg, ok := data["package"].(map[string]any)
	if !ok {
		t.Fatalf("data.package missing: %#v", data)
	}
	if pkg["name"] != filepath.Base(dir) {
		t.Fatalf("package.name = %v, want %s", pkg["name"], filepath.Base(dir))
	}
	if data["scribe_file"] != "scribe.yaml" {
		t.Fatalf("scribe_file = %v, want scribe.yaml", data["scribe_file"])
	}
	if _, err := os.Stat(filepath.Join(dir, "scribe.yaml")); err != nil {
		t.Fatalf("scribe.yaml not written: %v", err)
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

func runGitForInitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func withInitWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
