package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteToStoreFlat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	files := []SkillFile{
		{Path: "SKILL.md", Content: []byte("# My Skill")},
	}

	dir, err := WriteToStore("cleanup", files)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	// Should be directly under ~/.scribe/skills/<name> — no slug subdirectory.
	wantSuffix := filepath.Join(".scribe", "skills", "cleanup")
	if !containsSuffix(dir, wantSuffix) {
		t.Errorf("got dir %q, want suffix %q", dir, wantSuffix)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(content) != "# My Skill" {
		t.Errorf("SKILL.md content = %q, want %q", content, "# My Skill")
	}
}

func TestWriteToStoreBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	files := []SkillFile{
		{Path: "SKILL.md", Content: []byte("# Base Content")},
		{Path: "scripts/run.sh", Content: []byte("#!/bin/bash")},
	}

	dir, err := WriteToStore("deploy", files)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	basePath := filepath.Join(dir, ".scribe-base.md")
	content, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read .scribe-base.md: %v", err)
	}
	if string(content) != "# Base Content" {
		t.Errorf(".scribe-base.md content = %q, want %q", content, "# Base Content")
	}
}

func TestWriteToStoreNoBaseWithoutSkillMD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	files := []SkillFile{
		{Path: "README.md", Content: []byte("# Readme")},
	}

	dir, err := WriteToStore("nobase", files)
	if err != nil {
		t.Fatalf("WriteToStore: %v", err)
	}

	basePath := filepath.Join(dir, ".scribe-base.md")
	if _, err := os.Stat(basePath); !os.IsNotExist(err) {
		t.Errorf(".scribe-base.md should not exist when SKILL.md is absent")
	}
}

func TestWriteToStoreReservedNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, name := range []string{"versions", ".git", ".DS_Store"} {
		_, err := WriteToStore(name, []SkillFile{
			{Path: "SKILL.md", Content: []byte("x")},
		})
		if err == nil {
			t.Errorf("expected error for reserved name %q, got nil", name)
		}
	}
}

func TestWriteToStorePathTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := WriteToStore("../escape", []SkillFile{
		{Path: "SKILL.md", Content: []byte("x")},
	})
	if err == nil {
		t.Error("expected error for path traversal in skillName, got nil")
	}
}

func containsSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) &&
		path[len(path)-len(suffix):] == suffix
}
