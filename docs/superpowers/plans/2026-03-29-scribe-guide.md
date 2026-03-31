# `scribe guide` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an interactive `scribe guide` command that walks users through Scribe setup — prereq checks, path selection, connect/create, sync progress, and summary.

**Architecture:** Hybrid UI — huh v2 forms for input, a reusable Bubble Tea model for sync progress, and lipgloss for styled static output. The guide orchestrates existing functions (`connectToRepo`, create registry logic, `Syncer.Run`) — no reimplementation. Non-TTY/`--json` mode outputs a structured JSON with prereq status and step commands for agents.

**Tech Stack:** Go 1.26.1, Cobra, huh v2, Bubble Tea v2 (`charm.land/bubbletea/v2`), lipgloss v2 (`charm.land/lipgloss/v2`), bubbles v2 spinner

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/prereq/prereq.go` | UI-agnostic prerequisite checks (auth, scribe dir, connections) |
| `internal/prereq/prereq_test.go` | Tests for prereq checks |
| `internal/ui/syncprogress.go` | Bubble Tea model for real-time sync progress display |
| `internal/ui/syncprogress_test.go` | Tests for sync progress model |
| `internal/ui/styles.go` | Shared lipgloss styles for guide output |
| `cmd/guide.go` | Cobra command — orchestrates the full guide flow |
| `cmd/guide_test.go` | Tests for guide JSON output and non-TTY behavior |

---

### Task 1: Prerequisite Checker (`internal/prereq/`)

**Files:**
- Create: `internal/prereq/prereq.go`
- Create: `internal/prereq/prereq_test.go`

This is the UI-agnostic engine that checks what's ready and what's not. It returns data, never prints.

- [ ] **Step 1: Write failing tests for prereq checks**

```go
// internal/prereq/prereq_test.go
package prereq_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/prereq"
)

func TestCheck_NoScribeDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	result := prereq.Check()

	if result.ScribeDir.OK {
		t.Error("expected ScribeDir.OK to be false for missing dir")
	}
}

func TestCheck_NoAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "") // prevent gh CLI from being found

	result := prereq.Check()

	if result.GitHubAuth.OK {
		t.Error("expected GitHubAuth.OK to be false without any auth")
	}
}

func TestCheck_WithToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	result := prereq.Check()

	if !result.GitHubAuth.OK {
		t.Error("expected GitHubAuth.OK to be true with GITHUB_TOKEN")
	}
	if result.GitHubAuth.Method != "GITHUB_TOKEN" {
		t.Errorf("expected method GITHUB_TOKEN, got %s", result.GitHubAuth.Method)
	}
}

func TestCheck_WithConnections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	// Write a config with team_repos
	configDir := home + "/.scribe"
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configDir+"/config.toml", []byte("team_repos = [\"Org/repo\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := prereq.Check()

	if len(result.Connections.Repos) != 1 {
		t.Errorf("expected 1 connection, got %d", len(result.Connections.Repos))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/prereq/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement the prereq checker**

```go
// internal/prereq/prereq.go
package prereq

import (
	"os"
	"os/exec"
	"strings"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

// AuthResult describes the GitHub authentication status.
type AuthResult struct {
	OK     bool   `json:"ok"`
	Method string `json:"method,omitempty"` // "gh_cli", "GITHUB_TOKEN", "config", ""
}

// DirResult describes the ~/.scribe/ directory status.
type DirResult struct {
	OK   bool   `json:"ok"`
	Path string `json:"path"`
}

// ConnectionsResult describes existing team connections.
type ConnectionsResult struct {
	Repos []string `json:"repos,omitempty"`
}

// Result holds all prerequisite check outcomes.
type Result struct {
	GitHubAuth  AuthResult        `json:"github_auth"`
	ScribeDir   DirResult         `json:"scribe_dir"`
	Connections ConnectionsResult `json:"connections"`
}

// Check runs all prerequisite checks and returns the result.
func Check() Result {
	return Result{
		GitHubAuth:  checkAuth(),
		ScribeDir:   checkDir(),
		Connections: checkConnections(),
	}
}

func checkAuth() AuthResult {
	// 1. gh auth token
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return AuthResult{OK: true, Method: "gh_cli"}
		}
	}
	// 2. GITHUB_TOKEN env
	if os.Getenv("GITHUB_TOKEN") != "" {
		return AuthResult{OK: true, Method: "GITHUB_TOKEN"}
	}
	// 3. Config file token
	cfg, err := config.Load()
	if err == nil && cfg.Token != "" {
		return AuthResult{OK: true, Method: "config"}
	}
	return AuthResult{OK: false}
}

func checkDir() DirResult {
	dir, err := state.Dir()
	if err != nil {
		return DirResult{OK: false, Path: "~/.scribe"}
	}
	_, err = os.Stat(dir)
	return DirResult{OK: err == nil, Path: dir}
}

func checkConnections() ConnectionsResult {
	cfg, err := config.Load()
	if err != nil {
		return ConnectionsResult{}
	}
	return ConnectionsResult{Repos: cfg.TeamRepos}
}
```

- [ ] **Step 4: Add missing import to test file**

Add `"os"` to the import block in `prereq_test.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/prereq/ -v`
Expected: PASS (all 4 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/prereq/prereq.go internal/prereq/prereq_test.go
git commit -m "[agent] feat: add prereq checker for scribe guide

Step 1 of task: scribe guide implementation"
```

---

### Task 2: Shared Lipgloss Styles (`internal/ui/styles.go`)

**Files:**
- Create: `internal/ui/styles.go`

Lightweight styles file — no tests needed, it's just style constants.

- [ ] **Step 1: Create styles file**

```go
// internal/ui/styles.go
package ui

import "charm.land/lipgloss/v2"

var (
	// Title is used for section headers like "Scribe Guide".
	Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	// CheckOK renders a passing prereq check.
	CheckOK = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	// CheckFail renders a failing prereq check.
	CheckFail = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4672"))

	// CheckPending renders a neutral/pending check.
	CheckPending = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))

	// Subtle is for secondary information.
	Subtle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))

	// Bold is for emphasis in summaries.
	Bold = lipgloss.NewStyle().Bold(true)

	// Summary wraps the final summary box.
	Summary = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#04B575")).
		Padding(1, 2).
		MarginTop(1)
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/ui/`
Expected: success (no output)

- [ ] **Step 3: Commit**

```bash
git add internal/ui/styles.go
git commit -m "[agent] feat: add shared lipgloss styles for guide UI

Step 2 of task: scribe guide implementation"
```

---

### Task 3: Sync Progress Bubble Tea Model (`internal/ui/syncprogress.go`)

**Files:**
- Create: `internal/ui/syncprogress.go`
- Create: `internal/ui/syncprogress_test.go`

The reusable Bubble Tea model that displays real-time sync progress. Consumes events from `sync.Syncer` via `p.Send()`.

- [ ] **Step 1: Write failing tests for the sync progress model**

```go
// internal/ui/syncprogress_test.go
package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/ui"
)

func TestSyncProgress_ResolvedAddsRow(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	updated, _ := m.Update(sync.SkillResolvedMsg{
		SkillStatus: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing},
	})
	model := updated.(ui.SyncProgress)

	if len(model.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(model.Skills))
	}
	if model.Skills[0].Name != "cleanup" {
		t.Errorf("expected name cleanup, got %s", model.Skills[0].Name)
	}
}

func TestSyncProgress_InstalledUpdatesRow(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	m, _ = m.Update(sync.SkillResolvedMsg{
		SkillStatus: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing},
	})
	updated, _ := m.Update(sync.SkillInstalledMsg{Name: "cleanup", Version: "v2.1.0"})
	model := updated.(ui.SyncProgress)

	if model.Skills[0].State != ui.SkillInstalled {
		t.Errorf("expected state Installed, got %v", model.Skills[0].State)
	}
	if model.Skills[0].Version != "v2.1.0" {
		t.Errorf("expected version v2.1.0, got %s", model.Skills[0].Version)
	}
}

func TestSyncProgress_CompleteQuits(t *testing.T) {
	m := ui.NewSyncProgress("Org/repo")

	_, cmd := m.Update(sync.SyncCompleteMsg{Installed: 3, Skipped: 2})
	if cmd == nil {
		t.Fatal("expected quit command on SyncCompleteMsg")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Implement the sync progress model**

```go
// internal/ui/syncprogress.go
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"github.com/Naoray/scribe/internal/sync"
)

// SkillState tracks the display state of a single skill row.
type SkillState int

const (
	SkillPending     SkillState = iota
	SkillDownloading
	SkillInstalled
	SkillSkipped
	SkillFailed
)

// SkillRow is a single row in the sync progress display.
type SkillRow struct {
	Name    string
	State   SkillState
	Version string
	Targets string
	Error   string
}

// SyncProgress is a Bubble Tea model that displays real-time sync progress.
type SyncProgress struct {
	Registry string
	Skills   []SkillRow
	Summary  sync.SyncCompleteMsg
	Done     bool
	spinner  spinner.Model
}

// NewSyncProgress creates a new sync progress model for the given registry.
func NewSyncProgress(registry string) SyncProgress {
	s := spinner.New()
	s.SetSpinner(spinner.Dot)
	return SyncProgress{
		Registry: registry,
		spinner:  s,
	}
}

func (m SyncProgress) Init() tea.Cmd {
	return m.spinner.Tick()
}

func (m SyncProgress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case tea.InterruptMsg:
		return m, tea.Quit

	case sync.SkillResolvedMsg:
		m.Skills = append(m.Skills, SkillRow{
			Name:  msg.Name,
			State: SkillPending,
		})

	case sync.SkillDownloadingMsg:
		for i := range m.Skills {
			if m.Skills[i].Name == msg.Name {
				m.Skills[i].State = SkillDownloading
				break
			}
		}

	case sync.SkillInstalledMsg:
		for i := range m.Skills {
			if m.Skills[i].Name == msg.Name {
				m.Skills[i].State = SkillInstalled
				m.Skills[i].Version = msg.Version
				break
			}
		}

	case sync.SkillSkippedMsg:
		for i := range m.Skills {
			if m.Skills[i].Name == msg.Name {
				m.Skills[i].State = SkillSkipped
				break
			}
		}

	case sync.SkillErrorMsg:
		for i := range m.Skills {
			if m.Skills[i].Name == msg.Name {
				m.Skills[i].State = SkillFailed
				m.Skills[i].Error = msg.Err.Error()
				break
			}
		}

	case sync.SyncCompleteMsg:
		m.Summary = msg
		m.Done = true
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m SyncProgress) View() tea.View {
	var v tea.View
	var b strings.Builder

	b.WriteString(Title.Render(fmt.Sprintf("Syncing %s", m.Registry)))
	b.WriteString("\n\n")

	installed := 0
	total := len(m.Skills)

	for _, sk := range m.Skills {
		var icon, detail string
		switch sk.State {
		case SkillPending:
			icon = CheckPending.Render("○")
			detail = Subtle.Render("pending")
		case SkillDownloading:
			icon = m.spinner.View()
			detail = Subtle.Render("downloading...")
		case SkillInstalled:
			icon = CheckOK.Render("✓")
			detail = sk.Version
			installed++
		case SkillSkipped:
			icon = Subtle.Render("–")
			detail = Subtle.Render("current")
			installed++
		case SkillFailed:
			icon = CheckFail.Render("✗")
			detail = CheckFail.Render(sk.Error)
		}

		b.WriteString(fmt.Sprintf("  %s %-20s %s\n", icon, sk.Name, detail))
	}

	if total > 0 {
		b.WriteString(fmt.Sprintf("\n  %d/%d skills processed\n", installed, total))
	}

	v.Body = b.String()
	return v
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/ui/syncprogress.go internal/ui/syncprogress_test.go
git commit -m "[agent] feat: add Bubble Tea sync progress model

Step 3 of task: scribe guide implementation
Reusable model that consumes sync.Syncer events for real-time display."
```

---

### Task 4: Extract Shared Helpers from `cmd/connect.go`

**Files:**
- Modify: `cmd/connect.go` — export `connectToRepo` and `parseOwnerRepo` for use by `guide.go`

The guide needs to call `connectToRepo` and `parseOwnerRepo`. They're currently unexported. Since both files are in the same `cmd` package, we only need to verify they're accessible (they already are — same package). However, we need to extract the create-registry logic so the guide can call it without duplicating.

Actually, since `cmd/guide.go` will be in package `cmd`, it can already call `connectToRepo`, `parseOwnerRepo`, `runCreateRegistry`, etc. directly. No extraction needed.

- [ ] **Step 1: Verify existing functions are accessible from package cmd**

Run: `grep -n "^func " cmd/connect.go cmd/create_registry.go`
Expected: `connectToRepo`, `parseOwnerRepo`, `resolveRepo`, `runCreateRegistry`, etc. are all in package `cmd` — accessible to `guide.go`.

- [ ] **Step 2: Commit (skip — no changes needed)**

No commit needed. This task confirmed no extraction is required.

---

### Task 5: Guide JSON Output Mode

**Files:**
- Create: `cmd/guide.go` (JSON/non-TTY path first)
- Create: `cmd/guide_test.go`

Build the non-interactive JSON path first — it's simpler and testable without a terminal.

- [ ] **Step 1: Write failing test for JSON output**

```go
// cmd/guide_test.go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestGuideJSON_NotConnected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	t.Setenv("PATH", os.Getenv("PATH"))

	var buf bytes.Buffer
	err := runGuideJSON(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "not_connected" {
		t.Errorf("expected status not_connected, got %v", result["status"])
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Error("expected non-empty steps array")
	}
}

func TestGuideJSON_AlreadyConnected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	configDir := home + "/.scribe"
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(configDir+"/config.toml", []byte("team_repos = [\"Org/repo\"]\n"), 0o644)

	var buf bytes.Buffer
	err := runGuideJSON(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "connected" {
		t.Errorf("expected status connected, got %v", result["status"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestGuideJSON -v`
Expected: FAIL — `runGuideJSON` not defined

- [ ] **Step 3: Implement the guide command with JSON path**

```go
// cmd/guide.go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/prereq"
)

var guideJSON bool

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Interactive setup guide for Scribe",
	Long: `Walk through Scribe setup step by step.

Run with --json or pipe to get machine-readable steps for agents.

Examples:
  scribe guide          # interactive setup
  scribe guide --json   # agent-friendly step list`,
	Args: cobra.NoArgs,
	RunE: runGuide,
}

func init() {
	guideCmd.Flags().BoolVar(&guideJSON, "json", false, "Output machine-readable JSON (for CI/agents)")
}

func runGuide(cmd *cobra.Command, args []string) error {
	useJSON := guideJSON || !isatty.IsTerminal(os.Stdout.Fd())
	if useJSON {
		return runGuideJSON(os.Stdout)
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("scribe guide requires an interactive terminal — use --json for agent-friendly output")
	}

	return runGuideInteractive()
}

// runGuideJSON writes the guide steps as JSON to w.
func runGuideJSON(w io.Writer) error {
	result := prereq.Check()

	status := "not_connected"
	if len(result.Connections.Repos) > 0 {
		status = "connected"
	}

	type step struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}

	var steps []step

	if !result.GitHubAuth.OK {
		steps = append(steps, step{
			Command:     "gh auth login",
			Description: "Authenticate with GitHub",
		})
	}

	if len(result.Connections.Repos) == 0 {
		steps = append(steps, step{
			Command:     "scribe connect <owner/repo>",
			Description: "Connect to your team's skill registry",
		})
	}

	steps = append(steps, step{
		Command:     "scribe sync",
		Description: "Sync skills to your local machine",
	})

	steps = append(steps, step{
		Command:     "scribe list",
		Description: "Verify installed skills",
	})

	return json.NewEncoder(w).Encode(map[string]any{
		"status":        status,
		"prerequisites": result,
		"steps":         steps,
	})
}

// runGuideInteractive runs the full interactive guide flow.
// Implemented in Task 6.
func runGuideInteractive() error {
	return fmt.Errorf("interactive guide not yet implemented")
}
```

- [ ] **Step 4: Register the guide command in root.go**

In `cmd/root.go`, add `guideCmd` to the `init()` function:

```go
func init() {
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(guideCmd)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestGuideJSON -v`
Expected: PASS

- [ ] **Step 6: Verify the command registers**

Run: `go run ./cmd/scribe guide --help`
Expected: shows guide help text

- [ ] **Step 7: Commit**

```bash
git add cmd/guide.go cmd/guide_test.go cmd/root.go
git commit -m "[agent] feat: add scribe guide command with JSON output

Step 5 of task: scribe guide implementation
Non-TTY/--json path outputs prereq status and setup steps for agents."
```

---

### Task 6: Interactive Guide Flow (`runGuideInteractive`)

**Files:**
- Modify: `cmd/guide.go` — replace stub `runGuideInteractive` with full flow

This is the main interactive path: prereq display → path selection → connect/create → sync progress → summary.

- [ ] **Step 1: Implement the prereq display helper**

Add to `cmd/guide.go`:

```go
func displayPrereqs(result prereq.Result) {
	fmt.Println()
	fmt.Println(ui.Title.Render("Scribe Guide"))
	fmt.Println()

	if result.GitHubAuth.OK {
		fmt.Printf("  %s GitHub authenticated (%s)\n", ui.CheckOK.Render("✓"), result.GitHubAuth.Method)
	} else {
		fmt.Printf("  %s GitHub not authenticated\n", ui.CheckFail.Render("✗"))
	}

	if result.ScribeDir.OK {
		fmt.Printf("  %s Scribe directory exists\n", ui.CheckOK.Render("✓"))
	} else {
		fmt.Printf("  %s Scribe directory will be created\n", ui.CheckPending.Render("○"))
	}

	if len(result.Connections.Repos) > 0 {
		fmt.Printf("  %s Connected to %d registry\n", ui.CheckOK.Render("✓"), len(result.Connections.Repos))
	} else {
		fmt.Printf("  %s No team registries connected\n", ui.CheckPending.Render("○"))
	}

	fmt.Println()
}
```

- [ ] **Step 2: Implement auth retry loop**

Add to `cmd/guide.go`:

```go
func waitForAuth() error {
	for {
		fmt.Println(ui.Subtle.Render("  To authenticate, run one of:"))
		fmt.Println(ui.Subtle.Render("    • gh auth login"))
		fmt.Println(ui.Subtle.Render("    • export GITHUB_TOKEN=<your-token>"))
		fmt.Println()

		var retry bool
		if err := huh.NewConfirm().Title("Ready to re-check?").Value(&retry).Run(); err != nil {
			return err
		}
		if !retry {
			return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
		}

		result := prereq.Check()
		if result.GitHubAuth.OK {
			fmt.Printf("  %s GitHub authenticated (%s)\n\n", ui.CheckOK.Render("✓"), result.GitHubAuth.Method)
			return nil
		}
		fmt.Printf("  %s Still not authenticated\n\n", ui.CheckFail.Render("✗"))
	}
}
```

- [ ] **Step 3: Implement path selection and connect flow**

Add to `cmd/guide.go`:

```go
func guideConnect(cfg *config.Config, client *gh.Client) (string, error) {
	repo, err := resolveRepo(nil) // triggers huh prompt
	if err != nil {
		return "", err
	}

	if err := connectToRepo(repo, cfg, client); err != nil {
		return "", err
	}

	return repo, nil
}
```

Wait — `connectToRepo` already does its own sync with `fmt.Printf` output. For the guide, we want to use the Bubble Tea sync progress instead. We need to split the connect step from the sync step.

- [ ] **Step 3 (revised): Implement connect-without-sync helper**

Add to `cmd/guide.go`:

```go
// connectOnly performs the connect workflow without syncing.
// Returns the validated repo slug.
func connectOnly(repo string, cfg *config.Config, client *gh.Client) error {
	owner, name, err := parseOwnerRepo(repo)
	if err != nil {
		return err
	}

	for _, existing := range cfg.TeamRepos {
		if strings.EqualFold(existing, repo) {
			fmt.Printf("  Already connected to %s\n", existing)
			return nil
		}
	}

	ctx := context.Background()
	raw, err := client.FetchFile(ctx, owner, name, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("could not access %s: %w", repo, err)
	}

	m, err := manifest.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid scribe.toml in %s: %w", repo, err)
	}
	if !m.IsLoadout() {
		return fmt.Errorf("%s/scribe.toml has no [team] section — is this a skill package?", repo)
	}

	cfg.TeamRepos = append(cfg.TeamRepos, repo)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("  Connected to %s\n\n", repo)
	return nil
}
```

- [ ] **Step 4: Implement the Bubble Tea sync runner**

Add to `cmd/guide.go`:

```go
func runSyncWithProgress(repo string, cfg *config.Config, client *gh.Client) (sync.SyncCompleteMsg, error) {
	st, err := state.Load()
	if err != nil {
		return sync.SyncCompleteMsg{}, err
	}

	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	model := ui.NewSyncProgress(repo)
	p := tea.NewProgram(model)

	syncer := &syncsvc.Syncer{
		Client:  client,
		Targets: tgts,
		Emit:    func(msg any) { p.Send(msg) },
	}

	// Run sync in background, sending events to the Bubble Tea program.
	go func() {
		if err := syncer.Run(context.Background(), repo, st); err != nil {
			p.Send(sync.SkillErrorMsg{Name: "sync", Err: err})
			p.Send(sync.SyncCompleteMsg{Failed: 1})
		}
	}()

	finalModel, err := p.Run()
	if err != nil {
		return sync.SyncCompleteMsg{}, fmt.Errorf("TUI error: %w", err)
	}

	if fm, ok := finalModel.(ui.SyncProgress); ok {
		return fm.Summary, nil
	}
	return sync.SyncCompleteMsg{}, nil
}
```

- [ ] **Step 5: Implement the summary display**

Add to `cmd/guide.go`:

```go
func displaySummary(repo string, summary sync.SyncCompleteMsg, path string) {
	total := summary.Installed + summary.Updated + summary.Skipped
	var content strings.Builder

	content.WriteString(ui.Bold.Render("All set!"))
	content.WriteString("\n\n")
	content.WriteString(fmt.Sprintf("  Registry    %s\n", repo))
	content.WriteString(fmt.Sprintf("  Skills      %d installed, %d current, %d failed\n", summary.Installed+summary.Updated, summary.Skipped, summary.Failed))
	content.WriteString(fmt.Sprintf("  Targets     claude, cursor\n"))
	content.WriteString("\n")
	content.WriteString(ui.Bold.Render("  What's next:"))
	content.WriteString("\n")

	switch path {
	case "join":
		content.WriteString("  • scribe sync       Keep skills up to date\n")
		content.WriteString("  • scribe list       See installed skills and status\n")
	case "create":
		content.WriteString("  • scribe add        Add skills to your registry\n")
		content.WriteString("  • scribe list       See installed skills and status\n")
	}

	_ = total // suppress unused warning
	content.WriteString("  • scribe guide      Run this guide again anytime\n")

	fmt.Println(ui.Summary.Render(content.String()))
}
```

- [ ] **Step 6: Wire it all together in runGuideInteractive**

Replace the stub `runGuideInteractive` in `cmd/guide.go`:

```go
func runGuideInteractive() error {
	result := prereq.Check()
	displayPrereqs(result)

	// Auth gate — loop until authenticated.
	if !result.GitHubAuth.OK {
		if err := waitForAuth(); err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := gh.NewClient(cfg.Token)

	// Build path options based on current state.
	type pathOption struct {
		Label string
		Value string
	}
	options := []huh.Option[string]{
		huh.NewOption("Join an existing team", "join"),
		huh.NewOption("Create a new skill registry", "create"),
	}
	if len(result.Connections.Repos) > 0 {
		options = append(options, huh.NewOption("View my current setup", "view"))
	}

	var chosen string
	if err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(options...).
		Value(&chosen).
		Run(); err != nil {
		return err
	}

	switch chosen {
	case "join":
		repo, err := resolveRepo(nil)
		if err != nil {
			return err
		}
		if err := connectOnly(repo, cfg, client); err != nil {
			return err
		}
		summary, err := runSyncWithProgress(repo, cfg, client)
		if err != nil {
			return err
		}
		displaySummary(repo, summary, "join")

	case "create":
		// Delegate to create registry — it already handles prompts and calls connectToRepo.
		// After it completes, show summary.
		if err := runCreateRegistry(createRegistryCmd, nil); err != nil {
			return err
		}
		// Reload config to get the newly connected repo.
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		if len(cfg.TeamRepos) > 0 {
			repo := cfg.TeamRepos[len(cfg.TeamRepos)-1]
			displaySummary(repo, sync.SyncCompleteMsg{}, "create")
		}

	case "view":
		return runList(listCmd, nil)
	}

	return nil
}
```

- [ ] **Step 7: Update imports in guide.go**

Ensure the import block includes all needed packages:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/prereq"
	"github.com/Naoray/scribe/internal/state"
	syncsvc "github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
	"github.com/Naoray/scribe/internal/ui"
)
```

Note: `sync` is aliased to `syncsvc` to avoid conflict with the stdlib `sync` package.

- [ ] **Step 8: Verify everything compiles**

Run: `go build ./...`
Expected: success

- [ ] **Step 9: Run all tests**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add cmd/guide.go
git commit -m "[agent] feat: implement interactive guide flow

Step 6 of task: scribe guide implementation
Full interactive flow: prereqs, path selection, connect/create, sync progress, summary."
```

---

### Task 7: Promote Bubble Tea Dependencies to Direct

**Files:**
- Modify: `go.mod`

Bubble Tea and lipgloss are currently indirect dependencies (pulled in by huh). Since we now import them directly, they should be direct.

- [ ] **Step 1: Run go mod tidy**

Run: `go mod tidy`
Expected: `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` and `charm.land/bubbles/v2` move from `// indirect` to direct in `go.mod`.

- [ ] **Step 2: Verify**

Run: `grep -E "bubbletea|lipgloss|bubbles" go.mod`
Expected: all three show without `// indirect` comment.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "[agent] chore: promote Charm deps to direct imports

Step 7 of task: scribe guide implementation"
```

---

### Task 8: Manual Smoke Test

No code changes — this is a verification step.

- [ ] **Step 1: Test JSON mode**

Run: `go run ./cmd/scribe guide --json | jq .`
Expected: JSON with status, prerequisites, and steps array.

- [ ] **Step 2: Test help text**

Run: `go run ./cmd/scribe guide --help`
Expected: shows guide description and --json flag.

- [ ] **Step 3: Test non-TTY detection**

Run: `go run ./cmd/scribe guide | cat`
Expected: JSON output (auto-detected non-TTY).

- [ ] **Step 4: Test interactive mode (if possible)**

Run: `go run ./cmd/scribe guide`
Expected: shows prereq checklist, then path selection prompt.

- [ ] **Step 5: Commit (skip — no changes)**

No commit needed. Verification only.
