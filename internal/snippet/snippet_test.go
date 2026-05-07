package snippet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectWritesManagedBlocksIdempotently(t *testing.T) {
	project := t.TempDir()
	sn := Snippet{
		Name:        "commit-discipline",
		Description: "Commit rules",
		Targets:     []string{"claude", "codex"},
		Body:        []byte("# Agent Commit Discipline\n\nCommit after each logical phase.\n"),
	}
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("user rule\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	paths, err := Project(project, []Snippet{sn}, []string{"claude", "codex"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want two targets", paths)
	}
	first, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(first), "user rule") || !strings.Contains(string(first), "Agent Commit Discipline") {
		t.Fatalf("CLAUDE.md missing user content or snippet:\n%s", first)
	}

	if _, err := Project(project, []Snippet{sn}, []string{"claude", "codex"}); err != nil {
		t.Fatalf("Project again: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md again: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("projection not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestProjectRemovesStaleBlocksAndAppliesOrder(t *testing.T) {
	project := t.TempDir()
	first := Snippet{Name: "first", Targets: []string{"claude"}, Body: []byte("first body\n")}
	second := Snippet{Name: "second", Targets: []string{"claude"}, Body: []byte("second body\n")}

	if _, err := Project(project, []Snippet{first, second}, []string{"claude"}); err != nil {
		t.Fatalf("Project initial: %v", err)
	}
	if _, err := Project(project, []Snippet{second}, []string{"claude"}); err != nil {
		t.Fatalf("Project updated: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if strings.Contains(string(data), "first body") {
		t.Fatalf("removed snippet still projected:\n%s", data)
	}
	if !strings.Contains(string(data), "second body") {
		t.Fatalf("kept snippet missing:\n%s", data)
	}

	if _, err := Project(project, []Snippet{first, second}, []string{"claude"}); err != nil {
		t.Fatalf("Project reordered: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read reordered CLAUDE.md: %v", err)
	}
	if strings.Index(string(data), "first body") > strings.Index(string(data), "second body") {
		t.Fatalf("snippet order not applied:\n%s", data)
	}
}

func TestProjectRemovesCursorRuleWhenSnippetRemoved(t *testing.T) {
	project := t.TempDir()
	sn := Snippet{Name: "commit-discipline", Targets: []string{"cursor"}, Body: []byte("# Body\n")}
	if _, err := Project(project, []Snippet{sn}, []string{"cursor"}); err != nil {
		t.Fatalf("Project initial: %v", err)
	}
	path := filepath.Join(project, ".cursor", "rules", "commit-discipline.mdc")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat cursor rule: %v", err)
	}
	if _, err := Project(project, nil, []string{"cursor"}); err != nil {
		t.Fatalf("Project removed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cursor rule still exists or stat failed: %v", err)
	}
}

func TestContentForBudgetAndCursorRuleQuoteYAMLDescriptions(t *testing.T) {
	sn := Snippet{
		Name:        "quotes",
		Description: "Commit: \"carefully\"\nwith newline",
		Targets:     []string{"cursor"},
		Body:        []byte("# Body\n"),
	}
	if !strings.Contains(string(ContentForBudget(sn)), "description:") {
		t.Fatalf("budget content missing description:\n%s", ContentForBudget(sn))
	}
	if strings.Contains(string(ContentForBudget(sn)), "name:") || strings.Contains(string(ContentForBudget(sn)), "targets:") {
		t.Fatalf("budget content includes empty metadata:\n%s", ContentForBudget(sn))
	}
	project := t.TempDir()
	if _, err := Project(project, []Snippet{sn}, []string{"cursor"}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, ".cursor", "rules", "quotes.mdc"))
	if err != nil {
		t.Fatalf("read cursor rule: %v", err)
	}
	if !strings.Contains(string(data), "Commit:") || !strings.Contains(string(data), "with newline") {
		t.Fatalf("cursor YAML lost description:\n%s", data)
	}
}

func TestProjectWritesCursorRule(t *testing.T) {
	project := t.TempDir()
	sn := Snippet{
		Name:        "commit discipline",
		Description: "Commit rules",
		Targets:     []string{"cursor"},
		Body:        []byte("# Agent Commit Discipline\n"),
	}

	if _, err := Project(project, []Snippet{sn}, []string{"cursor"}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, ".cursor", "rules", "commit-discipline.mdc"))
	if err != nil {
		t.Fatalf("read cursor rule: %v", err)
	}
	if !strings.Contains(string(data), "alwaysApply: false") || !strings.Contains(string(data), "Agent Commit Discipline") {
		t.Fatalf("cursor rule missing expected content:\n%s", data)
	}
}

func TestLoadStripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	data := []byte("---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude, codex]\n---\n\n# Body\n")
	if err := os.WriteFile(filepath.Join(dir, "commit-discipline.md"), data, 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	sn, err := Load(dir, "commit-discipline")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(sn.Body) != "# Body\n" {
		t.Fatalf("Body = %q, want frontmatter stripped", sn.Body)
	}
	if len(sn.Targets) != 2 || sn.Targets[0] != "claude" || sn.Targets[1] != "codex" {
		t.Fatalf("Targets = %v", sn.Targets)
	}
}
