# Local Skill Inventory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--local` flag to `scribe list` that shows locally installed skills from state.json, and make `scribe list` fall back to local view when no registries are connected.

**Architecture:** Single file change to `cmd/list.go`. Add `--local` flag, a `printLocalTable()` function, a `printLocalJSON()` function, and change the no-registry error path to call the local renderer instead. All data comes from `state.Load()`.

**Tech Stack:** Go, Cobra, text/tabwriter, encoding/json, go-isatty

---

## File Structure

- **Modify:** `cmd/list.go` — add `--local` flag, local rendering functions, fallback logic
- **Create:** `cmd/list_test.go` — tests for local view rendering and flag interactions

---

### Task 1: Add `--local` flag and mutually exclusive constraint

**Files:**
- Modify: `cmd/list.go:22-34`

- [ ] **Step 1: Add the flag variable and register it**

In `cmd/list.go`, add the flag variable next to `listJSON`:

```go
var (
	listJSON  bool
	listLocal bool
)
```

In `init()`, register the flag and mark mutual exclusion:

```go
func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
	listCmd.Flags().BoolVar(&listLocal, "local", false, "Show locally installed skills (offline, no registry needed)")
	listCmd.Flags().StringVar(&registryFlag, "registry", "", "Show only this registry (owner/repo or repo name)")
	listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
	listCmd.Flags().MarkHidden("all")
	listCmd.MarkFlagsMutuallyExclusive("local", "registry")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: clean build, no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/list.go
git commit -m "feat(list): add --local flag with --registry mutual exclusion"
```

---

### Task 2: Write the `printLocalTable` function (TDD)

**Files:**
- Create: `cmd/list_test.go`
- Modify: `cmd/list.go`

- [ ] **Step 1: Write the failing test for table output with skills**

Create `cmd/list_test.go`:

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestPrintLocalTable_WithSkills(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version:   "v0.12.9.0",
				Source:    "github:garrytan/gstack@v0.12.9.0",
				Targets:  []string{"claude", "cursor"},
				Registries: []string{"ArtistfyHQ/team-skills"},
			},
			"deploy": {
				Version:   "main",
				CommitSHA: "e4f8a2d1234567",
				Source:    "github:ArtistfyHQ/team-skills@main",
				Targets:  []string{"claude"},
				Registries: []string{"ArtistfyHQ/team-skills"},
			},
		},
	}

	var buf bytes.Buffer
	err := printLocalTable(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()

	// Header present.
	if !strings.Contains(out, "SKILL") || !strings.Contains(out, "VERSION") {
		t.Errorf("missing table header, got:\n%s", out)
	}

	// Skills are sorted alphabetically: deploy before gstack.
	deployIdx := strings.Index(out, "deploy")
	gstackIdx := strings.Index(out, "gstack")
	if deployIdx == -1 || gstackIdx == -1 {
		t.Fatalf("missing skill names, got:\n%s", out)
	}
	if deployIdx > gstackIdx {
		t.Errorf("skills not sorted alphabetically: deploy at %d, gstack at %d", deployIdx, gstackIdx)
	}

	// Version display: branch ref shows sha.
	if !strings.Contains(out, "main@e4f8a2d") {
		t.Errorf("expected branch@sha version display, got:\n%s", out)
	}

	// Source column strips @ref.
	if !strings.Contains(out, "github:garrytan/gstack") {
		t.Errorf("expected stripped source, got:\n%s", out)
	}
	// Should NOT contain the full @ref in the source column (version is separate).
	// Check that the source doesn't appear with @v0.12.9.0 by looking at the gstack line.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "gstack") && strings.Contains(line, "github:garrytan/gstack@v0.12.9.0") {
			t.Errorf("source column should strip @ref, got line: %s", line)
		}
	}

	// Targets column.
	if !strings.Contains(out, "claude, cursor") {
		t.Errorf("expected targets 'claude, cursor', got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestPrintLocalTable_WithSkills -v`
Expected: FAIL — `printLocalTable` not defined

- [ ] **Step 3: Write the `printLocalTable` function**

Add to `cmd/list.go`:

```go
// printLocalTable renders the local skill inventory as a table.
func printLocalTable(w io.Writer, st *state.State) error {
	if len(st.Installed) == 0 {
		fmt.Fprintln(w, "No skills installed.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Install skills from a registry:  scribe connect <owner/repo>")
		return nil
	}

	names := make([]string, 0, len(st.Installed))
	for name := range st.Installed {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SKILL\tVERSION\tTARGETS\tSOURCE")

	for _, name := range names {
		skill := st.Installed[name]
		source, _, _ := strings.Cut(skill.Source, "@")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			name,
			skill.DisplayVersion(),
			strings.Join(skill.Targets, ", "),
			source,
		)
	}

	return tw.Flush()
}
```

Add `"io"` and `"sort"` to the imports at the top of `cmd/list.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ -run TestPrintLocalTable_WithSkills -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/list.go cmd/list_test.go
git commit -m "feat(list): add printLocalTable with sorted output and source stripping"
```

---

### Task 3: Test and implement empty state for table output

**Files:**
- Modify: `cmd/list_test.go`

- [ ] **Step 1: Write the failing test for empty state**

Add to `cmd/list_test.go`:

```go
func TestPrintLocalTable_EmptyState(t *testing.T) {
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := printLocalTable(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No skills installed.") {
		t.Errorf("expected empty state message, got:\n%s", out)
	}
	if !strings.Contains(out, "scribe connect") {
		t.Errorf("expected connect hint, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./cmd/ -run TestPrintLocalTable_EmptyState -v`
Expected: PASS (the empty path is already implemented in the function)

- [ ] **Step 3: Commit**

```bash
git add cmd/list_test.go
git commit -m "test(list): add empty state test for printLocalTable"
```

---

### Task 4: Write the `printLocalJSON` function (TDD)

**Files:**
- Modify: `cmd/list_test.go`
- Modify: `cmd/list.go`

- [ ] **Step 1: Write the failing test for JSON output**

Add to `cmd/list_test.go`:

```go
func TestPrintLocalJSON_WithSkills(t *testing.T) {
	now := time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC)
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version:     "v0.12.9.0",
				Source:      "github:garrytan/gstack@v0.12.9.0",
				InstalledAt: now,
				Targets:     []string{"claude", "cursor"},
				Registries:  []string{"ArtistfyHQ/team-skills"},
			},
		},
	}

	var buf bytes.Buffer
	err := printLocalJSON(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"name":"gstack"`) && !strings.Contains(out, `"name": "gstack"`) {
		t.Errorf("expected name field in JSON, got:\n%s", out)
	}
	if !strings.Contains(out, `"version":"v0.12.9.0"`) && !strings.Contains(out, `"version": "v0.12.9.0"`) {
		t.Errorf("expected version field in JSON, got:\n%s", out)
	}
}

func TestPrintLocalJSON_EmptyState(t *testing.T) {
	st := &state.State{
		Installed: make(map[string]state.InstalledSkill),
	}

	var buf bytes.Buffer
	err := printLocalJSON(&buf, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected empty JSON array, got: %s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestPrintLocalJSON -v`
Expected: FAIL — `printLocalJSON` not defined

- [ ] **Step 3: Write the `printLocalJSON` function**

Add to `cmd/list.go`:

```go
// localSkillJSON is the JSON representation of a locally installed skill.
type localSkillJSON struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Source      string    `json:"source"`
	Targets     []string  `json:"targets"`
	InstalledAt time.Time `json:"installed_at"`
	Registries  []string  `json:"registries,omitempty"`
}

// printLocalJSON renders the local skill inventory as a JSON array.
func printLocalJSON(w io.Writer, st *state.State) error {
	names := make([]string, 0, len(st.Installed))
	for name := range st.Installed {
		names = append(names, name)
	}
	sort.Strings(names)

	skills := make([]localSkillJSON, 0, len(names))
	for _, name := range names {
		sk := st.Installed[name]
		skills = append(skills, localSkillJSON{
			Name:        name,
			Version:     sk.DisplayVersion(),
			Source:      sk.Source,
			Targets:     sk.Targets,
			InstalledAt: sk.InstalledAt,
			Registries:  sk.Registries,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(skills)
}
```

Add `"time"` to the imports in `cmd/list.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestPrintLocalJSON -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/list.go cmd/list_test.go
git commit -m "feat(list): add printLocalJSON with sorted output"
```

---

### Task 5: Wire `--local` flag and no-registry fallback into `runList`

**Files:**
- Modify: `cmd/list.go:36-69`
- Modify: `cmd/list_test.go`

- [ ] **Step 1: Write the failing test for `--local` flag behavior**

Add to `cmd/list_test.go`:

```go
func TestRunList_LocalFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Write a state file with one skill.
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"gstack": {
				Version: "v0.12.9.0",
				Source:  "github:garrytan/gstack@v0.12.9.0",
				Targets: []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	// Reset flag state for test.
	listLocal = true
	listJSON = false
	defer func() { listLocal = false }()

	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)

	err := runList(listCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "gstack") {
		t.Errorf("expected skill in output, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Write the failing test for no-registry fallback**

Add to `cmd/list_test.go`:

```go
func TestRunList_NoRegistries_FallsBackToLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Write a config with no team repos.
	// config.Load() returns empty TeamRepos when file is missing — that's our test case.

	// Write a state file with one skill.
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"deploy": {
				Version: "v1.0.0",
				Source:  "github:ArtistfyHQ/team-skills@v1.0.0",
				Targets: []string{"claude"},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	// Ensure no --local flag — this tests the fallback path.
	listLocal = false
	listJSON = false
	registryFlag = ""

	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)

	err := runList(listCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy") {
		t.Errorf("expected local skill in fallback output, got:\n%s", out)
	}
	if !strings.Contains(out, "scribe connect") {
		t.Errorf("expected connect hint in fallback output, got:\n%s", out)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestRunList_LocalFlag|TestRunList_NoRegistries" -v`
Expected: FAIL — `runList` still errors on no registries, doesn't check `listLocal`

- [ ] **Step 4: Rewrite `runList` to handle `--local` and no-registry fallback**

Replace the `runList` function in `cmd/list.go`:

```go
func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	useJSON := listJSON || !isatty.IsTerminal(os.Stdout.Fd())

	// Local view: --local flag or no registries connected.
	if listLocal || len(cfg.TeamRepos) == 0 {
		w := cmd.OutOrStdout()
		if useJSON {
			return printLocalJSON(w, st)
		}
		err := printLocalTable(w, st)
		if err != nil {
			return err
		}
		// Show hint when falling back due to no registries (not when --local is explicit).
		if !listLocal && len(cfg.TeamRepos) == 0 && len(st.Installed) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Tip: connect a registry with \"scribe connect\" to see team skill status")
		}
		return nil
	}

	// Remote diff view (existing behavior).
	st.MigrateRegistries(cfg.TeamRepos[0])

	repos, err := filterRegistries(registryFlag, cfg.TeamRepos)
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	syncer := &sync.Syncer{Client: client, Targets: []targets.Target{}}

	multiRegistry := len(repos) > 1

	if useJSON {
		return printMultiListJSON(repos, syncer, st)
	}
	return printMultiListTable(repos, syncer, st, multiRegistry)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/ -run "TestRunList_LocalFlag|TestRunList_NoRegistries" -v`
Expected: PASS

- [ ] **Step 6: Run all tests to verify nothing is broken**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/list.go cmd/list_test.go
git commit -m "feat(list): wire --local flag and no-registry fallback to local view

Closes #22"
```

---

### Task 6: Update command help text

**Files:**
- Modify: `cmd/list.go:23-25`

- [ ] **Step 1: Update the command Short and Long descriptions**

Update the `listCmd` definition:

```go
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show installed skills and their sync status",
	Long: `Show installed skills and their status.

Without flags, compares local skills against team registries.
With --local, shows only locally installed skills (offline, no registry needed).
When no registries are connected, automatically shows local skills.`,
	RunE: runList,
}
```

- [ ] **Step 2: Verify help output**

Run: `go run ./cmd/scribe list --help`
Expected: shows updated description with `--local` flag documented

- [ ] **Step 3: Commit**

```bash
git add cmd/list.go
git commit -m "docs(list): update help text to describe --local flag and fallback behavior"
```

---

### Task 7: Final verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 2: Run the linter**

Run: `go vet ./...`
Expected: no issues

- [ ] **Step 3: Build the binary**

Run: `go build ./cmd/scribe`
Expected: clean build

- [ ] **Step 4: Manual smoke test**

Run: `go run ./cmd/scribe list --local`
Expected: either a table of installed skills or "No skills installed." message

Run: `go run ./cmd/scribe list --local --json`
Expected: JSON array output

Run: `go run ./cmd/scribe list --local --registry foo`
Expected: error about mutually exclusive flags
