package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	root.AddCommand(newExplainCommand())
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
		Version:     "1.0.0",
		Source:      "github.com/test/repo",
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
	if !strings.Contains(output, "1.0.0") {
		t.Errorf("expected version in output, got:\n%s", output)
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

func TestExplainWithAISuccess(t *testing.T) {
	original := buildLLMCmd
	buildLLMCmd = func(ctx context.Context, prompt string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "AI explanation here")
	}
	t.Cleanup(func() { buildLLMCmd = original })

	skill := discovery.Skill{Name: "my-skill", Description: "Does things"}
	buf := new(bytes.Buffer)
	err := explainWithAI(buf, context.Background(), skill, "---\nname: my-skill\n---\n\n# Body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "my-skill") {
		t.Errorf("expected skill name in output, got:\n%s", output)
	}
}

func TestExplainWithAIFallback(t *testing.T) {
	original := buildLLMCmd
	buildLLMCmd = func(ctx context.Context, prompt string) *exec.Cmd {
		return exec.CommandContext(ctx, "false") // exits non-zero
	}
	t.Cleanup(func() { buildLLMCmd = original })

	skill := discovery.Skill{Name: "my-skill"}
	content := "---\nname: my-skill\n---\n\n# Body\n\nFallback content."
	buf := new(bytes.Buffer)
	err := explainWithAI(buf, context.Background(), skill, content)
	if err != nil {
		t.Fatalf("unexpected error on fallback: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Fallback content") {
		t.Errorf("expected fallback body in output, got:\n%s", output)
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
		Version:     "2.0.0",
		Source:      "github.com/org/repo",
		Targets:     []string{"claude"},
		LocalPath:   "/tmp/skills/my-skill",
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
	if !strings.Contains(output, `"version": "2.0.0"`) {
		t.Errorf("expected version field, got:\n%s", output)
	}
	if !strings.Contains(output, `"content": "# My Skill\n\nHello world."`) {
		t.Errorf("expected content field, got:\n%s", output)
	}
}
