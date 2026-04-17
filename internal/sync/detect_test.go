package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func TestDetectKind(t *testing.T) {
	tests := []struct {
		name  string
		files []tools.SkillFile
		want  sync.Kind
	}{
		{
			name: "plain skill: root SKILL.md only",
			files: []tools.SkillFile{
				{Path: "SKILL.md"},
			},
			want: sync.KindSkill,
		},
		{
			name: "plain skill: SKILL.md plus resources",
			files: []tools.SkillFile{
				{Path: "SKILL.md"},
				{Path: "scripts/deploy.sh"},
				{Path: "README.md"},
			},
			want: sync.KindSkill,
		},
		{
			name: "package: root SKILL.md AND nested SKILL.md",
			files: []tools.SkillFile{
				{Path: "SKILL.md"},
				{Path: "browse/SKILL.md"},
			},
			want: sync.KindPackage,
		},
		{
			name: "package: only nested SKILL.md, no root",
			files: []tools.SkillFile{
				{Path: "codex/SKILL.md"},
				{Path: "ship/SKILL.md"},
			},
			want: sync.KindPackage,
		},
		{
			name: "package: no SKILL.md anywhere but has setup script",
			files: []tools.SkillFile{
				{Path: "setup"},
				{Path: "lib/main.js"},
			},
			want: sync.KindPackage,
		},
		{
			name: "package: install.sh at root",
			files: []tools.SkillFile{
				{Path: "install.sh"},
				{Path: "README.md"},
			},
			want: sync.KindPackage,
		},
		{
			name: "package: package.json at root",
			files: []tools.SkillFile{
				{Path: "package.json"},
				{Path: "src/index.ts"},
			},
			want: sync.KindPackage,
		},
		{
			name: "skill: root SKILL.md wins over install script",
			files: []tools.SkillFile{
				{Path: "SKILL.md"},
				{Path: "setup"},
			},
			want: sync.KindSkill,
		},
		{
			name: "skill: empty tree",
			files: []tools.SkillFile{},
			want:  sync.KindSkill,
		},
		{
			name: "package: deeply nested SKILL.md",
			files: []tools.SkillFile{
				{Path: "SKILL.md"},
				{Path: "openclaw/skills/gstack-openclaw-office-hours/SKILL.md"},
			},
			want: sync.KindPackage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sync.DetectKind(tt.files)
			if got != tt.want {
				t.Errorf("DetectKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectKindFromDir(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("SKILL.md")
	write("browse/SKILL.md")
	write(".scribe-base.md")

	kind, err := sync.DetectKindFromDir(dir)
	if err != nil {
		t.Fatalf("DetectKindFromDir: %v", err)
	}
	if kind != sync.KindPackage {
		t.Errorf("got %q, want %q", kind, sync.KindPackage)
	}
}

func TestDetectKindFromDir_Skill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".scribe-base.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	kind, err := sync.DetectKindFromDir(dir)
	if err != nil {
		t.Fatalf("DetectKindFromDir: %v", err)
	}
	if kind != sync.KindSkill {
		t.Errorf("got %q, want %q", kind, sync.KindSkill)
	}
}
