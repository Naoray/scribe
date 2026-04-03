# Registry List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `scribe registry list` to show connected registries with skill counts and relative sync time.

**Architecture:** New `registry` parent command (extending existing `cmd/registry.go`) with a `list` subcommand. Workflow step in `internal/workflow/registry_list.go` reuses `LoadConfig`, `LoadState`, `MigrateRegistries`, then renders styled text or JSON.

**Tech Stack:** Go, Cobra, lipgloss, tabwriter (existing stack)

**Spec:** `docs/superpowers/specs/2026-04-03-registry-list-design.md`

---

### Task 1: Relative Time Helper + Tests

**Files:**
- Create: `internal/workflow/timeago.go`
- Create: `internal/workflow/timeago_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/workflow/timeago_test.go
package workflow

import (
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero value", time.Time{}, "never synced"},
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes ago", now.Add(-25 * time.Minute), "25 minutes ago"},
		{"1 minute ago", now.Add(-90 * time.Second), "1 minute ago"},
		{"hours ago", now.Add(-3 * time.Hour), "3 hours ago"},
		{"1 hour ago", now.Add(-90 * time.Minute), "1 hour ago"},
		{"days ago", now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{"1 day ago", now.Add(-36 * time.Hour), "1 day ago"},
		{"old date", now.Add(-45 * 24 * time.Hour), now.Add(-45 * 24 * time.Hour).Format("2006-01-02")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := timeAgo(c.t)
			if got != c.want {
				t.Errorf("timeAgo(%v) = %q, want %q", c.t, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/workflow/ -run TestTimeAgo -v`
Expected: FAIL — `timeAgo` not defined

- [ ] **Step 3: Write the implementation**

```go
// internal/workflow/timeago.go
package workflow

import (
	"fmt"
	"time"
)

// timeAgo returns a human-readable relative time string.
// Returns "never synced" for the zero value.
func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never synced"
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
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("2006-01-02")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/workflow/ -run TestTimeAgo -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/timeago.go internal/workflow/timeago_test.go
git commit -m "[agent] feat: add relative time helper for registry list

Step 1 of task: registry list command"
```

---

### Task 2: Skill Counting Logic + Tests

**Files:**
- Create: `internal/workflow/registry_list.go`
- Create: `internal/workflow/registry_list_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/workflow/registry_list_test.go
package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

func TestCountSkillsPerRegistry(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"browse": {Registries: []string{"ArtistfyHQ/skills"}},
			"deploy": {Registries: []string{"ArtistfyHQ/skills", "Naoray/my-skills"}},
			"lint":   {Registries: []string{"Naoray/my-skills"}},
			"orphan": {Registries: nil},
			"empty":  {Registries: []string{}},
		},
	}

	cases := []struct {
		name     string
		repos    []string
		wantMap  map[string]int
	}{
		{
			"multi-registry counts",
			[]string{"ArtistfyHQ/skills", "Naoray/my-skills"},
			map[string]int{"ArtistfyHQ/skills": 2, "Naoray/my-skills": 2},
		},
		{
			"case-insensitive match",
			[]string{"artistfyhq/skills"},
			map[string]int{"artistfyhq/skills": 2},
		},
		{
			"no matching skills",
			[]string{"unknown/repo"},
			map[string]int{"unknown/repo": 0},
		},
		{
			"empty repos",
			[]string{},
			map[string]int{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := workflow.CountSkillsPerRegistry(c.repos, st)
			for repo, want := range c.wantMap {
				if got[repo] != want {
					t.Errorf("repo %q: got %d, want %d", repo, got[repo], want)
				}
			}
		})
	}
}

func TestRegistryListSteps_Composition(t *testing.T) {
	steps := workflow.RegistryListSteps()
	if len(steps) == 0 {
		t.Fatal("RegistryListSteps() returned empty list")
	}

	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "PrintRegistryList" {
		t.Errorf("expected last step PrintRegistryList, got %s", steps[len(steps)-1].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/workflow/ -run "TestCountSkillsPerRegistry|TestRegistryListSteps" -v`
Expected: FAIL — `CountSkillsPerRegistry` and `RegistryListSteps` not defined

- [ ] **Step 3: Write the implementation**

```go
// internal/workflow/registry_list.go
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/state"
)

// RegistryListSteps returns the step list for the registry list command.
func RegistryListSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"MigrateRegistries", stepMigrateRegistriesSafe},
		{"PrintRegistryList", StepPrintRegistryList},
	}
}

// stepMigrateRegistriesSafe wraps StepMigrateRegistries with a guard
// for empty TeamRepos (avoids index-out-of-range on TeamRepos[0]).
func stepMigrateRegistriesSafe(_ context.Context, b *Bag) error {
	if len(b.Config.TeamRepos) == 0 {
		return nil
	}
	return StepMigrateRegistries(nil, b)
}

// CountSkillsPerRegistry counts installed skills per registry.
func CountSkillsPerRegistry(repos []string, st *state.State) map[string]int {
	counts := make(map[string]int, len(repos))
	for _, repo := range repos {
		counts[repo] = 0
	}
	for _, skill := range st.Installed {
		for _, repo := range repos {
			for _, r := range skill.Registries {
				if strings.EqualFold(r, repo) {
					counts[repo]++
					break
				}
			}
		}
	}
	return counts
}

// list styles
var (
	regNameStyle  = lipgloss.NewStyle().Bold(true)
	regCountStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	regFootStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// StepPrintRegistryList renders connected registries as styled text or JSON.
func StepPrintRegistryList(_ context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	repos := b.Config.TeamRepos

	if len(repos) == 0 {
		if useJSON {
			return printRegistryJSON(w, nil, b.State)
		}
		fmt.Fprintln(w, "No registries connected.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Connect a registry:  scribe connect <owner/repo>")
		return nil
	}

	counts := CountSkillsPerRegistry(repos, b.State)

	if useJSON {
		return printRegistryJSON(w, repos, b.State)
	}
	return printRegistryTable(w, repos, counts, b.State)
}

func printRegistryTable(w io.Writer, repos []string, counts map[string]int, st *state.State) error {
	for _, repo := range repos {
		count := regCountStyle.Render(fmt.Sprintf("(%d)", counts[repo]))
		fmt.Fprintf(w, "%s %s\n", regNameStyle.Render(repo), count)
	}

	fmt.Fprintln(w)

	footer := fmt.Sprintf("%d registries connected", len(repos))
	if len(repos) == 1 {
		footer = "1 registry connected"
	}
	if st.Team.LastSync.IsZero() {
		footer += " · never synced"
	} else {
		footer += " · last sync " + timeAgo(st.Team.LastSync)
	}

	fmt.Fprintln(w, regFootStyle.Render(footer))
	return nil
}

type registryJSON struct {
	Registry   string `json:"registry"`
	SkillCount int    `json:"skill_count"`
}

type registryListJSON struct {
	Registries []registryJSON `json:"registries"`
	LastSync   *string        `json:"last_sync"`
}

func printRegistryJSON(w io.Writer, repos []string, st *state.State) error {
	counts := CountSkillsPerRegistry(repos, st)

	entries := make([]registryJSON, 0, len(repos))
	for _, repo := range repos {
		entries = append(entries, registryJSON{
			Registry:   repo,
			SkillCount: counts[repo],
		})
	}

	var lastSync *string
	if !st.Team.LastSync.IsZero() {
		s := st.Team.LastSync.UTC().Format("2006-01-02T15:04:05Z")
		lastSync = &s
	}

	out := registryListJSON{
		Registries: entries,
		LastSync:   lastSync,
	}

	// Ensure empty slice renders as [] not null.
	if out.Registries == nil {
		out.Registries = []registryJSON{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/workflow/ -run "TestCountSkillsPerRegistry|TestRegistryListSteps" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/registry_list.go internal/workflow/registry_list_test.go
git commit -m "[agent] feat: add registry list workflow step and skill counting

Step 2 of task: registry list command"
```

---

### Task 3: Command Wiring

**Files:**
- Modify: `cmd/registry.go` — add `registryCmd` parent command
- Create: `cmd/registry_list.go` — list subcommand
- Modify: `cmd/root.go` — register `registryCmd`

- [ ] **Step 1: Add the parent command to `cmd/registry.go`**

Add at the top of the file (before existing helpers):

```go
var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage connected skill registries",
	RunE:  runRegistryList,
	Args:  cobra.NoArgs,
}
```

Add the import for `"github.com/spf13/cobra"` to the existing import block. Add an `init` function:

```go
func init() {
	registryCmd.AddCommand(registryListCmd)
}
```

- [ ] **Step 2: Create `cmd/registry_list.go`**

```go
// cmd/registry_list.go
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/workflow"
)

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show connected registries",
	Args:  cobra.NoArgs,
	RunE:  runRegistryList,
}

func init() {
	registryListCmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	bag := &workflow.Bag{
		JSONFlag: jsonFlag,
	}

	return workflow.Run(cmd.Context(), workflow.RegistryListSteps(), bag)
}
```

- [ ] **Step 3: Register in `cmd/root.go`**

Add `registryCmd` to the `init()` function in `cmd/root.go`:

```go
rootCmd.AddCommand(registryCmd)
```

- [ ] **Step 4: Verify it compiles and runs**

Run: `go build ./... && go run ./cmd/scribe registry list --help`
Expected: Shows help for `registry list` with `--json` flag

Run: `go run ./cmd/scribe registry --help`
Expected: Shows help for `registry` with `list` subcommand

- [ ] **Step 5: Verify bare `scribe registry` delegates to list**

Run: `go run ./cmd/scribe registry`
Expected: Same output as `scribe registry list` (either registry table or "No registries connected")

- [ ] **Step 6: Commit**

```bash
git add cmd/registry.go cmd/registry_list.go cmd/root.go
git commit -m "[agent] feat: wire scribe registry list command

Step 3 of task: registry list command"
```

---

### Task 4: Integration Smoke Test

**Files:**
- Modify: `internal/workflow/registry_list_test.go` — add output tests

- [ ] **Step 1: Write JSON output shape test**

Add to `internal/workflow/registry_list_test.go`:

```go
func TestPrintRegistryList_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"browse": {Registries: []string{"Foo/bar"}},
			"deploy": {Registries: []string{"Foo/bar"}},
		},
	}

	repos := []string{"Foo/bar"}

	var buf bytes.Buffer
	counts := workflow.CountSkillsPerRegistry(repos, st)

	// Verify counts are correct.
	if counts["Foo/bar"] != 2 {
		t.Fatalf("expected 2 skills for Foo/bar, got %d", counts["Foo/bar"])
	}
}

func TestPrintRegistryList_EmptyRepos(t *testing.T) {
	repos := []string{}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{},
	}
	counts := workflow.CountSkillsPerRegistry(repos, st)
	if len(counts) != 0 {
		t.Fatalf("expected empty map, got %v", counts)
	}
}
```

Add `"bytes"` to the import block.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/workflow/ -run "TestPrintRegistryList" -v`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/workflow/registry_list_test.go
git commit -m "[agent] test: add integration tests for registry list

Step 4 of task: registry list command"
```

---

### Task 5: Manual Verification + Final Cleanup

- [ ] **Step 1: Test with real config (if registries connected)**

Run: `go run ./cmd/scribe registry list`
Expected: Styled output like:
```
ArtistfyHQ/skills (12)
Naoray/my-skills (3)

2 registries connected · last sync 2 hours ago
```

- [ ] **Step 2: Test JSON output**

Run: `go run ./cmd/scribe registry list --json`
Expected: JSON with `{"registries": [...], "last_sync": "..."}`

- [ ] **Step 3: Test piped output auto-JSON**

Run: `go run ./cmd/scribe registry list | cat`
Expected: Same JSON as `--json`

- [ ] **Step 4: Test bare `scribe registry`**

Run: `go run ./cmd/scribe registry`
Expected: Same as `scribe registry list`

- [ ] **Step 5: Run linter if available**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 6: Final commit if any cleanup needed**

Only if changes were made during manual verification.
