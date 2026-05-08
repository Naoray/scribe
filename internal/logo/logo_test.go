package logo_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/logo"
)

// ansiEscape strips CSI / OSC escape sequences for plain-text assertions.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func resetLogoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("SCRIBE_NO_BANNER", "")
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			return line
		}
	}
	return ""
}

func TestRenderBannerSingleLine(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.13", 80)

	out := buf.String()
	plain := stripANSI(out)
	first := firstLine(plain)
	if !strings.Contains(first, "█████") {
		t.Errorf("expected block characters on first line, got: %q", first)
	}
	if !strings.Contains(first, "scribe") {
		t.Errorf("expected 'scribe' on first line, got: %q", first)
	}
	if !strings.Contains(first, "─────") {
		t.Errorf("expected '─────' divider on first line, got: %q", first)
	}
	if !strings.Contains(first, "v1.0.13") {
		t.Errorf("expected version inline on first line, got: %q", first)
	}

	// Banner must be a single visual line — no second pixel-art row.
	if strings.Count(plain, "█████") > 1 {
		t.Errorf("expected only one block-character row, got: %q", plain)
	}
}

func TestRenderNarrowFallback(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "2.0.0", 20)

	out := buf.String()
	if !strings.Contains(out, "Scribe v2.0.0") {
		t.Errorf("expected plain text fallback at narrow width, got: %q", out)
	}
	if strings.Contains(out, "█") {
		t.Errorf("should not contain block characters at narrow width, got: %q", out)
	}
}

func TestRenderNoColor(t *testing.T) {
	resetLogoEnv(t)
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if !strings.Contains(out, "█████") {
		t.Error("expected block characters even with NO_COLOR")
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Error("expected version inline even with NO_COLOR")
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("should not contain ANSI escape sequences when NO_COLOR is set")
	}
}

func TestRenderDumbTerminal(t *testing.T) {
	resetLogoEnv(t)
	t.Setenv("TERM", "dumb")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if !strings.Contains(out, "Scribe v1.0.0") {
		t.Errorf("expected plain text for TERM=dumb, got: %q", out)
	}
	if strings.Contains(out, "█") {
		t.Error("should not contain block characters for TERM=dumb")
	}
}

func TestRenderNoBanner(t *testing.T) {
	resetLogoEnv(t)
	t.Setenv("SCRIBE_NO_BANNER", "1")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if out != "" {
		t.Errorf("expected empty output when SCRIBE_NO_BANNER is set, got: %q", out)
	}
}

func TestRenderZeroWidth(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 0)

	// Width 0 means "unknown" — assume wide and render the banner.
	plain := stripANSI(buf.String())
	if !strings.Contains(plain, "█████") {
		t.Errorf("expected banner for unknown width (0), got: %q", plain)
	}
	if !strings.Contains(plain, "v1.0.0") {
		t.Errorf("expected version in output, got: %q", plain)
	}
}
