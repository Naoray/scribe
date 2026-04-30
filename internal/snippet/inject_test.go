package snippet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTargetsForExplicitList(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "claude")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "codex")
	writeFile(t, filepath.Join(root, ".cursorrules"), "cursor")

	got := TargetsFor(&Snippet{Targets: []string{"codex", "cursor"}}, root)
	want := []string{filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".cursorrules")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetsFor = %#v, want %#v", got, want)
	}
}

func TestTargetsForAllUsesExistingRuleFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "claude")
	writeFile(t, filepath.Join(root, ".cursorrules"), "cursor")

	got := TargetsFor(&Snippet{Targets: []string{"all"}}, root)
	want := []string{filepath.Join(root, "CLAUDE.md"), filepath.Join(root, ".cursorrules")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetsFor = %#v, want %#v", got, want)
	}
}

func TestInjectAppendsBlockToExistingEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	writeFile(t, path, "")
	s := &Snippet{Name: "terse-output", Body: "Keep replies short.\n"}

	if err := Inject(path, []*Snippet{s}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got := readFile(t, path)
	want := "<!-- scribe:start name=terse-output hash=" + BodyHash(s.Body) + " -->\n" +
		"Keep replies short.\n" +
		"<!-- scribe:end name=terse-output -->\n"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestInjectSkipsMissingTargetFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := Inject(path, []*Snippet{{Name: "missing", Body: "Body\n"}}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target stat err = %v, want not exist", err)
	}
}

func TestInjectSameHashIsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	s := &Snippet{Name: "stable", Body: "Stable body\n"}
	original := "Intro\n" + managedBlock(s) + "Outro\n"
	writeFile(t, path, original)

	if err := Inject(path, []*Snippet{s}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if got := readFile(t, path); got != original {
		t.Fatalf("content changed:\n%s", got)
	}
}

func TestInjectHashMismatchReplacesOnlyManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	old := &Snippet{Name: "rules", Body: "Old body\n"}
	next := &Snippet{Name: "rules", Body: "New body\n"}
	original := "User header\n" + managedBlock(old) + "User footer\n"
	writeFile(t, path, original)

	if err := Inject(path, []*Snippet{next}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got := readFile(t, path)
	want := "User header\n" + managedBlock(next) + "User footer\n"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if !strings.Contains(got, "User header\n") || !strings.Contains(got, "User footer\n") {
		t.Fatalf("user content was not preserved: %q", got)
	}
}

func TestInjectMultipleSnippetsInDeclaredOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	writeFile(t, path, "Intro\n")
	first := &Snippet{Name: "first", Body: "First\n"}
	second := &Snippet{Name: "second", Body: "Second\n"}

	if err := Inject(path, []*Snippet{first, second}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got := readFile(t, path)
	want := "Intro\n" + managedBlock(first) + managedBlock(second)
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestInjectTwiceIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	writeFile(t, path, "Intro\n")
	snippets := []*Snippet{
		{Name: "first", Body: "First\n"},
		{Name: "second", Body: "Second\n"},
	}

	if err := Inject(path, snippets); err != nil {
		t.Fatalf("first Inject: %v", err)
	}
	first := readFile(t, path)
	if err := Inject(path, snippets); err != nil {
		t.Fatalf("second Inject: %v", err)
	}
	second := readFile(t, path)
	if second != first {
		t.Fatalf("second inject changed content:\nfirst=%q\nsecond=%q", first, second)
	}
}

func TestRemoveDeletesOnlyMatchingBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	keep := &Snippet{Name: "keep", Body: "Keep\n"}
	drop := &Snippet{Name: "drop", Body: "Drop\n"}
	original := "Intro\n" + managedBlock(keep) + "Between\n" + managedBlock(drop) + "Outro\n"
	writeFile(t, path, original)

	if err := Remove(path, "drop"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got := readFile(t, path)
	want := "Intro\n" + managedBlock(keep) + "Between\n" + "Outro\n"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}
