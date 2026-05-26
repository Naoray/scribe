package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/projectfile"
)

func TestProjectEditMutatesProjectFile(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		args    []string
		want    projectfile.ProjectFile
	}{
		{
			name:    "kit add is idempotent",
			initial: "kits: [core]\n",
			args:    []string{"kit", "add", "core", "deploy", "--no-sync"},
			want:    projectfile.ProjectFile{Kits: []string{"core", "deploy"}},
		},
		{
			name:    "kit remove is idempotent",
			initial: "kits: [core, deploy]\n",
			args:    []string{"kit", "remove", "core", "missing", "--no-sync"},
			want:    projectfile.ProjectFile{Kits: []string{"deploy"}},
		},
		{
			name:    "snippet add is idempotent",
			initial: "snippets: [editor]\n",
			args:    []string{"snippet", "add", "editor", "review", "--no-sync"},
			want:    projectfile.ProjectFile{Snippets: []string{"editor", "review"}},
		},
		{
			name:    "skill add appends to add list",
			initial: "add: [review]\n",
			args:    []string{"skill", "add", "review", "deploy", "--no-sync"},
			want:    projectfile.ProjectFile{Add: []string{"review", "deploy"}},
		},
		{
			name:    "skill remove deletes from add list",
			initial: "add: [review, deploy]\n",
			args:    []string{"skill", "remove", "review", "missing", "--no-sync"},
			want:    projectfile.ProjectFile{Add: []string{"deploy"}},
		},
		{
			name:    "skill remove block appends to remove list",
			initial: "add: [review]\nremove: [legacy]\n",
			args:    []string{"skill", "remove", "review", "legacy", "--block", "--no-sync"},
			want:    projectfile.ProjectFile{Add: []string{"review"}, Remove: []string{"legacy", "review"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			t.Setenv("HOME", home)
			mustChdir(t, project)
			projectPath := filepath.Join(project, projectfile.Filename)
			mustWriteProjectFile(t, projectPath, tt.initial)

			cmd := newProjectCommand()
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			got, err := projectfile.Load(projectPath)
			if err != nil {
				t.Fatalf("load project file: %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("ProjectFile = %#v, want %#v", *got, tt.want)
			}
		})
	}
}

func TestProjectEditAcceptsQualifiedSkillAndSnippetRefs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want projectfile.ProjectFile
	}{
		{
			name: "skill qualified refs",
			args: []string{"skill", "add", "registry:review", "owner/repo:deploy", "--no-sync"},
			want: projectfile.ProjectFile{Add: []string{"registry:review", "owner/repo:deploy"}},
		},
		{
			name: "snippet qualified refs",
			args: []string{"snippet", "add", "registry:commit", "owner/repo:review", "--no-sync"},
			want: projectfile.ProjectFile{Snippets: []string{"registry:commit", "owner/repo:review"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			t.Setenv("HOME", home)
			mustChdir(t, project)
			projectPath := filepath.Join(project, projectfile.Filename)
			mustWriteProjectFile(t, projectPath, "{}\n")

			cmd := newProjectCommand()
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			got, err := projectfile.Load(projectPath)
			if err != nil {
				t.Fatalf("load project file: %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("ProjectFile = %#v, want %#v", *got, tt.want)
			}
		})
	}
}

func TestProjectEditRejectsTraversalRefs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "skill traversal", args: []string{"skill", "add", "../review", "--no-sync"}, code: "SKILL_REF_INVALID"},
		{name: "skill leading slash", args: []string{"skill", "add", "/review", "--no-sync"}, code: "SKILL_REF_INVALID"},
		{name: "snippet traversal", args: []string{"snippet", "add", "owner/../repo:review", "--no-sync"}, code: "SNIPPET_REF_INVALID"},
		{name: "snippet slash without qualifier", args: []string{"snippet", "add", "owner/review", "--no-sync"}, code: "SNIPPET_REF_INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			t.Setenv("HOME", home)
			mustChdir(t, project)
			mustWriteProjectFile(t, filepath.Join(project, projectfile.Filename), "{}\n")

			cmd := newProjectCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("execute error = nil, want invalid ref")
			}
			if !clierrors.IsCode(err, tt.code) {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if got := clierrors.ExitCode(err); got != clierrors.ExitValid {
				t.Fatalf("exit = %d, want %d", got, clierrors.ExitValid)
			}
		})
	}
}

func TestProjectSnippetAddJSONReportsNotSynced(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	mustChdir(t, project)
	mustWriteProjectFile(t, filepath.Join(project, projectfile.Filename), "snippets: []\n")

	var stdout bytes.Buffer
	cmd := newProjectCommand()
	cmd.SetArgs([]string{"snippet", "add", "commit-discipline", "--json"})
	cmd.SetOut(&stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s", err, stdout.String())
	}
	var data projectEditOutput
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Synced {
		t.Fatalf("synced = true, want false because project sync does not project snippets")
	}
}

func TestProjectEditJSONEmptyListIsArray(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	mustChdir(t, project)
	projectPath := filepath.Join(project, projectfile.Filename)
	mustWriteProjectFile(t, projectPath, "add: [review]\n")

	var stdout bytes.Buffer
	cmd := newProjectCommand()
	cmd.SetArgs([]string{"skill", "remove", "review", "--no-sync", "--json"})
	cmd.SetOut(&stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var raw struct {
		Data struct {
			List json.RawMessage `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s", err, stdout.String())
	}
	if string(raw.Data.List) != "[]" {
		t.Fatalf("data.list = %s, want []", raw.Data.List)
	}
	projectData, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if bytes.Contains(projectData, []byte("null")) {
		t.Fatalf("project file contains null after removing last entry:\n%s", projectData)
	}
}

func TestProjectEditMissingProjectFileExitCode(t *testing.T) {
	project := t.TempDir()
	mustChdir(t, project)

	cmd := newProjectCommand()
	cmd.SetArgs([]string{"kit", "add", "core", "--no-sync"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("execute error = nil, want missing project file")
	}
	if got := clierrors.ExitCode(err); got != clierrors.ExitNotFound {
		t.Fatalf("exit = %d, want %d; err=%v", got, clierrors.ExitNotFound, err)
	}
}

func TestProjectEditNoSyncSkipsSync(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	mustChdir(t, project)
	mustWriteProjectFile(t, filepath.Join(project, projectfile.Filename), "kits: []\n")

	cmd := newProjectCommand()
	cmd.SetArgs([]string{"kit", "add", "missing-kit", "--no-sync"})
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute with --no-sync: %v", err)
	}

	cmd = newProjectCommand()
	cmd.SetArgs([]string{"kit", "add", "another-missing-kit"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("execute without --no-sync error = nil, want sync failure")
	}
	if !clierrors.IsCode(err, "PROJECT_KIT_NOT_FOUND") {
		t.Fatalf("error = %v, want PROJECT_KIT_NOT_FOUND", err)
	}
}
