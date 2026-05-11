package snippet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadParsesFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	data := []byte("---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude, codex]\nsource: local\n---\n\n# Body\n")
	if err := os.WriteFile(filepath.Join(dir, "commit-discipline.md"), data, 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	sn, err := Load(dir, "commit-discipline")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sn.Name != "commit-discipline" || sn.Description != "Commit rules" || sn.Source != "local" {
		t.Fatalf("snippet metadata = %#v", sn)
	}
	if string(sn.Body) != "# Body\n" {
		t.Fatalf("Body = %q, want frontmatter stripped", sn.Body)
	}
	if len(sn.Targets) != 2 || sn.Targets[0] != "claude" || sn.Targets[1] != "codex" {
		t.Fatalf("Targets = %v", sn.Targets)
	}
}

func TestLoadParsesScalarAllTarget(t *testing.T) {
	dir := t.TempDir()
	data := []byte("---\nname: all-rules\ndescription: All rules\ntargets: all\nsource: local\n---\n# Body\n")
	if err := os.WriteFile(filepath.Join(dir, "all-rules.md"), data, 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	sn, err := Load(dir, "all-rules")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(sn.Targets) != 1 || sn.Targets[0] != "all" {
		t.Fatalf("Targets = %v, want [all]", sn.Targets)
	}
}

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
	if !strings.Contains(string(first), "user rule\n") || !strings.Contains(string(first), "<!-- scribe:start name=commit-discipline hash=") {
		t.Fatalf("CLAUDE.md missing user content or marker:\n%s", first)
	}
	if !strings.Contains(string(first), "<!-- scribe:end name=commit-discipline -->") {
		t.Fatalf("CLAUDE.md missing end marker:\n%s", first)
	}

	paths, err = Project(project, []Snippet{sn}, []string{"claude", "codex"})
	if err != nil {
		t.Fatalf("Project again: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want no changed targets", paths)
	}
	second, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md again: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("projection not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestProjectUpdatesExistingBlockWhenHashChanges(t *testing.T) {
	project := t.TempDir()
	sn := Snippet{Name: "rules", Targets: []string{"claude"}, Body: []byte("old body\n")}
	if _, err := Project(project, []Snippet{sn}, []string{"claude"}); err != nil {
		t.Fatalf("Project initial: %v", err)
	}
	sn.Body = []byte("new body\n")
	if _, err := Project(project, []Snippet{sn}, []string{"claude"}); err != nil {
		t.Fatalf("Project update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if strings.Contains(string(data), "old body") || !strings.Contains(string(data), "new body") {
		t.Fatalf("block was not hash-updated:\n%s", data)
	}
	if strings.Count(string(data), "<!-- scribe:start name=rules hash=") != 1 {
		t.Fatalf("expected one rules block:\n%s", data)
	}
}

func TestProjectPreservesUserContentAroundManagedBlocks(t *testing.T) {
	project := t.TempDir()
	above := "top: user bytes stay exactly\n\n"
	between := "between: keep spacing\tand punctuation!\n\n"
	below := "bottom: no rewrite\n"
	existing := above +
		"<!-- scribe:start name=old hash=abc -->\nold\n<!-- scribe:end name=old -->\n" +
		between +
		"<!-- scribe:start name=keep hash=abc -->\nstale\n<!-- scribe:end name=keep -->\n" +
		below
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	sn := Snippet{Name: "keep", Targets: []string{"codex"}, Body: []byte("fresh\n")}
	if _, err := Project(project, []Snippet{sn}, []string{"codex"}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	got := string(data)
	managedStart := strings.Index(got, "<!-- scribe:start name=keep hash=")
	if managedStart < 0 {
		t.Fatalf("updated managed block missing:\n%s", got)
	}
	unmanaged := got[:managedStart]
	if unmanaged != above+between+below+"\n" {
		t.Fatalf("unmanaged content changed\n got: %q\nwant: %q", unmanaged, above+between+below+"\n")
	}
	if strings.Contains(got, "old\n") || strings.Contains(got, "stale\n") || !strings.Contains(got, "fresh\n") {
		t.Fatalf("managed block content mismatch:\n%s", got)
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

func TestProjectWritesCursorRulesFile(t *testing.T) {
	project := t.TempDir()
	sn := Snippet{
		Name:        "commit-discipline",
		Description: "Commit rules",
		Targets:     []string{"cursor"},
		Body:        []byte("# Agent Commit Discipline\n"),
	}

	paths, err := Project(project, []Snippet{sn}, []string{"cursor"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %v, want one changed target", paths)
	}
	data, err := os.ReadFile(filepath.Join(project, ".cursorrules"))
	if err != nil {
		t.Fatalf("read .cursorrules: %v", err)
	}
	if !strings.Contains(string(data), "<!-- scribe:start name=commit-discipline hash=") || !strings.Contains(string(data), "Agent Commit Discipline") {
		t.Fatalf(".cursorrules missing expected content:\n%s", data)
	}
	paths, err = Project(project, []Snippet{sn}, []string{"cursor"})
	if err != nil {
		t.Fatalf("Project again: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want no changed targets", paths)
	}
}

func TestProjectAllTargetsExistingOnlyUnlessCreateTargets(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("codex user\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	sn := Snippet{Name: "all-rules", Targets: []string{"all"}, Body: []byte("body\n")}
	paths, err := Project(project, []Snippet{sn}, []string{"claude", "codex", "cursor"})
	if err != nil {
		t.Fatalf("Project existing-only: %v", err)
	}
	if len(paths) != 1 || filepath.Base(paths[0]) != "AGENTS.md" {
		t.Fatalf("paths = %v, want only AGENTS.md", paths)
	}
	for _, rel := range []string{"CLAUDE.md", ".cursorrules"} {
		if _, err := os.Stat(filepath.Join(project, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s created without CreateTargets: %v", rel, err)
		}
	}

	paths, err = ProjectWithOptions(project, []Snippet{sn}, []string{"claude", "codex", "cursor"}, ProjectOptions{CreateTargets: true})
	if err != nil {
		t.Fatalf("Project create-targets: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want two newly created targets", paths)
	}
	for _, rel := range []string{"CLAUDE.md", "AGENTS.md", ".cursorrules"} {
		data, err := os.ReadFile(filepath.Join(project, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "body\n") {
			t.Fatalf("%s missing snippet body:\n%s", rel, data)
		}
	}
}

func TestContentForBudget(t *testing.T) {
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
}
