# Multi-Registry Sync/List UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--registry` flag to `sync` and `list` so multi-registry users can filter by registry, and show grouped output when multiple registries are connected.

**Architecture:** Add `Registries []string` field to `InstalledSkill` with migration on load. Add a shared `resolveRegistry` helper for partial matching. Update `cmd/sync.go` to filter registries and print per-registry headers. Rewrite `cmd/list.go` to iterate all registries with grouped output.

**Tech Stack:** Go, Cobra, go-isatty, tabwriter

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/state/state.go` | Modify | Add `Registries` field, migration in `Load()`, `AddRegistry`/`RemoveRegistry` helpers |
| `internal/state/state_test.go` | Modify | Test migration, `AddRegistry`, `RemoveRegistry` |
| `cmd/registry.go` | Create | Shared `resolveRegistry` helper + `--registry` flag variable |
| `cmd/registry_test.go` | Create | Test `resolveRegistry` (exact, partial, ambiguous, unknown) |
| `cmd/sync.go` | Modify | Add `--registry` flag, filter loop, per-registry headers, registry tracking |
| `cmd/list.go` | Modify | Add `--registry` flag, multi-registry grouped output, per-registry footer |

---

### Task 1: Add `Registries` field to `InstalledSkill`

**Files:**
- Modify: `internal/state/state.go:22-29`
- Modify: `internal/state/state_test.go`

- [ ] **Step 1: Write test for Registries field round-trip**

In `internal/state/state_test.go`, add:

```go
func TestRegistriesRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := state.Load()
	s.RecordInstall("deploy", state.InstalledSkill{
		Version:    "v1.0.0",
		Source:     "github:org/deploy@v1.0.0",
		Targets:    []string{"claude"},
		Registries: []string{"ArtistfyHQ/team-skills"},
	})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _ := state.Load()
	skill := loaded.Installed["deploy"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("Registries: got %v", skill.Registries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -run TestRegistriesRoundTrip -v`
Expected: FAIL — `InstalledSkill` has no `Registries` field yet.

- [ ] **Step 3: Add `Registries` field to `InstalledSkill`**

In `internal/state/state.go`, change the `InstalledSkill` struct (line 22-29):

```go
type InstalledSkill struct {
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
	Targets     []string  `json:"targets"`
	Paths       []string  `json:"paths"`
	Registries  []string  `json:"registries,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/state/ -run TestRegistriesRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "[agent] feat: add Registries field to InstalledSkill

Step 1 of task: multi-registry sync/list UX"
```

---

### Task 2: Add `AddRegistry` / `RemoveRegistry` helpers + migration

**Files:**
- Modify: `internal/state/state.go`
- Modify: `internal/state/state_test.go`

- [ ] **Step 1: Write tests for AddRegistry, RemoveRegistry, and migration**

In `internal/state/state_test.go`, add:

```go
func TestAddRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, _ := state.Load()

	s.RecordInstall("deploy", state.InstalledSkill{
		Version: "v1.0.0",
		Source:  "github:org/deploy@v1.0.0",
	})

	s.AddRegistry("deploy", "ArtistfyHQ/team-skills")
	s.AddRegistry("deploy", "vercel/skills")
	s.AddRegistry("deploy", "ArtistfyHQ/team-skills") // duplicate — should be a no-op

	skill := s.Installed["deploy"]
	if len(skill.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d: %v", len(skill.Registries), skill.Registries)
	}
}

func TestRemoveRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, _ := state.Load()

	s.RecordInstall("deploy", state.InstalledSkill{
		Version:    "v1.0.0",
		Source:     "github:org/deploy@v1.0.0",
		Registries: []string{"ArtistfyHQ/team-skills", "vercel/skills"},
	})

	s.RemoveRegistry("deploy", "ArtistfyHQ/team-skills")
	skill := s.Installed["deploy"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "vercel/skills" {
		t.Fatalf("expected [vercel/skills], got %v", skill.Registries)
	}
}

func TestMigrationBackfillsRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a legacy state.json with no Registries field.
	dir := filepath.Join(home, ".scribe")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{
		"team": {},
		"installed": {
			"gstack": {
				"version": "v0.12.9.0",
				"source": "github:garrytan/gstack@v0.12.9.0",
				"installed_at": "2026-01-01T00:00:00Z",
				"targets": ["claude"],
				"paths": ["/Users/test/.claude/skills/gstack/"]
			}
		}
	}`), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skill := s.Installed["gstack"]
	if len(skill.Registries) != 0 {
		t.Errorf("pre-migration skill should have empty registries, got %v", skill.Registries)
	}

	// Migrate applies a default registry.
	s.MigrateRegistries("ArtistfyHQ/team-skills")
	skill = s.Installed["gstack"]
	if len(skill.Registries) != 1 || skill.Registries[0] != "ArtistfyHQ/team-skills" {
		t.Errorf("after migration: got %v", skill.Registries)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/state/ -run "TestAddRegistry|TestRemoveRegistry|TestMigrationBackfillsRegistries" -v`
Expected: FAIL — methods don't exist.

- [ ] **Step 3: Implement helpers and migration**

In `internal/state/state.go`, add after the `Remove` method:

```go
// AddRegistry appends a registry to a skill's Registries list (dedup, case-insensitive).
func (s *State) AddRegistry(name, registry string) {
	skill, ok := s.Installed[name]
	if !ok {
		return
	}
	for _, r := range skill.Registries {
		if strings.EqualFold(r, registry) {
			return
		}
	}
	skill.Registries = append(skill.Registries, registry)
	s.Installed[name] = skill
}

// RemoveRegistry removes a registry from a skill's Registries list.
func (s *State) RemoveRegistry(name, registry string) {
	skill, ok := s.Installed[name]
	if !ok {
		return
	}
	filtered := skill.Registries[:0]
	for _, r := range skill.Registries {
		if !strings.EqualFold(r, registry) {
			filtered = append(filtered, r)
		}
	}
	skill.Registries = filtered
	s.Installed[name] = skill
}

// MigrateRegistries backfills the Registries field for skills that predate
// multi-registry support. Called from the cmd layer with config.TeamRepos[0].
func (s *State) MigrateRegistries(defaultRegistry string) {
	for name, skill := range s.Installed {
		if len(skill.Registries) == 0 {
			skill.Registries = []string{defaultRegistry}
			s.Installed[name] = skill
		}
	}
}
```

Also add `"strings"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/state/ -run "TestAddRegistry|TestRemoveRegistry|TestMigrationBackfillsRegistries" -v`
Expected: PASS

- [ ] **Step 5: Run all state tests**

Run: `go test ./internal/state/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "[agent] feat: add AddRegistry, RemoveRegistry, MigrateRegistries to state

Step 2 of task: multi-registry sync/list UX"
```

---

### Task 3: Create `resolveRegistry` helper

**Files:**
- Create: `cmd/registry.go`
- Create: `cmd/registry_test.go`

- [ ] **Step 1: Write tests for resolveRegistry**

Create `cmd/registry_test.go`:

```go
package cmd

import "testing"

func TestResolveRegistry(t *testing.T) {
	repos := []string{"ArtistfyHQ/team-skills", "vercel/skills", "acme/tools"}

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{"exact match", "ArtistfyHQ/team-skills", "ArtistfyHQ/team-skills", ""},
		{"exact case-insensitive", "artistfyhq/team-skills", "ArtistfyHQ/team-skills", ""},
		{"partial repo name", "team-skills", "ArtistfyHQ/team-skills", ""},
		{"partial case-insensitive", "Team-Skills", "ArtistfyHQ/team-skills", ""},
		{"partial tools", "tools", "acme/tools", ""},
		{"ambiguous partial", "skills", "", "ambiguous"},
		{"unknown", "nonexistent", "", "not connected"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveRegistry(c.input, repos)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErr)
				}
				if !containsCI(err.Error(), c.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// containsCI checks if s contains substr (case-insensitive).
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(substr) == 0 ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if equalFoldAt(s[i:i+len(substr)], substr) {
						return true
					}
				}
				return false
			}())
}

func equalFoldAt(a, b string) bool {
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestResolveRegistry -v`
Expected: FAIL — `resolveRegistry` not defined.

- [ ] **Step 3: Implement resolveRegistry**

Create `cmd/registry.go`:

```go
package cmd

import (
	"fmt"
	"strings"
)

// registryFlag is the shared --registry flag value for sync and list.
var registryFlag string

// resolveRegistry matches a user-provided registry string against connected repos.
// Accepts full "owner/repo" (case-insensitive) or partial "repo" name if unambiguous.
func resolveRegistry(input string, repos []string) (string, error) {
	// Try exact match first (case-insensitive).
	for _, r := range repos {
		if strings.EqualFold(r, input) {
			return r, nil
		}
	}

	// Try partial match on repo name (the part after the slash).
	var matches []string
	for _, r := range repos {
		parts := strings.SplitN(r, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], input) {
			matches = append(matches, r)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("not connected to %q — run: scribe connect %s", input, input)
	default:
		return "", fmt.Errorf("ambiguous registry %q — did you mean:\n  %s", input, strings.Join(matches, "\n  "))
	}
}

// filterRegistries returns the subset of repos to operate on, based on the --registry flag.
// If flag is empty, returns all repos. Otherwise resolves and returns a single-element slice.
func filterRegistries(flag string, repos []string) ([]string, error) {
	if flag == "" {
		return repos, nil
	}
	resolved, err := resolveRegistry(flag, repos)
	if err != nil {
		return nil, err
	}
	return []string{resolved}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestResolveRegistry -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/registry.go cmd/registry_test.go
git commit -m "[agent] feat: add resolveRegistry helper with partial matching

Step 3 of task: multi-registry sync/list UX"
```

---

### Task 4: Add `--registry` flag and per-registry headers to `sync`

**Files:**
- Modify: `cmd/sync.go`
- Modify: `cmd/registry.go` (flag already defined there)

- [ ] **Step 1: Register `--registry` flag on sync command**

In `cmd/sync.go`, change the `init()` function:

```go
func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output machine-readable JSON (for CI/agents)")
	syncCmd.Flags().StringVar(&registryFlag, "registry", "", "Sync only this registry (owner/repo or repo name)")
}
```

- [ ] **Step 2: Update `runSync` to filter registries and print headers**

Replace the registry loop and JSON output in `runSync` (lines 42-164 of `cmd/sync.go`):

```go
func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}

	// Migrate legacy state (no Registries field) on first multi-registry run.
	if len(cfg.TeamRepos) > 0 {
		st.MigrateRegistries(cfg.TeamRepos[0])
	}

	repos, err := filterRegistries(registryFlag, cfg.TeamRepos)
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	useJSON := syncJSON || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(cfg.TeamRepos) > 1

	resolved := map[string]sync.SkillStatus{}

	type skillResult struct {
		Name    string `json:"name"`
		Action  string `json:"action"`
		Status  string `json:"status,omitempty"`
		Version string `json:"version,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	// For JSON: collect per-registry results.
	type registryResult struct {
		Registry string        `json:"registry"`
		Skills   []skillResult `json:"skills"`
	}
	var jsonRegistries []registryResult
	totalSummary := sync.SyncCompleteMsg{}

	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
	}

	for _, teamRepo := range repos {
		clear(resolved)
		var jsonResults []skillResult

		syncer.Emit = func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus

			case sync.SkillSkippedMsg:
				sk := resolved[m.Name]
				ver := ""
				if sk.Installed != nil {
					ver = sk.Installed.DisplayVersion()
				}
				if useJSON {
					jsonResults = append(jsonResults, skillResult{
						Name:    m.Name,
						Action:  "skipped",
						Status:  sk.Status.String(),
						Version: ver,
					})
				} else {
					fmt.Printf("  %-20s ok (%s)\n", m.Name, ver)
				}

			case sync.SkillDownloadingMsg:
				if !useJSON {
					fmt.Printf("  %-20s downloading...\n", m.Name)
				}

			case sync.SkillInstalledMsg:
				if useJSON {
					action := "installed"
					if m.Updated {
						action = "updated"
					}
					jsonResults = append(jsonResults, skillResult{
						Name:    m.Name,
						Action:  action,
						Version: m.Version,
					})
				} else {
					verb := "installed"
					if m.Updated {
						verb = "updated to"
					}
					fmt.Printf("  %-20s %s %s\n", m.Name, verb, m.Version)
				}

			case sync.SkillErrorMsg:
				if useJSON {
					jsonResults = append(jsonResults, skillResult{
						Name:   m.Name,
						Action: "error",
						Error:  m.Err.Error(),
					})
				} else {
					fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
				}

			case sync.SyncCompleteMsg:
				totalSummary.Installed += m.Installed
				totalSummary.Updated += m.Updated
				totalSummary.Skipped += m.Skipped
				totalSummary.Failed += m.Failed
			}
		}

		if !useJSON && multiRegistry {
			fmt.Fprintf(os.Stderr, "── %s ──\n", teamRepo)
		} else if !useJSON {
			fmt.Fprintf(os.Stderr, "syncing %s...\n\n", teamRepo)
		}

		if err := syncer.Run(context.Background(), teamRepo, st); err != nil {
			return err
		}

		// Track which registry each synced skill belongs to.
		for name := range resolved {
			st.AddRegistry(name, teamRepo)
		}
		_ = st.Save()

		if useJSON {
			jsonRegistries = append(jsonRegistries, registryResult{
				Registry: teamRepo,
				Skills:   jsonResults,
			})
		}

		if !useJSON && multiRegistry {
			fmt.Println()
		}
	}

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"registries": jsonRegistries,
			"summary": map[string]int{
				"installed": totalSummary.Installed,
				"updated":   totalSummary.Updated,
				"skipped":   totalSummary.Skipped,
				"failed":    totalSummary.Failed,
			},
		})
	}

	fmt.Printf("\ndone: %d installed, %d updated, %d current, %d failed\n",
		totalSummary.Installed, totalSummary.Updated, totalSummary.Skipped, totalSummary.Failed)

	return nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 4: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/sync.go
git commit -m "[agent] feat: add --registry flag and per-registry headers to sync

Step 4 of task: multi-registry sync/list UX"
```

---

### Task 5: Rewrite `list` for multi-registry grouped output

**Files:**
- Modify: `cmd/list.go`

- [ ] **Step 1: Register `--registry` flag on list command**

In `cmd/list.go`, change the `init()` function:

```go
func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output machine-readable JSON")
	listCmd.Flags().StringVar(&registryFlag, "registry", "", "Show only this registry (owner/repo or repo name)")
}
```

- [ ] **Step 2: Rewrite `runList` to iterate all registries**

Replace the full `runList` function:

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

	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}

	repos, err := filterRegistries(registryFlag, cfg.TeamRepos)
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	syncer := &sync.Syncer{Client: client, Targets: []targets.Target{}}

	useJSON := listJSON || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(repos) > 1

	if useJSON {
		return printMultiListJSON(repos, syncer, st)
	}
	return printMultiListTable(repos, syncer, st, multiRegistry)
}
```

- [ ] **Step 3: Rewrite table output for grouped display**

Replace the `printListTable` function:

```go
func printMultiListTable(repos []string, syncer *sync.Syncer, st *state.State, grouped bool) error {
	var footerParts []string

	for i, teamRepo := range repos {
		statuses, err := syncer.Diff(context.Background(), teamRepo, st)
		if err != nil {
			return err
		}

		if grouped {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("── %s ──\n", teamRepo)
		} else {
			fmt.Printf("team: %s\n\n", teamRepo)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SKILL\tVERSION\tSTATUS\tAGENTS")

		for _, sk := range statuses {
			ver := sk.LoadoutRef
			if ver == "" && sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
			}

			agents := ""
			if sk.Installed != nil {
				agents = strings.Join(sk.Installed.Targets, ", ")
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sk.Name, ver, sk.Status.String(), agents)
		}

		w.Flush()

		counts := countStatuses(statuses)
		if grouped {
			parts := formatCounts(counts)
			if parts != "" {
				footerParts = append(footerParts, fmt.Sprintf("%s: %s", teamRepo, parts))
			}
		} else {
			fmt.Printf("\n%d current · %d outdated · %d missing · %d extra\n",
				counts[sync.StatusCurrent], counts[sync.StatusOutdated],
				counts[sync.StatusMissing], counts[sync.StatusExtra])
		}
	}

	if grouped && len(footerParts) > 0 {
		fmt.Printf("\n%s\n", strings.Join(footerParts, "  ·  "))
	}

	if !st.Team.LastSync.IsZero() {
		fmt.Printf("Last sync: %s\n", st.Team.LastSync.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

// formatCounts builds a compact count string like "2 current · 1 outdated".
func formatCounts(counts map[sync.Status]int) string {
	var parts []string
	if n := counts[sync.StatusCurrent]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d current", n))
	}
	if n := counts[sync.StatusOutdated]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d outdated", n))
	}
	if n := counts[sync.StatusMissing]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", n))
	}
	if n := counts[sync.StatusExtra]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d extra", n))
	}
	return strings.Join(parts, " · ")
}
```

- [ ] **Step 4: Rewrite JSON output for registries structure**

Replace `printListJSON`:

```go
func printMultiListJSON(repos []string, syncer *sync.Syncer, st *state.State) error {
	type skillJSON struct {
		Name       string   `json:"name"`
		Status     string   `json:"status"`
		Version    string   `json:"version,omitempty"`
		LoadoutRef string   `json:"loadout_ref,omitempty"`
		Maintainer string   `json:"maintainer,omitempty"`
		Agents     []string `json:"agents,omitempty"`
	}

	type registryJSON struct {
		Registry string      `json:"registry"`
		Skills   []skillJSON `json:"skills"`
	}

	var registries []registryJSON

	for _, teamRepo := range repos {
		statuses, err := syncer.Diff(context.Background(), teamRepo, st)
		if err != nil {
			return err
		}

		skills := make([]skillJSON, 0, len(statuses))
		for _, sk := range statuses {
			ver := ""
			var agents []string
			if sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
				agents = sk.Installed.Targets
			}
			skills = append(skills, skillJSON{
				Name:       sk.Name,
				Status:     sk.Status.String(),
				Version:    ver,
				LoadoutRef: sk.LoadoutRef,
				Maintainer: sk.Maintainer,
				Agents:     agents,
			})
		}

		registries = append(registries, registryJSON{
			Registry: teamRepo,
			Skills:   skills,
		})
	}

	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"registries": registries,
	})
}
```

- [ ] **Step 5: Remove old `printListTable` and `printListJSON` functions**

Delete the old `printListTable` (lines 67-98) and `printListJSON` (lines 100-139) functions from `cmd/list.go`. Keep `countStatuses`.

- [ ] **Step 6: Verify it compiles**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 7: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/list.go
git commit -m "[agent] feat: rewrite list for multi-registry grouped output

Step 5 of task: multi-registry sync/list UX"
```

---

### Task 6: Accept `--all` silently

**Files:**
- Modify: `cmd/sync.go`
- Modify: `cmd/list.go`

- [ ] **Step 1: Add `--all` flag to both commands**

In `cmd/sync.go` `init()`, add:

```go
syncCmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
syncCmd.Flags().MarkHidden("all")
```

In `cmd/list.go` `init()`, add:

```go
listCmd.Flags().Bool("all", false, "List all registries (default behavior)")
listCmd.Flags().MarkHidden("all")
```

These flags are accepted but have no effect — they're hidden so they don't clutter help, but scripts using `--all` won't break.

- [ ] **Step 2: Verify it compiles and `--all` is accepted**

Run: `go build ./... && go run ./cmd/scribe sync --help`
Expected: `--all` does NOT appear in help output (hidden). But `go run ./cmd/scribe sync --all` does not error.

- [ ] **Step 3: Commit**

```bash
git add cmd/sync.go cmd/list.go
git commit -m "[agent] chore: accept --all flag silently on sync and list

Step 6 of task: multi-registry sync/list UX"
```
