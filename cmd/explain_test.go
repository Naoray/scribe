package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/spf13/cobra"
)

// newTestRoot returns a fresh, isolated cobra command tree for each test.
func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "scribe",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().Bool("json", false, "Output machine-readable JSON")
	root.AddCommand(newExplainCommand())
	wrapRunECommands(root)
	return root
}

// writeSkill writes a SKILL.md into a temp scribe skills dir and returns the home dir.
func writeSkill(t *testing.T, name, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(home, ".scribe", "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestExplainSkillNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := newTestRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"explain", "nonexistent-skill"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

func TestExplainJSON(t *testing.T) {
	skillMD := "---\nname: test-skill\ndescription: A test skill\nversion: \"1.0.0\"\n---\n\n# Test Skill\n\nThis skill does testing things.\n"
	writeSkill(t, "test-skill", skillMD)

	root := newTestRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"explain", "--json", "test-skill"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name"`) || !strings.Contains(output, "test-skill") {
		t.Errorf("expected JSON with skill name, got:\n%s", output)
	}
}

func TestExplainSnippetJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".scribe", "snippets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	content := "---\nname: commit-discipline\ndescription: Commit rules\ntargets: [claude]\n---\n# Agent Commit Discipline\n"
	if err := os.WriteFile(filepath.Join(dir, "commit-discipline.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}

	root := newTestRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"explain", "--json", "commit-discipline"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "commit-discipline") || !strings.Contains(output, "Agent Commit Discipline") {
		t.Fatalf("snippet explain output missing content:\n%s", output)
	}
}

func TestExplainJSONWithoutLLM(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no claude binary available

	writeSkill(t, "test-skill", "---\nname: test-skill\ndescription: A test\n---\n\n# Test\n\nBody.\n")

	root := newTestRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"explain", "--json", "test-skill"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExplainRawFlagAndJSONMutuallyExclusive(t *testing.T) {
	writeSkill(t, "test-skill", "---\nname: test-skill\n---\n\n# Test\n")

	root := newTestRoot()
	root.SetArgs([]string{"explain", "--json", "--raw", "test-skill"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --json and --raw are both set")
	}
}

func TestExtractPreview(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      int
		wantPrev string
		wantMore bool
	}{
		{
			name:     "empty body",
			body:     "",
			max:      3,
			wantPrev: "",
			wantMore: false,
		},
		{
			name:     "single paragraph",
			body:     "Hello world.",
			max:      3,
			wantPrev: "Hello world.",
			wantMore: false,
		},
		{
			name:     "exactly max paragraphs",
			body:     "# Title\n\nParagraph one.\n\nParagraph two.",
			max:      3,
			wantPrev: "# Title\n\nParagraph one.\n\nParagraph two.",
			wantMore: false,
		},
		{
			name:     "more than max",
			body:     "# Title\n\nFirst.\n\nSecond.\n\nThird.\n\nFourth.",
			max:      3,
			wantPrev: "# Title\n\nFirst.\n\nSecond.",
			wantMore: true,
		},
		{
			name:     "whitespace trimmed",
			body:     "\n\n  # Title\n\nBody.\n\n",
			max:      2,
			wantPrev: "# Title\n\nBody.",
			wantMore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasMore := extractPreview(tt.body, tt.max)
			if got != tt.wantPrev {
				t.Errorf("extractPreview preview = %q, want %q", got, tt.wantPrev)
			}
			if hasMore != tt.wantMore {
				t.Errorf("extractPreview hasMore = %v, want %v", hasMore, tt.wantMore)
			}
		})
	}
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no frontmatter",
			input: "# Hello\n\nWorld",
			want:  "# Hello\n\nWorld",
		},
		{
			name:  "with frontmatter",
			input: "---\nname: foo\nversion: 1.0\n---\n\n# Hello\n\nWorld",
			want:  "# Hello\n\nWorld",
		},
		{
			name:  "unclosed frontmatter",
			input: "---\nname: foo\n# Hello",
			want:  "---\nname: foo\n# Hello",
		},
		{
			name:  "frontmatter only",
			input: "---\nname: foo\n---\n",
			want:  "",
		},
		{
			name:  "windows crlf",
			input: "---\r\nname: foo\r\n---\r\n\r\n# Hello",
			want:  "# Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.input)
			if got != tt.want {
				t.Errorf("stripFrontmatter(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExplainRendered(t *testing.T) {
	skill := discovery.Skill{
		Name:        "test-skill",
		Description: "A test skill",
		Revision:    1,
		Targets:     []string{"claude", "cursor"},
	}
	content := "---\nname: test-skill\n---\n\n# Test Skill\n\nThis skill does testing things."

	buf := new(bytes.Buffer)
	err := explainRendered(buf, skill, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test-skill") {
		t.Errorf("expected skill name in output, got:\n%s", output)
	}
	if !strings.Contains(output, "A test skill") {
		t.Errorf("expected description in output, got:\n%s", output)
	}
	if !strings.Contains(output, "rev 1") {
		t.Errorf("expected revision in output, got:\n%s", output)
	}
}

func TestPrintSkillHeaderMinimal(t *testing.T) {
	skill := discovery.Skill{Name: "bare-skill"}
	buf := new(bytes.Buffer)
	printSkillHeader(buf, skill)
	output := buf.String()
	if !strings.Contains(output, "bare-skill") {
		t.Errorf("expected skill name, got:\n%s", output)
	}
	if strings.Contains(output, "Version:") || strings.Contains(output, "Source:") || strings.Contains(output, "Agents:") {
		t.Errorf("unexpected metadata in minimal header, got:\n%s", output)
	}
}

func TestRunAIExplanationSuccess(t *testing.T) {
	original := buildLLMCmd
	buildLLMCmd = func(ctx context.Context, prompt string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "**AI explanation** here")
	}
	t.Cleanup(func() { buildLLMCmd = original })

	buf := new(bytes.Buffer)
	err := runAIExplanation(buf, context.Background(), "---\nname: my-skill\n---\n\n# Body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Errorf("expected some output from LLM, got empty")
	}
}

func TestRunAIExplanationLLMFailure(t *testing.T) {
	original := buildLLMCmd
	buildLLMCmd = func(ctx context.Context, prompt string) *exec.Cmd {
		return exec.CommandContext(ctx, "false") // exits non-zero
	}
	t.Cleanup(func() { buildLLMCmd = original })

	buf := new(bytes.Buffer)
	// LLM failure returns nil — caller already rendered the skill file
	err := runAIExplanation(buf, context.Background(), "# Body")
	if err != nil {
		t.Fatalf("expected nil on LLM failure, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output on LLM failure, got: %s", buf.String())
	}
}

func TestStartSpinnerStopIdempotent(t *testing.T) {
	buf := new(bytes.Buffer)
	s := startSpinner(buf, "Loading...")
	time.Sleep(200 * time.Millisecond)
	s.stop()
	s.stop() // idempotent — must not panic
	select {
	case <-s.done:
		// goroutine terminated cleanly
	case <-time.After(time.Second):
		t.Fatal("spinner goroutine did not terminate after stop()")
	}
}

func TestFindSkillSuffix(t *testing.T) {
	skills := []discovery.Skill{
		{Name: "gstack/browse"},
		{Name: "other/deploy"},
		{Name: "standalone"},
	}

	tests := []struct {
		query string
		want  string
		found bool
	}{
		{"standalone", "standalone", true},
		{"browse", "gstack/browse", true},
		{"deploy", "other/deploy", true},
		{"gstack/browse", "gstack/browse", true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			sk, ok := findSkill(skills, tt.query)
			if ok != tt.found {
				t.Fatalf("findSkill(%q) found=%v, want %v", tt.query, ok, tt.found)
			}
			if ok && sk.Name != tt.want {
				t.Errorf("findSkill(%q) = %q, want %q", tt.query, sk.Name, tt.want)
			}
		})
	}
}

func TestFindSkillEmptySlice(t *testing.T) {
	_, ok := findSkill([]discovery.Skill{}, "anything")
	if ok {
		t.Error("expected not found on empty slice")
	}
}

func TestExplainJSONUnit(t *testing.T) {
	skill := discovery.Skill{
		Name:        "my-skill",
		Description: "Does things",
		Source: discovery.Source{
			URL:    "https://github.com/acme/my-skill",
			Author: "acme",
			Note:   "original source",
		},
		Revision:  2,
		Targets:   []string{"claude"},
		LocalPath: "/tmp/skills/my-skill",
	}
	content := "# My Skill\n\nHello world."

	buf := new(bytes.Buffer)
	err := explainJSON(buf, skill, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name": "my-skill"`) {
		t.Errorf("expected name field, got:\n%s", output)
	}
	if !strings.Contains(output, `"revision": 2`) {
		t.Errorf("expected revision field, got:\n%s", output)
	}
	if !strings.Contains(output, `"content": "# My Skill\n\nHello world."`) {
		t.Errorf("expected content field, got:\n%s", output)
	}
	if !strings.Contains(output, `"source": {`) {
		t.Errorf("expected source object, got:\n%s", output)
	}
	if !strings.Contains(output, `"author": "acme"`) {
		t.Errorf("expected source author, got:\n%s", output)
	}
}
