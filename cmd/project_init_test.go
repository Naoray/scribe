package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	clischema "github.com/Naoray/scribe/internal/cli/schema"
	"github.com/Naoray/scribe/internal/projectfile"
)

func TestProjectInitCreatesProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newProjectInitCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if len(pf.Kits) != 0 {
		t.Fatalf("kits = %#v, want empty", pf.Kits)
	}
	if !strings.Contains(stdout.String(), "Initialized .scribe.yaml") {
		t.Fatalf("stdout = %q, want initialized message", stdout.String())
	}
}

func TestProjectInitForceOverwritesExistingProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := projectfile.Save(filepath.Join(dir, projectfile.Filename), &projectfile.ProjectFile{Kits: []string{"old"}}); err != nil {
		t.Fatalf("write existing project file: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--force", "--kits", "new"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --force: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "new" {
		t.Fatalf("kits = %q, want new", got)
	}
}

func TestProjectInitConflictsWhenProjectFileExistsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := projectfile.Save(filepath.Join(dir, projectfile.Filename), &projectfile.ProjectFile{Kits: []string{"old"}}); err != nil {
		t.Fatalf("write existing project file: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error for existing project file")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitConflict {
		t.Fatalf("exit code = %d, want %d; err=%v", got, clierrors.ExitConflict, err)
	}
}

func TestProjectInitDoesNotGitignoreProjectFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	cmd := newProjectInitCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore exists after project init: %v", err)
	}
	if strings.Contains(stdout.String(), ".gitignore") {
		t.Fatalf("stdout = %q, want no gitignore message", stdout.String())
	}
}

func TestProjectInitLeavesExistingGitignoreAlone(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.scribe.yaml\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newProjectInitCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(data); got != "node_modules\n.scribe.yaml\n" {
		t.Fatalf(".gitignore = %q, want unchanged", got)
	}
}

func TestProjectInitJSONEnvelope(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"project", "init", "--json"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --json: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON envelope: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var data struct {
		Kits        []string `json:"kits"`
		ProjectFile string   `json:"project_file"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.ProjectFile != ".scribe.yaml" {
		t.Fatalf("project_file = %q, want .scribe.yaml", data.ProjectFile)
	}
	if data.Kits == nil {
		t.Fatal("kits = nil, want empty array")
	}
}

func TestProjectInitKitsFlagUsesProvidedValues(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".scribe", "kits"), 0o755); err != nil {
		t.Fatalf("mkdir kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".scribe", "kits", "go.yaml"), []byte("name: go\nskills: []\n"), 0o644); err != nil {
		t.Fatalf("write kit: %v", err)
	}
	t.Setenv("HOME", home)
	withProjectInitWorkingDir(t, dir)

	cmd := newProjectInitCommand()
	cmd.SetArgs([]string{"--kits", "go,unknown"})
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute project init --kits: %v", err)
	}

	pf, err := projectfile.Load(filepath.Join(dir, projectfile.Filename))
	if err != nil {
		t.Fatalf("load project file: %v", err)
	}
	if got := strings.Join(pf.Kits, ","); got != "go,unknown" {
		t.Fatalf("kits = %q, want go,unknown", got)
	}
	if !strings.Contains(stderr.String(), "warning: unknown kit unknown") {
		t.Fatalf("stderr = %q, want unknown kit warning", stderr.String())
	}
}

func TestProjectInitOutputSchemaRegistered(t *testing.T) {
	if _, ok := clischema.Get("scribe project init"); !ok {
		t.Fatal("missing output schema for scribe project init")
	}
}

func withProjectInitWorkingDir(t *testing.T, dir string) {
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
