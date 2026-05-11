package source_test

import (
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/source"
)

func TestParseSourceArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		raw  string
		want source.SourceSpec
		key  string
	}{
		{
			name: "github shorthand",
			raw:  "Owner/Repo",
			want: source.SourceSpec{Type: source.SourceGitHub, Repo: "Owner/Repo", URL: "https://github.com/Owner/Repo", Host: "github.com"},
			key:  "github:owner/repo",
		},
		{
			name: "github repo url",
			raw:  "https://github.com/owner/repo.git",
			want: source.SourceSpec{Type: source.SourceGitHub, Repo: "owner/repo", URL: "https://github.com/owner/repo", Host: "github.com"},
			key:  "github:owner/repo",
		},
		{
			name: "github tree url",
			raw:  "https://github.com/owner/repo/tree/main/skills",
			want: source.SourceSpec{Type: source.SourceGitHub, Repo: "owner/repo", URL: "https://github.com/owner/repo", Ref: "main", Path: "skills", Host: "github.com"},
			key:  "github:owner/repo:skills",
		},
		{
			name: "gitlab repo url",
			raw:  "https://gitlab.com/group/project",
			want: source.SourceSpec{Type: source.SourceGitLab, Repo: "group/project", URL: "https://gitlab.com/group/project", Host: "gitlab.com"},
			key:  "gitlab:gitlab.com/group/project",
		},
		{
			name: "gitlab tree url",
			raw:  "https://gitlab.com/group/subgroup/project/-/tree/main/skills",
			want: source.SourceSpec{Type: source.SourceGitLab, Repo: "group/subgroup/project", URL: "https://gitlab.com/group/subgroup/project", Ref: "main", Path: "skills", Host: "gitlab.com"},
			key:  "gitlab:gitlab.com/group/subgroup/project:skills",
		},
		{
			name: "arbitrary git url",
			raw:  "https://example.com/org/skills.git",
			want: source.SourceSpec{Type: source.SourceGit, URL: "https://example.com/org/skills.git"},
			key:  "git:https://example.com/org/skills.git",
		},
		{
			name: "ssh git url",
			raw:  "git+ssh://git@example.com/org/skills.git",
			want: source.SourceSpec{Type: source.SourceGit, URL: "ssh://git@example.com/org/skills.git"},
			key:  "git:ssh://git@example.com/org/skills.git",
		},
		{
			name: "absolute local path",
			raw:  filepath.Join(home, "skills"),
			want: source.SourceSpec{Type: source.SourceLocal, Path: filepath.Join(home, "skills")},
			key:  "local:" + filepath.Join(home, "skills"),
		},
		{
			name: "tilde local path",
			raw:  "~/skills",
			want: source.SourceSpec{Type: source.SourceLocal, Path: filepath.Join(home, "skills")},
			key:  "local:" + filepath.Join(home, "skills"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := source.ParseSourceArg(tt.raw)
			if err != nil {
				t.Fatalf("ParseSourceArg: %v", err)
			}
			if got != tt.want {
				t.Fatalf("spec = %#v, want %#v", got, tt.want)
			}
			_, ident, err := source.Canonicalize(got)
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			if ident.Key != tt.key {
				t.Fatalf("key = %q, want %q", ident.Key, tt.key)
			}
		})
	}
}

func TestCanonicalizeRejectsInvalidSource(t *testing.T) {
	tests := []struct {
		name string
		spec source.SourceSpec
	}{
		{name: "invalid type", spec: source.SourceSpec{Type: "svn", URL: "https://example.com/repo"}},
		{name: "github traversal", spec: source.SourceSpec{Type: source.SourceGitHub, Repo: "owner/repo", Path: "../skills"}},
		{name: "github absolute path", spec: source.SourceSpec{Type: source.SourceGitHub, Repo: "owner/repo", Path: "/skills"}},
		{name: "local ref", spec: source.SourceSpec{Type: source.SourceLocal, Path: "/tmp/skills", Ref: "main"}},
		{name: "missing git url", spec: source.SourceSpec{Type: source.SourceGit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := source.Canonicalize(tt.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseInstallRef(t *testing.T) {
	tests := []struct {
		raw       string
		wantRepo  string
		wantSkill string
		wantPath  string
	}{
		{raw: "owner/repo:skill", wantRepo: "owner/repo", wantSkill: "skill"},
		{raw: "[https://github.com/owner/repo/tree/main/skills]:skill", wantRepo: "owner/repo", wantSkill: "skill", wantPath: "skills"},
	}
	for _, tt := range tests {
		got, err := source.ParseInstallRef(tt.raw)
		if err != nil {
			t.Fatalf("ParseInstallRef(%q): %v", tt.raw, err)
		}
		if got.Source.Repo != tt.wantRepo || got.Skill != tt.wantSkill || got.Source.Path != tt.wantPath {
			t.Fatalf("ParseInstallRef(%q) = %#v", tt.raw, got)
		}
	}

	if _, err := source.ParseInstallRef("https://github.com/owner/repo:skill"); err == nil {
		t.Fatal("expected unbracketed URL ref to fail")
	}
}
