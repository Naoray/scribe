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

func TestRenderLockup(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.13", 80)

	plain := stripANSI(buf.String())
	// Brand mark frame
	for _, want := range []string{"┌──────┐", "└──────┘", "│"} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected mark frame %q, got: %q", want, plain)
		}
	}
	// Chip square (orange in styled output)
	if !strings.Contains(plain, "▇") {
		t.Errorf("expected chip square ▇, got: %q", plain)
	}
	// Wordmark + version live on the middle row of the lockup.
	if !strings.Contains(plain, "scribe") {
		t.Errorf("expected 'scribe' wordmark, got: %q", plain)
	}
	if !strings.Contains(plain, "v1.0.13") {
		t.Errorf("expected version, got: %q", plain)
	}
	// Italic stylized 'S' inside the mark
	if !strings.Contains(plain, "S") {
		t.Errorf("expected 'S' inside mark, got: %q", plain)
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
	if strings.Contains(out, "┌") {
		t.Errorf("should not contain mark frame at narrow width, got: %q", out)
	}
}

func TestRenderNoColor(t *testing.T) {
	resetLogoEnv(t)
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if !strings.Contains(out, "┌──────┐") {
		t.Error("expected mark frame even with NO_COLOR")
	}
	if !strings.Contains(out, "scribe") {
		t.Error("expected wordmark even with NO_COLOR")
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Error("expected version even with NO_COLOR")
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
	if strings.Contains(out, "┌") {
		t.Error("should not contain mark frame for TERM=dumb")
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

	// Width 0 means "unknown" — assume wide and render the full lockup.
	plain := stripANSI(buf.String())
	if !strings.Contains(plain, "┌──────┐") {
		t.Errorf("expected lockup for unknown width (0), got: %q", plain)
	}
	if !strings.Contains(plain, "v1.0.0") {
		t.Errorf("expected version in output, got: %q", plain)
	}
}
