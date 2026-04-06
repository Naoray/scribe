# ASCII Logo & Interactive Root Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an ANSI Shadow ASCII logo with teal→cyan gradient to the bare `scribe` command, and make it an interactive hub showing status + quick actions.

**Architecture:** A new `internal/logo/` package owns the logo constants and gradient rendering with zero Scribe-internal dependencies. `cmd/root_hub.go` wires up the interactive hub (logo + Lip Gloss status + Huh action menu). The root command gains `RunE`, `cobra.NoArgs`, and `SilenceUsage`. Non-TTY/CI/`--json` outputs status as JSON.

**Tech Stack:** Go 1.26.1, `charm.land/lipgloss/v2`, `charm.land/huh/v2`, `github.com/mattn/go-isatty`, `golang.org/x/term` (new)

**Spec:** `docs/superpowers/specs/2026-04-06-ascii-logo-design.md`

---

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Logo | `internal/logo/logo.go` | Create | Logo constants (full + compact), `Render(w, version, width)`, gradient, suppression |
| Logo | `internal/logo/logo_test.go` | Create | Width selection, NO_COLOR, TERM=dumb, output assertions |
| Root hub | `cmd/root_hub.go` | Create | `runHub()`: status gathering, Lip Gloss output, Huh menu, JSON mode |
| Root hub | `cmd/root_hub_test.go` | Create | JSON output, status formatting, degradation |
| Root | `cmd/root.go` | Modify | Add `RunE`, `--json` local flag, `cobra.NoArgs`, `SilenceUsage` |

---

### Task 1: Add `golang.org/x/term` dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

```bash
go get golang.org/x/term
```

- [ ] **Step 2: Verify it resolved**

```bash
grep "golang.org/x/term" go.mod
```

Expected: a line like `golang.org/x/term v0.X.0`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/term for terminal width detection"
```

---

### Task 2: Logo package — tests first

**Files:**
- Create: `internal/logo/logo_test.go`

- [ ] **Step 1: Write tests for logo rendering**

```go
package logo_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/logo"
)

func TestRenderFull(t *testing.T) {
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
	t.Setenv("SCRIBE_NO_BANNER", "1")

	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 80)

	out := buf.String()
	if out != "" {
		t.Errorf("expected empty output when SCRIBE_NO_BANNER is set, got: %s", out)
	}
}

func TestRenderZeroWidth(t *testing.T) {
	var buf bytes.Buffer
	logo.Render(&buf, "1.0.0", 0)

	out := buf.String()
	if !strings.Contains(out, "Scribe v1.0.0") {
		t.Errorf("expected plain text fallback for zero width, got: %s", out)
	}
}
```

- [ ] **Step 2: Verify tests fail (package doesn't exist yet)**

```bash
go test ./internal/logo/... 2>&1
```

Expected: compilation error — package `logo` does not exist.

- [ ] **Step 3: Commit**

```bash
git add internal/logo/logo_test.go
git commit -m "test: add logo rendering tests (red)"
```

---

### Task 3: Logo package — implementation

**Files:**
- Create: `internal/logo/logo.go`

- [ ] **Step 1: Implement the logo package**

```go
package logo

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// logoFull is the ANSI Shadow style logo (~48 chars wide × 6 lines).
const logoFull = `███████╗ ██████╗██████╗ ██╗██████╗ ███████╗
██╔════╝██╔════╝██╔══██╗██║██╔══██╗██╔════╝
███████╗██║     ██████╔╝██║██████╔╝█████╗
╚════██║██║     ██╔══██╗██║██╔══██╗██╔══╝
███████║╚██████╗██║  ██║██║██████╔╝███████╗
╚══════╝ ╚═════╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝`

// logoCompact is the small FIGlet logo (~28 chars wide × 4 lines).
const logoCompact = ` ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|`

// Render writes the Scribe logo and version to w.
// width is the terminal width in columns — used to select logo size.
// Respects SCRIBE_NO_BANNER, TERM=dumb, and NO_COLOR environment variables.
func Render(w io.Writer, version string, width int) {
	// SCRIBE_NO_BANNER: suppress entirely.
	if os.Getenv("SCRIBE_NO_BANNER") != "" {
		return
	}

	// TERM=dumb: plain text only, no block characters.
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	// Select logo size based on terminal width.
	var art string
	switch {
	case width >= 60:
		art = logoFull
	case width >= 40:
		art = logoCompact
	default:
		fmt.Fprintf(w, "Scribe v%s\n", version)
		return
	}

	noColor := os.Getenv("NO_COLOR") != ""
	lines := strings.Split(art, "\n")

	if noColor {
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	} else {
		colors := gradient(len(lines))
		for i, line := range lines {
			style := lipgloss.NewStyle().Foreground(colors[i]).Bold(true)
			fmt.Fprintln(w, style.Render(line))
		}
	}

	// Version below the logo, dimmed.
	if noColor {
		fmt.Fprintf(w, "v%s\n", version)
	} else {
		dim := lipgloss.NewStyle().Faint(true)
		fmt.Fprintln(w, dim.Render(fmt.Sprintf("v%s", version)))
	}
	fmt.Fprintln(w)
}

// gradient returns a slice of colors for per-line logo rendering.
// Uses dark or light palette based on terminal background detection.
func gradient(n int) []lipgloss.Color {
	var start, end string

	isDark, _ := lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	if isDark {
		start, end = "#00B4D8", "#60E890"
	} else {
		start, end = "#0077B6", "#2D6A4F"
	}

	blended := lipgloss.Blend1D(n, lipgloss.Color(start), lipgloss.Color(end))
	colors := make([]lipgloss.Color, n)
	for i, c := range blended {
		colors[i] = lipgloss.Color(lipgloss.FromColor(c))
	}
	return colors
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/logo/... -v
```

Expected: all tests pass. The `TestRenderNoColor` test verifies no ANSI escapes are present. The `TestRenderDumbTerminal` test verifies plain text fallback.

Note: if `lipgloss.Blend1D` returns `[]color.Color` instead of `[]lipgloss.Color`, or `lipgloss.FromColor` doesn't exist, adjust the `gradient()` function to use the actual Lip Gloss v2 API. The key contract is: produce `n` colors interpolated between start and end. Check `charm.land/lipgloss/v2` docs if the API differs.

- [ ] **Step 3: Fix any test failures and verify all pass**

```bash
go test ./internal/logo/... -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/logo/logo.go
git commit -m "feat: add logo package with ANSI Shadow art and gradient rendering"
```

---

### Task 4: Root command changes

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Update root.go**

Replace the entire contents of `cmd/root.go` with:

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:          "scribe",
	Short:        "Team skill sync for AI coding agents",
	Long:         "Scribe syncs AI coding agent skills across your team via a shared GitHub loadout.",
	Version:      Version,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runHub,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().Bool("json", false, "Output machine-readable JSON")

	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(guideCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(migrateCmd)
}
```

- [ ] **Step 2: Verify it compiles (runHub doesn't exist yet, so expect error)**

```bash
go build ./cmd/scribe 2>&1
```

Expected: compilation error — `runHub` undefined. This is expected; Task 5 will define it.

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "feat: add RunE, NoArgs, SilenceUsage, and --json flag to root command"
```

---

### Task 5: Root hub — tests first

**Files:**
- Create: `cmd/root_hub_test.go`

- [ ] **Step 1: Write tests for hub JSON output and status formatting**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

type hubStatus struct {
	Version        string   `json:"version"`
	Registries     []string `json:"registries"`
	InstalledCount int      `json:"installed_count"`
	LastSync       string   `json:"last_sync,omitempty"`
	PendingUpdates int      `json:"pending_updates"`
	StaleStatus    bool     `json:"stale_status"`
}

func TestHubJSONOutput(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "ArtistfyHQ/team-skills", Enabled: true},
			{Repo: "disabled/repo", Enabled: false},
		},
	}
	st := &state.State{
		LastSync: time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
		Installed: map[string]state.InstalledSkill{
			"deploy": {},
			"lint":   {},
		},
	}

	var buf bytes.Buffer
	err := writeHubJSON(&buf, "1.0.0", cfg, st)
	if err != nil {
		t.Fatalf("writeHubJSON error: %v", err)
	}

	var got hubStatus
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if got.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", got.Version, "1.0.0")
	}
	if len(got.Registries) != 1 {
		t.Errorf("registries: got %d, want 1 (only enabled)", len(got.Registries))
	}
	if got.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("registry: got %q, want %q", got.Registries[0], "ArtistfyHQ/team-skills")
	}
	if got.InstalledCount != 2 {
		t.Errorf("installed_count: got %d, want 2", got.InstalledCount)
	}
	if got.LastSync != "2026-04-06T10:00:00Z" {
		t.Errorf("last_sync: got %q, want %q", got.LastSync, "2026-04-06T10:00:00Z")
	}
	if !got.StaleStatus {
		t.Error("stale_status: got false, want true")
	}
}

func TestHubJSONNoState(t *testing.T) {
	cfg := &config.Config{}
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := writeHubJSON(&buf, "dev", cfg, st)
	if err != nil {
		t.Fatalf("writeHubJSON error: %v", err)
	}

	var got hubStatus
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got.Version != "dev" {
		t.Errorf("version: got %q, want %q", got.Version, "dev")
	}
	if len(got.Registries) != 0 {
		t.Errorf("registries: got %d, want 0", len(got.Registries))
	}
	if got.InstalledCount != 0 {
		t.Errorf("installed_count: got %d, want 0", got.InstalledCount)
	}
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"minutes", 5 * time.Minute, "5 minutes ago"},
		{"one hour", 1 * time.Hour, "1 hour ago"},
		{"hours", 3 * time.Hour, "3 hours ago"},
		{"one day", 25 * time.Hour, "1 day ago"},
		{"days", 72 * time.Hour, "3 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Now().Add(-tt.ago)
			got := formatRelativeTime(ts)
			if got != tt.want {
				t.Errorf("formatRelativeTime(%v ago): got %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}

func TestFormatRelativeTimeZero(t *testing.T) {
	got := formatRelativeTime(time.Time{})
	if got != "never" {
		t.Errorf("formatRelativeTime(zero): got %q, want %q", got, "never")
	}
}
```

- [ ] **Step 2: Verify tests fail**

```bash
go test ./cmd/... 2>&1 | head -20
```

Expected: compilation error — `writeHubJSON` and `formatRelativeTime` are not defined.

- [ ] **Step 3: Commit**

```bash
git add cmd/root_hub_test.go
git commit -m "test: add root hub JSON output and time formatting tests (red)"
```

---

### Task 6: Root hub — implementation

**Files:**
- Create: `cmd/root_hub.go`

- [ ] **Step 1: Implement the hub**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/logo"
	"github.com/Naoray/scribe/internal/state"
)

func runHub(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	cfg, _ := config.Load()
	st, _ := state.Load()

	// JSON mode: --json flag, non-TTY stdout, or CI environment.
	if jsonFlag || !isatty.IsTerminal(os.Stdout.Fd()) || os.Getenv("CI") != "" {
		return writeHubJSON(os.Stdout, Version, cfg, st)
	}

	// TERM=dumb: plain text, no menu.
	if os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(os.Stdout, "Scribe v%s\n", Version)
		writeStatusPlain(os.Stdout, cfg, st)
		return nil
	}

	// TTY mode: logo + styled status + action menu.
	width, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if width <= 0 {
		width = 80
	}

	logo.Render(os.Stdout, Version, width)
	writeStatusStyled(os.Stdout, cfg, st)

	// Stdin must be a TTY for the interactive menu.
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stdout, "Run 'scribe --help' to see available commands.")
		return nil
	}

	return showActionMenu(cmd)
}

func writeHubJSON(w io.Writer, version string, cfg *config.Config, st *state.State) error {
	repos := cfg.TeamRepos()

	status := struct {
		Version        string   `json:"version"`
		Registries     []string `json:"registries"`
		InstalledCount int      `json:"installed_count"`
		LastSync       string   `json:"last_sync,omitempty"`
		PendingUpdates int      `json:"pending_updates"`
		StaleStatus    bool     `json:"stale_status"`
	}{
		Version:        version,
		Registries:     repos,
		InstalledCount: len(st.Installed),
		PendingUpdates: 0,
		StaleStatus:    true,
	}

	if repos == nil {
		status.Registries = []string{}
	}

	if !st.LastSync.IsZero() {
		status.LastSync = st.LastSync.UTC().Format(time.RFC3339)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func writeStatusPlain(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()
	fmt.Fprintf(w, "Registries: %d connected\n", len(repos))
	fmt.Fprintf(w, "Skills:     %d installed\n", len(st.Installed))
	fmt.Fprintf(w, "Last sync:  %s\n", formatRelativeTime(st.LastSync))
}

func writeStatusStyled(w io.Writer, cfg *config.Config, st *state.State) {
	repos := cfg.TeamRepos()

	labelStyle := lipgloss.NewStyle().Faint(true)
	valueStyle := lipgloss.NewStyle().Bold(true)

	lines := []struct{ label, value string }{
		{"Registries", fmt.Sprintf("%d connected", len(repos))},
		{"Skills", fmt.Sprintf("%d installed", len(st.Installed))},
		{"Last sync", formatRelativeTime(st.LastSync)},
	}

	for _, r := range repos {
		lines = append(lines[:1], append([]struct{ label, value string }{{"", r}}, lines[1:]...)...)
	}

	for _, l := range lines {
		if l.label == "" {
			fmt.Fprintf(w, "  %s  %s\n", labelStyle.Render("         "), valueStyle.Render(l.value))
		} else {
			fmt.Fprintf(w, "  %s  %s\n", labelStyle.Render(fmt.Sprintf("%9s", l.label)), valueStyle.Render(l.value))
		}
	}
	fmt.Fprintln(w)
}

func showActionMenu(cmd *cobra.Command) error {
	var action string
	err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(
			huh.NewOption("Sync skills from registries", "sync"),
			huh.NewOption("List installed skills", "list"),
			huh.NewOption("Connect a registry", "connect"),
			huh.NewOption("Interactive setup guide", "guide"),
			huh.NewOption("Show help", "help"),
		).
		Value(&action).
		Run()

	if err != nil {
		// Ctrl+C or other interrupt — exit cleanly.
		if err == huh.ErrUserAborted {
			os.Exit(130)
		}
		return err
	}

	// Execute the selected command through the full Cobra lifecycle.
	rootCmd.SetArgs([]string{action})
	return rootCmd.Execute()
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./cmd/... -v -run "TestHub|TestFormat"
```

Expected: all hub tests pass.

- [ ] **Step 3: Run full test suite to check nothing is broken**

```bash
go test ./... 2>&1
```

Expected: all tests pass.

- [ ] **Step 4: Verify the build compiles**

```bash
go build ./cmd/scribe
```

Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/root_hub.go
git commit -m "feat: add interactive root hub with logo, status, and action menu"
```

---

### Task 7: Manual smoke test and polish

**Files:**
- None (verification only)

- [ ] **Step 1: Test interactive mode**

```bash
go run ./cmd/scribe
```

Expected: logo with gradient, status summary, action menu. Select "Show help" to verify command execution works.

- [ ] **Step 2: Test JSON mode**

```bash
go run ./cmd/scribe --json
```

Expected: JSON object with version, registries, installed_count, last_sync, pending_updates, stale_status.

- [ ] **Step 3: Test piped output (non-TTY)**

```bash
go run ./cmd/scribe | cat
```

Expected: JSON output (stdout is not a TTY when piped).

- [ ] **Step 4: Test NO_COLOR**

```bash
NO_COLOR=1 go run ./cmd/scribe
```

Expected: logo with block characters but no color. Menu still works.

- [ ] **Step 5: Test SCRIBE_NO_BANNER**

```bash
SCRIBE_NO_BANNER=1 go run ./cmd/scribe
```

Expected: no logo, status and menu still appear.

- [ ] **Step 6: Test TERM=dumb**

```bash
TERM=dumb go run ./cmd/scribe
```

Expected: plain text "Scribe vdev", status lines, no menu.

- [ ] **Step 7: Test that subcommands still work**

```bash
go run ./cmd/scribe list --help
go run ./cmd/scribe sync --help
```

Expected: normal help output, no logo.

- [ ] **Step 8: Test unknown args are rejected**

```bash
go run ./cmd/scribe notacommand 2>&1
```

Expected: error message (not the hub), because `cobra.NoArgs` is set.

- [ ] **Step 9: Fix any issues found during smoke testing**

If any of the above behave unexpectedly, fix the issue and add a test for it.

- [ ] **Step 10: Commit any fixes**

```bash
git add -p  # stage only the fixes
git commit -m "fix: polish hub behavior from smoke testing"
```

Only commit if there were actual fixes. Skip if everything passed.
