package logo_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/logo"
)

func resetLogoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("SCRIBE_NO_BANNER", "")
}

func TestRenderFull(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if !strings.Contains(out, "███") {
		t.Error("expected full block characters in wide terminal output")
	}
	if !strings.Contains(out, "1.0.0") {
		t.Error("expected version string in output")
	}
}

func TestRenderCompact(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 50)

	out := buf.String()
	if !strings.Contains(out, "/ __|") {
		t.Error("expected compact logo characters in medium terminal output")
	}
	if strings.Contains(out, "███") {
		t.Error("should not contain full block characters at width 50")
	}
}

func TestRenderPlainText(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "2.0.0", 30)

	out := buf.String()
	if !strings.Contains(out, "Scribe v2.0.0") {
		t.Errorf("expected plain text fallback, got: %s", out)
	}
	if strings.Contains(out, "███") || strings.Contains(out, "/ __|") {
		t.Error("should not contain any ASCII art at narrow width")
	}
}

func TestRenderNoColor(t *testing.T) {
	resetLogoEnv(t)
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	// Should still contain block characters, just no ANSI escapes
	if !strings.Contains(out, "███") {
		t.Error("expected block characters even with NO_COLOR")
	}
	if strings.Contains(out, "\033[") {
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
		t.Errorf("expected plain text for TERM=dumb, got: %s", out)
	}
	if strings.Contains(out, "███") {
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
		t.Errorf("expected empty output when SCRIBE_NO_BANNER is set, got: %s", out)
	}
}

func TestRenderZeroWidth(t *testing.T) {
	resetLogoEnv(t)

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 0)

	out := buf.String()
	if !strings.Contains(out, "Scribe v1.0.0") {
		t.Errorf("expected plain text fallback for zero width, got: %s", out)
	}
}
