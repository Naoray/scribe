package textdiff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		modified string
		want     []string
	}{
		{name: "identical inputs return empty", base: "one\ntwo\n", modified: "one\ntwo\n"},
		{name: "addition", base: "one\n", modified: "one\ntwo\n", want: []string{"--- a/SKILL.md", "+++ b/SKILL.md", "+two"}},
		{name: "deletion", base: "one\ntwo\n", modified: "one\n", want: []string{"--- a/SKILL.md", "+++ b/SKILL.md", "-two"}},
		{name: "hunk grouping", base: "a\nb\nc\nd\ne\nf\ng\n", modified: "a\nb\nC\nd\ne\nF\ng\n", want: []string{"@@", "-c", "+C", "-f", "+F"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified("SKILL.md", []byte(tt.base), []byte(tt.modified))
			if len(tt.want) == 0 {
				if got != "" {
					t.Fatalf("Unified() = %q, want empty", got)
				}
				return
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("Unified() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestTruncateUnified(t *testing.T) {
	tests := []struct {
		name      string
		diff      string
		maxLines  int
		maxBytes  int
		want      string
		truncated bool
	}{
		{name: "unchanged under cap", diff: "a\nb\n", maxLines: 5, maxBytes: 100, want: "a\nb\n"},
		{name: "line cap", diff: "a\nb\nc\nd\n", maxLines: 2, maxBytes: 100, want: "a\nb\n… diff truncated: 2 more lines not shown", truncated: true},
		{name: "byte cap", diff: "abcdef\nsecond\n", maxLines: 10, maxBytes: 3, want: "abc\n… diff truncated: 2 more lines not shown", truncated: true},
		{name: "zero cap", diff: "a\nb\n", maxLines: 0, maxBytes: 100, want: "… diff truncated: 2 more lines not shown", truncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := TruncateUnified(tt.diff, tt.maxLines, tt.maxBytes)
			if truncated != tt.truncated {
				t.Fatalf("truncated = %v, want %v", truncated, tt.truncated)
			}
			if got != tt.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestTruncateUnifiedLargeInput(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 600; i++ {
		b.WriteString("+line\n")
	}
	got, truncated := TruncateUnified(b.String(), 500, 64*1024)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if lines := strings.Count(got, "\n") + 1; lines != 501 {
		t.Fatalf("rendered lines = %d, want 501 including footer", lines)
	}
	if !strings.Contains(got, "… diff truncated: 100 more lines not shown") {
		t.Fatalf("missing footer in:\n%s", got)
	}
}
