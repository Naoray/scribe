package textdiff

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// Unified produces a unified diff "a/<label>" -> "b/<label>" between base and modified.
func Unified(label string, base, modified []byte) string {
	if string(base) == string(modified) {
		return ""
	}
	edits := myers.ComputeEdits(span.URIFromPath(label), string(base), string(modified))
	return fmt.Sprint(gotextdiff.ToUnified("a/"+label, "b/"+label, string(base), edits))
}

// TruncateUnified caps diff by line count or byte count, whichever hits first.
func TruncateUnified(diff string, maxLines, maxBytes int) (string, bool) {
	if diff == "" {
		return "", false
	}
	if maxLines <= 0 || maxBytes <= 0 {
		return fmt.Sprintf("… diff truncated: %d more lines not shown", countLines(diff)), true
	}
	lines := strings.SplitAfter(diff, "\n")
	totalLines := len(lines)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		totalLines--
		lines = lines[:len(lines)-1]
	}

	var b strings.Builder
	writtenLines := 0
	for _, line := range lines {
		if writtenLines >= maxLines {
			break
		}
		if b.Len()+len(line) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 0 {
				b.WriteString(validPrefix(line, remaining))
			}
			break
		}
		b.WriteString(line)
		writtenLines++
	}
	if writtenLines >= totalLines && b.Len() == len(diff) {
		return diff, false
	}
	remaining := totalLines - writtenLines
	if remaining < 0 {
		remaining = 0
	}
	out := strings.TrimRight(b.String(), "\n")
	if out != "" {
		out += "\n"
	}
	out += fmt.Sprintf("… diff truncated: %d more lines not shown", remaining)
	return out, true
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func validPrefix(s string, maxBytes int) string {
	if maxBytes >= len(s) {
		return s
	}
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
