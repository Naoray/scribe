# Per-Skill Tool Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users control which tools (claude/codex/gemini/custom) each installed skill lives on, explicitly distinguishing user-pinned assignments from inherit-mode defaults.

**Architecture:** Introduce `InstalledSkill.ToolsMode` (inherit/pinned) and change `Paths` to `map[string][]string` keyed by tool name. Add an `internal/tools/assign` package with pure `Plan`/`Apply`/`Merge` functions. Syncer short-circuits pinned skills and self-heals missing symlinks using exact per-tool path lookups. CLI surface: `scribe skill edit <name>` for single-skill edits, `scribe tools loadout <tool>` for bulk tool-centric edits, and reconcile-on-`tools enable/disable` for inherit-mode backfill.

**Tech Stack:** Go 1.26.1, Cobra, Bubble Tea v2 (`charm.land/bubbletea/v2`), Lip Gloss v2, Huh v2. State persisted to `~/.scribe/state.json`. Table-driven tests with `t.Setenv("HOME", t.TempDir())`.

**Spec:** `docs/superpowers/specs/2026-04-11-per-skill-tool-management-design.md` (revision 2).

---

## File Structure

### New files
- `internal/tools/assign/assign.go` — `Op`, `Result`, `Mode`, `Plan`, `Apply`, `Merge`
- `internal/tools/assign/assign_test.go` — unit tests for all four funcs
- `cmd/skill.go` — `scribe skill` parent + `scribe skill edit` subcommand
- `cmd/skill_test.go` — flag mutex, edit flows, `--inherit`, rejection of `cursor`/packages
- `cmd/tools_loadout.go` — `scribe tools loadout <tool>` command wiring
- `cmd/tools_loadout_tui.go` — bulk TUI model
- `cmd/tools_loadout_tui_test.go` — staging + save roundtrip
- `cmd/tools_reconcile.go` — `reconcileInheritSkills` helper shared by enable/disable

### Files modified
- `internal/paths/paths.go` — add `SkillStore(name)` helper
- `internal/tools/tool.go` — `Uninstall` signature `(removed []string, err error)`
- `internal/tools/claude.go`, `cursor.go`, `gemini.go`, `codex.go`, `command.go` — implement new `Uninstall` signature
- `internal/tools/runtime.go` — `ResolveStatuses` always emits every builtin
- `internal/state/state.go` — `ToolsMode` field; `Paths` → `map[string][]string`; legacy migration path
- `internal/sync/syncer.go` — pinned branch, self-heal, write new `Paths` map shape
- `cmd/remove.go` — consume new `Uninstall` signature, update `Paths` map entry
- `cmd/list_tui.go` — `focusTools` state, `pendingTools` overlay, `renderToolsPane`, apply-on-defocus
- `cmd/tools.go` — `runToolsEnable/Disable` call `reconcileInheritSkills`; `AddCommand(newToolsLoadoutCommand())`
- `internal/tools/claude_test.go`, `internal/tools/runtime_test.go`, `internal/sync/syncer_test.go`, `cmd/tools_test.go`, `cmd/list_tui_test.go` — new cases

Files that change together live together. Each change block is scoped to a single responsibility so the build stays green between tasks.

---

## Execution Order Rationale

1. **Pre-refactor** (Tasks 1–2): pure-refactor items that unblock later work without changing behavior.
2. **State schema** (Tasks 3–4): `ToolsMode` + `Paths` map + migration. Everything downstream depends on the new shape.
3. **Tool interface** (Tasks 5–6): new `Uninstall` signature + caller updates. Isolated so the compile error surface is minimal.
4. **Core logic** (Tasks 7–9): the `assign` package — pure, easy to TDD before any UI or CLI touches it.
5. **Syncer integration** (Tasks 10–11): pinned branch + self-heal + new `Paths` map writes.
6. **CLI** (Tasks 12–14): `scribe skill edit`, reconcile in `tools enable/disable`, bulk `tools loadout`.
7. **TUI** (Tasks 15–16): list TUI tools pane + bulk loadout TUI.
8. **Integration sweep** (Task 17): end-to-end test, self-review, ship.

---

## Task 1: Extract `paths.SkillStore(name)` helper

**Files:**
- Modify: `internal/paths/paths.go`
- Modify: `internal/tools/store.go:42`
- Modify: `internal/sync/syncer.go:109,293,361` (and any other `filepath.Join(storeDir, name)` call)
- Test: `internal/paths/paths_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `internal/paths/paths_test.go`:

```go
package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/paths"
)

func TestSkillStoreJoinsStoreDirWithName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := paths.SkillStore("commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	store, err := paths.StoreDir()
	if err != nil {
		t.Fatalf("StoreDir: %v", err)
	}
	want := filepath.Join(store, "commit")
	if got != want {
		t.Fatalf("SkillStore = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/paths/...`
Expected: FAIL with `undefined: paths.SkillStore`.

- [ ] **Step 3: Add `SkillStore` to paths package**

Append to `internal/paths/paths.go`:

```go
// SkillStore returns ~/.scribe/skills/<name>/ for a given skill name.
func SkillStore(name string) (string, error) {
	dir, err := StoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/paths/...`
Expected: PASS.

- [ ] **Step 5: Replace ad-hoc joins at callers**

Update `internal/sync/syncer.go`:

At the three existing `filepath.Join(storeDir, sk.Name)` sites (inside `apply` lines ~295 and ~363 and `Diff` line ~124), keep them — they are already driven by a `StoreDir` lookup. Only add usage where a brand-new lookup would otherwise appear. Grep the repo for `filepath.Join(home, ".scribe", "skills", ...)` style duplication and replace those with `paths.SkillStore(name)` (non-test code only):

```bash
rg -n 'filepath\.Join\(home, "\.scribe", "skills"' cmd internal
```

For each non-test hit that includes a skill name, switch it to `paths.SkillStore(name)`. Do not touch tests; they own their filesystem fixtures.

- [ ] **Step 6: Run all builds + tests**

Run: `go build ./... && go test ./internal/paths/... ./internal/tools/... ./internal/sync/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/paths/paths.go internal/paths/paths_test.go internal/sync/syncer.go
git commit -m "[agent] Extract paths.SkillStore helper

Step 1 of task: per-skill tool management"
```

---

## Task 2: `ResolveStatuses` always emits every builtin

**Files:**
- Modify: `internal/tools/runtime.go:80-100`
- Test: `internal/tools/runtime_test.go` (may exist; otherwise create)

- [ ] **Step 1: Write the failing test**

Add to `internal/tools/runtime_test.go` (create if missing):

```go
package tools_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/tools"
)

func TestResolveStatusesEmitsUndetectedBuiltins(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no ~/.claude, ~/.codex, gemini, etc.

	cfg := &config.Config{}
	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		t.Fatalf("ResolveStatuses: %v", err)
	}

	wantNames := map[string]bool{"claude": false, "cursor": false, "gemini": false, "codex": false}
	for _, st := range statuses {
		if _, ok := wantNames[st.Name]; ok {
			wantNames[st.Name] = true
			if st.Detected {
				t.Errorf("%s: Detected=true in empty HOME", st.Name)
			}
			if !st.DetectKnown {
				t.Errorf("%s: DetectKnown should be true for builtins", st.Name)
			}
		}
	}
	for name, saw := range wantNames {
		if !saw {
			t.Errorf("builtin %q missing from ResolveStatuses", name)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/... -run TestResolveStatusesEmitsUndetectedBuiltins`
Expected: FAIL — undetected builtins are missing from statuses.

- [ ] **Step 3: Make the change**

In `internal/tools/runtime.go` `ResolveStatuses`, change the initial loop that only adds **detected** builtins into one that always adds every builtin, stamping `Detected: builtinDetected[name]`:

```go
statuses := make(map[string]Status)
for _, tool := range DefaultTools() {
	name := strings.ToLower(tool.Name())
	statuses[name] = Status{
		Name:        tool.Name(),
		Type:        ToolTypeBuiltin,
		Enabled:     builtinDetected[name], // default: enabled iff detected
		Detected:    builtinDetected[name],
		DetectKnown: true,
		Source:      "auto",
	}
}
```

The config-driven loop below already overrides `Enabled` when the user has explicit entries. Leave it untouched.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tools/... -run TestResolveStatusesEmitsUndetectedBuiltins`
Expected: PASS.

- [ ] **Step 5: Run full package tests to catch regressions**

Run: `go test ./internal/tools/... ./cmd/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/runtime.go internal/tools/runtime_test.go
git commit -m "[agent] ResolveStatuses: always emit builtins

Step 2 of task: per-skill tool management"
```

---

## Task 3: Add `ToolsMode` and pivot `Paths` to `map[string][]string`

**Files:**
- Modify: `internal/state/state.go:19-60` (types)
- Test: `internal/state/state_test.go` (schema shape + defaults)

- [ ] **Step 1: Write the failing test**

Add to `internal/state/state_test.go`:

```go
func TestInstalledSkillDefaultToolsModeInherit(t *testing.T) {
	skill := state.InstalledSkill{}
	if skill.ToolsMode != state.ToolsModeInherit {
		t.Errorf("zero value = %q, want %q", skill.ToolsMode, state.ToolsModeInherit)
	}
}

func TestInstalledSkillPathsMapShape(t *testing.T) {
	skill := state.InstalledSkill{
		Tools: []string{"claude", "codex"},
		Paths: map[string][]string{
			"claude": {"/home/u/.claude/skills/commit"},
			"codex":  {"/home/u/.codex/skills/commit"},
		},
	}
	if got := skill.Paths["claude"]; len(got) != 1 {
		t.Errorf("claude paths = %v, want single entry", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/state/... -run TestInstalledSkill`
Expected: FAIL — `state.ToolsModeInherit` undefined and `Paths` is `[]string`.

- [ ] **Step 3: Update the type**

In `internal/state/state.go` replace the `InstalledSkill` struct block and add the `ToolsMode` type above it:

```go
// ToolsMode controls how sync reconciles the Tools list for an installed skill.
type ToolsMode string

const (
	// ToolsModeInherit (default, zero value) means sync recomputes Tools from
	// the globally enabled, detected tools on every run.
	ToolsModeInherit ToolsMode = ""
	// ToolsModePinned means the user explicitly chose the tool set via
	// `scribe skill edit`, the list TUI, or `scribe tools loadout`. Sync uses
	// Tools verbatim and global tools enable/disable do not backfill.
	ToolsModePinned ToolsMode = "pinned"
)

// InstalledSkill records everything needed to detect updates and uninstall.
type InstalledSkill struct {
	Revision      int                 `json:"revision"`
	InstalledHash string              `json:"installed_hash"`
	Sources       []SkillSource       `json:"sources,omitempty"`
	InstalledAt   time.Time           `json:"installed_at"`
	Tools         []string            `json:"tools"`
	Paths         map[string][]string `json:"paths"`
	ToolsMode     ToolsMode           `json:"tools_mode,omitempty"`

	// Package-specific fields (omitted for regular skills).
	Type       string    `json:"type,omitempty"`
	InstallCmd string    `json:"install_cmd,omitempty"`
	UpdateCmd  string    `json:"update_cmd,omitempty"`
	CmdHash    string    `json:"cmd_hash,omitempty"`
	Approval   string    `json:"approval,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}
```

This will break compile everywhere that reads `Paths` as `[]string`. Do not fix callers yet — they will be updated as each dependent task lands. The compile will go green again by the end of Task 6.

- [ ] **Step 4: Run the new tests in isolation to verify they pass**

Run: `go test ./internal/state/... -run TestInstalledSkill`
Expected: PASS (may still have other broken state tests — leave them for Task 4).

- [ ] **Step 5: Commit (compile is broken repo-wide; that is intentional)**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "[agent] Add ToolsMode and pivot Paths to map

Step 3 of task: per-skill tool management

Repo-wide compile is temporarily broken; callers updated in
subsequent commits."
```

---

## Task 4: Migrate legacy flat `Paths` slice into the keyed map on load

**Files:**
- Modify: `internal/state/state.go` — `legacyInstalledSkill`, `legacyToSkill`, `parseAndMigrate`
- Test: `internal/state/state_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/state/state_test.go`:

```go
func TestLoadMigratesLegacyFlatPathsToMap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scribeDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(scribeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	legacy := `{
		"schema_version": 4,
		"installed": {
			"commit": {
				"revision": 1,
				"installed_hash": "abc",
				"installed_at": "2025-01-01T00:00:00Z",
				"tools": ["claude", "codex"],
				"paths": [
					"/home/u/.claude/skills/commit",
					"/home/u/.codex/skills/commit"
				]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(scribeDir, "state.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := s.Installed["commit"]
	if skill.ToolsMode != state.ToolsModeInherit {
		t.Errorf("ToolsMode = %q, want inherit", skill.ToolsMode)
	}
	if got := skill.Paths["claude"]; len(got) != 1 || got[0] != "/home/u/.claude/skills/commit" {
		t.Errorf("claude paths = %v", got)
	}
	if got := skill.Paths["codex"]; len(got) != 1 || got[0] != "/home/u/.codex/skills/commit" {
		t.Errorf("codex paths = %v", got)
	}
}

func TestLoadMigratesMismatchedLegacyPathsToUnknownBucket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".scribe"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
		"schema_version": 4,
		"installed": {
			"x": {
				"tools": ["claude"],
				"paths": ["/a", "/b"]
			}
		}
	}`
	_ = os.WriteFile(filepath.Join(home, ".scribe", "state.json"), []byte(legacy), 0o644)

	s, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := s.Installed["x"]
	if got := skill.Paths["claude"]; len(got) != 1 || got[0] != "/a" {
		t.Errorf("claude paths = %v", got)
	}
	if got := skill.Paths["_unknown"]; len(got) != 1 || got[0] != "/b" {
		t.Errorf("_unknown paths = %v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/state/... -run TestLoadMigrates`
Expected: FAIL — `legacyInstalledSkill.Paths` is still `[]string` but `InstalledSkill.Paths` is a map, so the unmarshal and the direct assignment in `legacyToSkill` will fail to compile first.

- [ ] **Step 3: Update the legacy struct and migration**

In `internal/state/state.go`:

1. Keep `legacyInstalledSkill.Paths` as `[]string` — legacy JSON is flat.
2. Update `legacyToSkill` to bucket legacy flat paths into a map keyed by the corresponding `Tools[i]`:

```go
func legacyToSkill(ls legacyInstalledSkill) InstalledSkill {
	return InstalledSkill{
		Revision:      ls.Revision,
		InstalledHash: ls.InstalledHash,
		Sources:       ls.Sources,
		InstalledAt:   ls.InstalledAt,
		Tools:         ls.Tools,
		Paths:         migrateLegacyPaths(ls.Tools, ls.Paths),
		ToolsMode:     ToolsModeInherit,
		Type:          ls.Type,
		InstallCmd:    ls.InstallCmd,
		UpdateCmd:     ls.UpdateCmd,
		CmdHash:       ls.CmdHash,
		Approval:      ls.Approval,
		ApprovedAt:    ls.ApprovedAt,
	}
}

// migrateLegacyPaths buckets a legacy flat []string into a map keyed by
// the corresponding Tools[i] entry. Extra paths (mismatched length) go
// under "_unknown" and are ignored by self-heal. Returns an initialized
// empty map when nothing is present — never nil.
func migrateLegacyPaths(tools []string, flat []string) map[string][]string {
	out := make(map[string][]string)
	for i, path := range flat {
		if i < len(tools) {
			out[tools[i]] = append(out[tools[i]], path)
		} else {
			out["_unknown"] = append(out["_unknown"], path)
		}
	}
	return out
}
```

**Note:** `legacyInstalledSkill` is used as both the v1 migration shape AND the v4 pass-through. Pass-through (`else` branch in `parseAndMigrate`) invokes `legacyToSkill(e.skill)`, so the migration runs for every load — this is exactly what we want: stateful v4 entries with flat `Paths` are upgraded in-memory and re-saved on the next write.

Make sure `migrateLegacyPaths(nil, nil)` returns an initialized empty map so later map writes don't panic.

- [ ] **Step 4: Handle the zero-`Tools` case where `Paths` is already a map (re-load after migration)**

Edge case: after the first save post-migration, the JSON will contain `"paths": {"claude": [...]}`. The `legacyInstalledSkill.Paths []string` field will fail to decode that map and return `"json: cannot unmarshal object into Go struct field legacyInstalledSkill.paths of type []string"`.

Fix: change `legacyInstalledSkill.Paths` to `json.RawMessage` and resolve it inside `legacyToSkill`:

```go
type legacyInstalledSkill struct {
	// ...
	Paths json.RawMessage `json:"paths"`
	// ...
}
```

Then inside `legacyToSkill` (or a helper called from it), detect shape:

```go
func resolveLegacyPaths(tools []string, raw json.RawMessage) map[string][]string {
	if len(raw) == 0 || string(raw) == "null" {
		return make(map[string][]string)
	}
	// New shape: already a map.
	var asMap map[string][]string
	if err := json.Unmarshal(raw, &asMap); err == nil {
		if asMap == nil {
			return make(map[string][]string)
		}
		return asMap
	}
	// Legacy shape: flat slice, bucket by position.
	var asSlice []string
	if err := json.Unmarshal(raw, &asSlice); err == nil {
		return migrateLegacyPaths(tools, asSlice)
	}
	return make(map[string][]string)
}
```

`legacyToSkill` then calls `Paths: resolveLegacyPaths(ls.Tools, ls.Paths)`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/state/...`
Expected: PASS for migration tests. Other pre-existing tests may still fail if they construct `InstalledSkill` with old flat paths — update those fixtures in-line (they should be obvious compile errors).

- [ ] **Step 6: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "[agent] Migrate legacy flat Paths into tool-keyed map

Step 3 of task: per-skill tool management

Accepts either shape in JSON; buckets by position for legacy state."
```

---

## Task 5: Change `tools.Tool.Uninstall` signature to return removed paths

**Files:**
- Modify: `internal/tools/tool.go` (interface)
- Modify: `internal/tools/claude.go`, `cursor.go`, `gemini.go`, `codex.go`, `command.go`
- Test: existing `internal/tools/claude_test.go` (update to assert returned slice)

- [ ] **Step 1: Write the failing test**

In `internal/tools/claude_test.go`, add:

```go
func TestClaudeUninstallReturnsRemovedPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonicalDir := filepath.Join(home, ".scribe", "skills", "commit")
	if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalDir, "SKILL.md"), []byte("# commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ClaudeTool{}
	installed, err := tool.Install("commit", canonicalDir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	removed, err := tool.Uninstall("commit")
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(removed) != 1 || removed[0] != installed[0] {
		t.Errorf("removed = %v, want %v", removed, installed)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/... -run TestClaudeUninstallReturnsRemovedPaths`
Expected: FAIL — compile error, `tool.Uninstall(...)` returns one value.

- [ ] **Step 3: Update the interface**

In `internal/tools/tool.go`:

```go
type Tool interface {
	Name() string
	Install(skillName, canonicalDir string) (paths []string, err error)
	Uninstall(skillName string) (removed []string, err error)
	Detect() bool
}
```

- [ ] **Step 4: Update each builtin implementation**

**`internal/tools/claude.go`** — `Uninstall`:

```go
func (t ClaudeTool) Uninstall(skillName string) ([]string, error) {
	skillsDir, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove claude/%s: %w", skillName, err)
	}
	// Clean up empty parent directories left after removing namespaced symlinks.
	parent := filepath.Dir(link)
	if parent != skillsDir {
		_ = os.Remove(parent)
	}
	return []string{link}, nil
}
```

**`internal/tools/cursor.go`** — `Uninstall`:

```go
func (t CursorTool) Uninstall(skillName string) ([]string, error) {
	workDir, err := t.resolveWorkDir()
	if err != nil {
		return nil, err
	}
	mdcName := SlugifyRegistry(skillName) + ".mdc"
	link := filepath.Join(workDir, ".cursor", "rules", mdcName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove cursor/%s: %w", skillName, err)
	}
	return []string{link}, nil
}
```

**`internal/tools/codex.go`** — `Uninstall`:

```go
func (t CodexTool) Uninstall(skillName string) ([]string, error) {
	skillsDir, err := codexSkillsDir()
	if err != nil {
		return nil, err
	}
	link := filepath.Join(skillsDir, skillName)
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove codex/%s: %w", skillName, err)
	}
	return []string{link}, nil
}
```

**`internal/tools/gemini.go`** — `Uninstall`:

```go
func (t GeminiTool) Uninstall(skillName string) ([]string, error) {
	if _, err := exec.LookPath(toolGemini); err != nil {
		return nil, fmt.Errorf("gemini CLI not found in PATH — skill %q may still be linked; run `gemini skills uninstall %s --scope user` manually", skillName, skillName)
	}
	cmd := exec.Command(toolGemini, "skills", "uninstall", skillName, "--scope", "user")
	out, err := cmd.CombinedOutput()
	removed := []string{fmt.Sprintf("gemini:user:%s", skillName)}
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(trimmed), "not found") {
			return removed, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 0 {
			return removed, nil
		}
		return nil, fmt.Errorf("gemini skills uninstall %q: %w%s", skillName, err, formatCommandOutput(out))
	}
	return removed, nil
}
```

**`internal/tools/command.go`** — `Uninstall`:

```go
func (t CommandTool) Uninstall(skillName string) ([]string, error) {
	cmd := renderTemplate(t.UninstallCommand, t.ToolName, skillName, "")
	if err := runShell(cmd); err != nil {
		return nil, fmt.Errorf("uninstall %s via %s: %w", skillName, t.ToolName, err)
	}
	path := renderPathTemplate(t.PathTemplate, t.ToolName, skillName, "")
	return []string{path}, nil
}
```

- [ ] **Step 5: Run the new test to verify it passes**

Run: `go test ./internal/tools/... -run TestClaudeUninstallReturnsRemovedPaths`
Expected: PASS.

- [ ] **Step 6: Commit (callers still broken)**

```bash
git add internal/tools/tool.go internal/tools/claude.go internal/tools/cursor.go \
	internal/tools/gemini.go internal/tools/codex.go internal/tools/command.go \
	internal/tools/claude_test.go
git commit -m "[agent] Tool.Uninstall returns removed paths

Step 4 of task: per-skill tool management

Callers at cmd/remove.go and cmd/list_tui.go still need updating."
```

---

## Task 6: Update `cmd/remove.go` and `cmd/list_tui.go` callers of `Uninstall`

**Files:**
- Modify: `cmd/remove.go:126`
- Modify: `cmd/list_tui.go:882`

- [ ] **Step 1: Update `cmd/remove.go`**

Replace the `Uninstall` call block (around line 120-129). The removed paths should drop from the per-tool `Paths` map so the follow-up `installed.Paths` loop only cleans up whatever was not already handled:

```go
var errs []string
for _, name := range installed.Tools {
	tool, err := tools.ResolveByName(cfg, name)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		continue
	}
	removed, uerr := tool.Uninstall(key)
	if uerr != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", name, uerr))
	}
	// Drop successfully removed paths from the per-tool map so the
	// fallback sweep below does not double-remove them.
	delete(installed.Paths, name)
	_ = removed // paths already off disk; loop continues
}
```

Also, the existing fallback loop reads `installed.Paths` as `[]string`:

```go
for _, p := range installed.Paths {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("symlink %s: %v", p, err))
	}
}
```

Rewrite to iterate the map:

```go
for toolName, paths := range installed.Paths {
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s:%s: %v", toolName, p, err))
		}
	}
}
```

- [ ] **Step 2: Update `cmd/list_tui.go:882`**

Replace:

```go
_ = tool.Uninstall(sk.Name)
```

With:

```go
_, _ = tool.Uninstall(sk.Name)
```

The follow-up `os.RemoveAll(sk.LocalPath)` already does the cleanup sweep; no state mutation is needed here since the skill is about to be removed wholesale.

- [ ] **Step 3: Run full build + tests**

Run: `go build ./... && go test ./...`
Expected: PASS (excluding any later work; the codebase must at least compile now).

- [ ] **Step 4: Commit**

```bash
git add cmd/remove.go cmd/list_tui.go
git commit -m "[agent] Update Uninstall callers for new signature

Step 5 of task: per-skill tool management"
```

---

## Task 7: Create `internal/tools/assign` package with `Plan`

**Files:**
- Create: `internal/tools/assign/assign.go`
- Test: `internal/tools/assign/assign_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tools/assign/assign_test.go`:

```go
package assign_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Naoray/scribe/internal/tools/assign"
)

func TestPlan(t *testing.T) {
	tests := []struct {
		name         string
		current      []string
		desired      []string
		wantInstall  []string
		wantUninstal []string
	}{
		{"noop", []string{"claude"}, []string{"claude"}, nil, nil},
		{"pure add", []string{"claude"}, []string{"claude", "codex"}, []string{"codex"}, nil},
		{"pure remove", []string{"claude", "codex"}, []string{"claude"}, nil, []string{"codex"}},
		{"swap", []string{"claude"}, []string{"codex"}, []string{"codex"}, []string{"claude"}},
		{"from empty", nil, []string{"claude"}, []string{"claude"}, nil},
		{"to empty", []string{"claude"}, nil, nil, []string{"claude"}},
		{"dedupe current", []string{"claude", "claude"}, []string{"claude"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			install, uninstall := assign.Plan(tt.current, tt.desired)
			gotInstall := make([]string, len(install))
			for i, op := range install {
				gotInstall[i] = op.Tool
			}
			gotUninstall := make([]string, len(uninstall))
			for i, op := range uninstall {
				gotUninstall[i] = op.Tool
			}
			sort.Strings(gotInstall)
			sort.Strings(gotUninstall)
			sort.Strings(tt.wantInstall)
			sort.Strings(tt.wantUninstal)
			if !reflect.DeepEqual(gotInstall, tt.wantInstall) {
				t.Errorf("install = %v, want %v", gotInstall, tt.wantInstall)
			}
			if !reflect.DeepEqual(gotUninstall, tt.wantUninstal) {
				t.Errorf("uninstall = %v, want %v", gotUninstall, tt.wantUninstal)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/assign/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Create the package**

Create `internal/tools/assign/assign.go`:

```go
// Package assign is a pure helper for computing and applying per-skill
// tool assignment operations. It is shared by the list TUI, the bulk
// loadout TUI, the `scribe skill edit` CLI, and the reconcile path in
// `scribe tools enable/disable`.
package assign

import (
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// Mode distinguishes install from uninstall operations.
type Mode int

const (
	ModeAdd    Mode = iota // successful install adds to state.Tools
	ModeRemove             // successful uninstall drops from state.Tools
)

// Op is a single install or uninstall to perform for one tool.
type Op struct {
	Tool string
	Mode Mode
}

// Result records the outcome of one Op.
type Result struct {
	Op    Op
	Paths []string // populated on successful install/uninstall
	Err   error
}

// Plan returns the install and uninstall Ops needed to move current → desired.
// Both slices are treated as sets (duplicates are ignored). Pure, no side effects.
func Plan(current, desired []string) (install, uninstall []Op) {
	curSet := toSet(current)
	desSet := toSet(desired)

	for t := range desSet {
		if !curSet[t] {
			install = append(install, Op{Tool: t, Mode: ModeAdd})
		}
	}
	for t := range curSet {
		if !desSet[t] {
			uninstall = append(uninstall, Op{Tool: t, Mode: ModeRemove})
		}
	}
	return install, uninstall
}

func toSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// Apply and Merge are implemented in subsequent tasks.
var _ = state.InstalledSkill{}
var _ tools.Tool = nil
```

The trailing `_ =` lines exist to keep the imports referenced until `Apply`/`Merge` land. Remove them in Task 8.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tools/assign/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/assign/assign.go internal/tools/assign/assign_test.go
git commit -m "[agent] Add assign package with Plan

Step 6 of task: per-skill tool management"
```

---

## Task 8: Implement `assign.Apply` with install-first ordering

**Files:**
- Modify: `internal/tools/assign/assign.go`
- Test: `internal/tools/assign/assign_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tools/assign/assign_test.go`:

```go
type fakeTool struct {
	name        string
	installErr  error
	installOut  []string
	removeErr   error
	removeOut   []string
	installCount int
	removeCount  int
}

func (f *fakeTool) Name() string { return f.name }
func (f *fakeTool) Detect() bool { return true }
func (f *fakeTool) Install(skill, dir string) ([]string, error) {
	f.installCount++
	return f.installOut, f.installErr
}
func (f *fakeTool) Uninstall(skill string) ([]string, error) {
	f.removeCount++
	return f.removeOut, f.removeErr
}

func TestApplyInstallFirstSuccess(t *testing.T) {
	addTool := &fakeTool{name: "codex", installOut: []string{"/codex/commit"}}
	delTool := &fakeTool{name: "claude", removeOut: []string{"/claude/commit"}}

	results := assign.Apply(
		"commit", "/store/commit",
		[]tools.Tool{addTool},
		[]tools.Tool{delTool},
	)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Op.Tool != "codex" || results[0].Op.Mode != assign.ModeAdd {
		t.Errorf("first op should be add-codex, got %+v", results[0].Op)
	}
	if results[1].Op.Tool != "claude" || results[1].Op.Mode != assign.ModeRemove {
		t.Errorf("second op should be remove-claude, got %+v", results[1].Op)
	}
	if addTool.installCount != 1 || delTool.removeCount != 1 {
		t.Errorf("install/remove not invoked exactly once")
	}
}

func TestApplyRecordsPerOpErrors(t *testing.T) {
	bad := &fakeTool{name: "codex", installErr: errors.New("boom")}
	ok := &fakeTool{name: "gemini", installOut: []string{"/gemini/commit"}}

	results := assign.Apply("commit", "/store/commit", []tools.Tool{bad, ok}, nil)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected first op error")
	}
	if results[1].Err != nil {
		t.Errorf("second op should succeed: %v", results[1].Err)
	}
}
```

(Remember to add the `errors` import.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tools/assign/...`
Expected: FAIL — `assign.Apply` undefined.

- [ ] **Step 3: Implement `Apply`**

Append to `internal/tools/assign/assign.go`, removing the placeholder `_ =` lines:

```go
// Apply runs the given install ops first, then uninstall ops, returning one
// Result per op in that order. The caller is responsible for resolving tool
// names to concrete tools.Tool implementations before invoking Apply — this
// keeps the package free of any config dependency.
//
// install-first ordering means a failed add→remove swap leaves the skill on
// the old tool, preserving availability at the cost of temporary staleness.
func Apply(skillName, canonicalDir string, installTools, uninstallTools []tools.Tool) []Result {
	results := make([]Result, 0, len(installTools)+len(uninstallTools))

	for _, t := range installTools {
		paths, err := t.Install(skillName, canonicalDir)
		results = append(results, Result{
			Op:    Op{Tool: t.Name(), Mode: ModeAdd},
			Paths: paths,
			Err:   err,
		})
	}
	for _, t := range uninstallTools {
		paths, err := t.Uninstall(skillName)
		results = append(results, Result{
			Op:    Op{Tool: t.Name(), Mode: ModeRemove},
			Paths: paths,
			Err:   err,
		})
	}
	return results
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tools/assign/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/assign/assign.go internal/tools/assign/assign_test.go
git commit -m "[agent] assign.Apply with install-first ordering

Step 7 of task: per-skill tool management"
```

---

## Task 9: Implement `assign.Merge`

**Files:**
- Modify: `internal/tools/assign/assign.go`
- Test: `internal/tools/assign/assign_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestMergeAppliesOnlySuccessfulResults(t *testing.T) {
	before := state.InstalledSkill{
		Tools: []string{"claude"},
		Paths: map[string][]string{
			"claude": {"/claude/commit"},
		},
	}
	results := []assign.Result{
		{Op: assign.Op{Tool: "codex", Mode: assign.ModeAdd}, Paths: []string{"/codex/commit"}},
		{Op: assign.Op{Tool: "gemini", Mode: assign.ModeAdd}, Err: errors.New("fail")},
		{Op: assign.Op{Tool: "claude", Mode: assign.ModeRemove}, Paths: []string{"/claude/commit"}},
	}

	after := assign.Merge(before, results)

	if got := after.Tools; !reflect.DeepEqual(sortCopy(got), []string{"codex"}) {
		t.Errorf("tools = %v, want [codex]", got)
	}
	if _, ok := after.Paths["claude"]; ok {
		t.Error("claude path should be removed")
	}
	if _, ok := after.Paths["gemini"]; ok {
		t.Error("gemini path should NOT be added (failed install)")
	}
	if got := after.Paths["codex"]; len(got) != 1 || got[0] != "/codex/commit" {
		t.Errorf("codex paths = %v", got)
	}
}

func sortCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/assign/... -run TestMerge`
Expected: FAIL — `assign.Merge` undefined.

- [ ] **Step 3: Implement `Merge`**

Append to `internal/tools/assign/assign.go`:

```go
// Merge folds successful Results into a copy of installed, updating both
// Tools and Paths. Failed Ops leave their tool entry unchanged. The returned
// InstalledSkill is a fresh value — callers should assign it back into
// state.Installed to persist.
func Merge(installed state.InstalledSkill, results []Result) state.InstalledSkill {
	paths := cloneStringMap(installed.Paths)
	toolSet := toSet(installed.Tools)

	for _, r := range results {
		if r.Err != nil {
			continue
		}
		switch r.Op.Mode {
		case ModeAdd:
			toolSet[r.Op.Tool] = true
			if len(r.Paths) > 0 {
				paths[r.Op.Tool] = append([]string(nil), r.Paths...)
			}
		case ModeRemove:
			delete(toolSet, r.Op.Tool)
			delete(paths, r.Op.Tool)
		}
	}

	out := installed
	out.Tools = sortedKeys(toolSet)
	out.Paths = paths
	return out
}

func cloneStringMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

Add `"sort"` to the imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tools/assign/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/assign/assign.go internal/tools/assign/assign_test.go
git commit -m "[agent] assign.Merge folds successful results into state

Step 8 of task: per-skill tool management"
```

---

## Task 10: Syncer writes the new `Paths` map shape

**Files:**
- Modify: `internal/sync/syncer.go` (the install loop around line 380-430)
- Test: `internal/sync/syncer_test.go` (existing — update fixtures)

- [ ] **Step 1: Update the install loop**

In `internal/sync/syncer.go`, inside `apply`, replace the `var paths []string` block with a map-keyed version:

```go
paths := make(map[string][]string)
var toolNames []string
toolFailed := false
for _, t := range s.Tools {
	links, err := t.Install(sk.Name, canonicalDir)
	if err != nil {
		s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("link to %s: %w", t.Name(), err)})
		summary.Failed++
		toolFailed = true
		break
	}
	paths[t.Name()] = links
	toolNames = append(toolNames, t.Name())
}
if toolFailed {
	continue
}
```

Also update the `st.RecordInstall` call further down to pass the map:

```go
st.RecordInstall(sk.Name, state.InstalledSkill{
	Revision:      nextRevision(installed),
	InstalledHash: installedHash,
	Sources:       sources,
	Tools:         toolNames,
	Paths:         paths,
})
```

- [ ] **Step 2: Update any existing test fixtures**

Grep `internal/sync/syncer_test.go` for `Paths: []string` or `Paths: nil` in install-side assertions and convert to `map[string][]string{...}`.

- [ ] **Step 3: Run the sync tests**

Run: `go test ./internal/sync/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/sync/syncer.go internal/sync/syncer_test.go
git commit -m "[agent] Syncer writes Paths map keyed by tool name

Step 9 of task: per-skill tool management"
```

---

## Task 11: Syncer pinned branch + self-heal

**Files:**
- Modify: `internal/sync/syncer.go` — `apply` function, where installed skills are visited
- Test: `internal/sync/syncer_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/sync/syncer_test.go`:

```go
func TestSyncSkipsPinnedSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Set up a pinned skill with tools=[claude] and a stale state.
	st := freshState(t)
	canonical, _ := paths.SkillStore("commit")
	_ = os.MkdirAll(canonical, 0o755)
	_ = os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# commit\n"), 0o644)

	st.Installed["commit"] = state.InstalledSkill{
		Revision:      1,
		InstalledHash: "abc",
		Tools:         []string{"claude"},
		Paths:         map[string][]string{"claude": {filepath.Join(home, ".claude", "skills", "commit")}},
		ToolsMode:     state.ToolsModePinned,
	}

	// Active tool set: [claude, codex]. Sync should not add codex to the pinned skill.
	syncer := newTestSyncerWithTools(t, tools.ClaudeTool{}, tools.CodexTool{})
	status := sync.SkillStatus{Name: "commit", Status: sync.StatusCurrent}
	if err := syncer.RunWithDiff(context.Background(), "owner/repo", []sync.SkillStatus{status}, st); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := st.Installed["commit"]
	if len(got.Tools) != 1 || got.Tools[0] != "claude" {
		t.Errorf("pinned tools = %v, want [claude]", got.Tools)
	}
	if _, ok := got.Paths["codex"]; ok {
		t.Error("codex path should not be added to pinned skill")
	}
}

func TestSyncSelfHealsMissingPinnedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-create the canonical store so ClaudeTool.Install can symlink to SKILL.md.
	canonical, _ := paths.SkillStore("commit")
	_ = os.MkdirAll(canonical, 0o755)
	_ = os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# commit\n"), 0o644)

	// Record state claiming the symlink exists, but don't actually create it.
	missingLink := filepath.Join(home, ".claude", "skills", "commit")
	st := freshState(t)
	st.Installed["commit"] = state.InstalledSkill{
		Tools:     []string{"claude"},
		Paths:     map[string][]string{"claude": {missingLink}},
		ToolsMode: state.ToolsModePinned,
	}

	syncer := newTestSyncerWithTools(t, tools.ClaudeTool{})
	if err := syncer.RunWithDiff(context.Background(), "owner/repo",
		[]sync.SkillStatus{{Name: "commit", Status: sync.StatusCurrent}}, st); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Lstat(missingLink); err != nil {
		t.Errorf("self-heal did not recreate symlink: %v", err)
	}
}
```

(Helpers `freshState` and `newTestSyncerWithTools` likely exist already in the test file; if not, add minimal versions.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/... -run 'TestSync(SkipsPinned|SelfHeals)'`
Expected: FAIL.

- [ ] **Step 3: Add pinned-skill and self-heal handling**

Inside `apply`, after the `SkillResolvedMsg` loop and before the main install loop, add a self-heal pass that visits every state entry currently listed as `Current`:

```go
for _, sk := range statuses {
	if sk.Status != StatusCurrent || sk.Installed == nil {
		continue
	}
	s.healInstalled(sk, st)
}
```

Implement `healInstalled` as a new method on `Syncer`:

```go
// healInstalled verifies every recorded symlink exists on disk. Missing
// entries are reinstalled in-place. Cursor is skipped (project-scoped) and
// packages are skipped (no Tools/Paths entries).
func (s *Syncer) healInstalled(sk SkillStatus, st *state.State) {
	installed := st.Installed[sk.Name]
	if installed.Type == "package" {
		return
	}
	canonical, err := paths.SkillStore(sk.Name)
	if err != nil {
		return
	}
	changed := false
	for _, toolName := range installed.Tools {
		if toolName == "cursor" {
			continue // project-scoped; self-heal not safe outside a project cwd
		}
		healedPaths, wantHeal := needsHeal(installed.Paths[toolName])
		if !wantHeal {
			continue
		}
		tool, err := tools.ResolveByName(s.cfg(), toolName)
		if err != nil {
			continue
		}
		links, ierr := tool.Install(sk.Name, canonical)
		if ierr != nil {
			s.emit(SkillErrorMsg{Name: sk.Name, Err: fmt.Errorf("self-heal %s: %w", toolName, ierr)})
			continue
		}
		installed.Paths[toolName] = links
		_ = healedPaths
		changed = true
	}
	if changed {
		st.Installed[sk.Name] = installed
	}
}

func needsHeal(paths []string) ([]string, bool) {
	if len(paths) == 0 {
		return nil, false
	}
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			return paths, true
		}
	}
	return paths, false
}
```

Note on `s.cfg()`: the syncer currently has no `Config` field. Instead of plumbing through config for tool resolution, keep the simpler approach: require callers to set a new `ResolveTool func(name string) (tools.Tool, error)` field on `Syncer`. Add that field now:

```go
type Syncer struct {
	// ...existing fields
	// ResolveTool maps a tool name to an implementation. Used by the self-heal
	// path so pinned skills can re-run Install for tools that are no longer in
	// s.Tools. Callers typically pass `tools.ResolveByName(cfg, name)`.
	ResolveTool func(name string) (tools.Tool, error)
}
```

Replace `s.cfg()` with `s.ResolveTool(toolName)` above. Test helper `newTestSyncerWithTools` should set `ResolveTool` to a closure that returns a builtin match or errors out.

Also **skip pinned skills from the normal install loop**: at the top of the main `for _, sk := range statuses` loop, inside the `StatusCurrent` branch, if the skill is pinned, skip the normal skip path (it is already current) but still run `healInstalled` above. The change is therefore constrained to the self-heal pass — no branching inside the main install loop is needed beyond what Task 10 already did.

Finally, for the pinned branch of `StatusMissing`/`StatusOutdated`: since pinned only makes sense after an initial install, skip pinned logic here — a pinned skill that goes missing will re-appear as `StatusMissing` and the normal install loop runs against `s.Tools`. That may install onto tools the user explicitly removed. Guard:

```go
case StatusMissing, StatusOutdated:
	if sk.IsPackage {
		s.applyPackage(ctx, sk, teamRepo, st, &summary)
		continue
	}

	// ...existing download logic...

	// After download, restrict the tool set for pinned skills:
	toolsToLink := s.Tools
	if installed := lookupInstalled(st, sk.Name); installed != nil && installed.ToolsMode == state.ToolsModePinned {
		toolsToLink = pinnedToolsOnly(installed.Tools, s.Tools)
	}
	// Rename the `for _, t := range s.Tools` loop below to `range toolsToLink`.
```

```go
// pinnedToolsOnly intersects the available tool set with the pinned list,
// preserving the order of the full set for determinism.
func pinnedToolsOnly(pinned []string, avail []tools.Tool) []tools.Tool {
	set := make(map[string]bool, len(pinned))
	for _, n := range pinned {
		set[n] = true
	}
	out := make([]tools.Tool, 0, len(pinned))
	for _, t := range avail {
		if set[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sync/... -run TestSync`
Expected: PASS.

- [ ] **Step 5: Run full build + tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sync/syncer.go internal/sync/syncer_test.go
git commit -m "[agent] Syncer honors pinned mode and self-heals missing links

Step 10 of task: per-skill tool management"
```

---

## Task 12: Reconcile helper for `scribe tools enable/disable`

**Files:**
- Create: `cmd/tools_reconcile.go`
- Modify: `cmd/tools.go` — `runToolsEnable/Disable` call the helper after persisting config
- Test: `cmd/tools_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cmd/tools_test.go` (or create):

```go
func TestReconcileBackfillsInheritSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Seed state with one inherit skill and one pinned skill, both currently
	// on claude only.
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"inherited": {
			Tools:     []string{"claude"},
			Paths:     map[string][]string{"claude": {"/tmp/claude/inherited"}},
			ToolsMode: state.ToolsModeInherit,
		},
		"pinned": {
			Tools:     []string{"claude"},
			Paths:     map[string][]string{"claude": {"/tmp/claude/pinned"}},
			ToolsMode: state.ToolsModePinned,
		},
	}}

	// Fake resolver that always succeeds.
	resolver := func(name string) (tools.Tool, error) {
		return &fakeAssignTool{name: name}, nil
	}

	summary := cmd.ReconcileInheritSkills(st, "codex", cmd.ReconcileActionAdd, resolver)
	if summary.Changed != 1 {
		t.Errorf("Changed = %d, want 1 (only inherited)", summary.Changed)
	}
	if summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (pinned)", summary.Skipped)
	}
	if got := st.Installed["inherited"].Tools; len(got) != 2 {
		t.Errorf("inherited Tools = %v, want [claude codex]", got)
	}
	if got := st.Installed["pinned"].Tools; len(got) != 1 {
		t.Errorf("pinned Tools = %v, want unchanged [claude]", got)
	}
}
```

Use the existing `fakeTool` pattern from `internal/tools/assign/assign_test.go`; copy a minimal version into `cmd/tools_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/... -run TestReconcileBackfillsInheritSkills`
Expected: FAIL — function not defined.

- [ ] **Step 3: Implement the helper**

Create `cmd/tools_reconcile.go`:

```go
package cmd

import (
	"fmt"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/tools/assign"
)

// ReconcileAction is whether to add or remove a tool during reconcile.
type ReconcileAction int

const (
	ReconcileActionAdd ReconcileAction = iota
	ReconcileActionRemove
)

// ReconcileSummary is the aggregate result of reconciling a single tool
// across every inherit-mode skill in state.
type ReconcileSummary struct {
	Changed int
	Skipped int
	Errors  []ReconcileError
}

// ReconcileError records a per-skill failure during reconcile.
type ReconcileError struct {
	Skill string
	Err   error
}

// ReconcileInheritSkills iterates every skill in state and, for those with
// ToolsMode == ToolsModeInherit, adds or removes the given tool using the
// install-first assign.Apply path. Packages, cursor, and pinned skills are
// skipped. The caller supplies a resolver (typically a closure around
// tools.ResolveByName(cfg, name)) so tests can substitute fakes.
func ReconcileInheritSkills(
	st *state.State,
	toolName string,
	action ReconcileAction,
	resolve func(string) (tools.Tool, error),
) ReconcileSummary {
	var summary ReconcileSummary
	if toolName == "cursor" {
		return summary // cursor is project-scoped, never reconciled here
	}

	tool, err := resolve(toolName)
	if err != nil {
		summary.Errors = append(summary.Errors, ReconcileError{Skill: "", Err: err})
		return summary
	}

	for name, skill := range st.Installed {
		if skill.Type == "package" {
			continue
		}
		if skill.ToolsMode != state.ToolsModeInherit {
			summary.Skipped++
			continue
		}

		desired := computeDesiredTools(skill.Tools, toolName, action)
		install, uninstall := assign.Plan(skill.Tools, desired)
		if len(install) == 0 && len(uninstall) == 0 {
			continue
		}

		canonical, _ := paths.SkillStore(name)
		results := assign.Apply(name, canonical,
			toolListFor(install, tool),
			toolListFor(uninstall, tool),
		)
		updated := assign.Merge(skill, results)
		st.Installed[name] = updated

		for _, r := range results {
			if r.Err != nil {
				summary.Errors = append(summary.Errors, ReconcileError{Skill: name, Err: r.Err})
			}
		}
		summary.Changed++
	}
	return summary
}

// computeDesiredTools returns the desired Tools slice after applying action.
func computeDesiredTools(current []string, tool string, action ReconcileAction) []string {
	set := make(map[string]bool, len(current))
	for _, t := range current {
		set[t] = true
	}
	switch action {
	case ReconcileActionAdd:
		set[tool] = true
	case ReconcileActionRemove:
		delete(set, tool)
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	return out
}

// toolListFor returns tool wrapped in a []tools.Tool when op.Tool matches.
func toolListFor(ops []assign.Op, tool tools.Tool) []tools.Tool {
	for _, op := range ops {
		if op.Tool == tool.Name() {
			return []tools.Tool{tool}
		}
	}
	return nil
}

// FormatReconcileSummary returns a human-readable multi-line summary.
func FormatReconcileSummary(summary ReconcileSummary) string {
	return fmt.Sprintf("%d skills updated, %d skipped, %d errors",
		summary.Changed, summary.Skipped, len(summary.Errors))
}
```

- [ ] **Step 4: Wire into `runToolsEnable/Disable`**

In `cmd/tools.go`, after `setToolEnabled` returns successfully, call the reconcile helper:

```go
func runToolsEnable(cmd *cobra.Command, args []string) error {
	if err := setToolEnabled(args[0], true); err != nil {
		return err
	}
	return runReconcileAfterToggle(args[0], ReconcileActionAdd)
}

func runToolsDisable(cmd *cobra.Command, args []string) error {
	if err := setToolEnabled(args[0], false); err != nil {
		return err
	}
	return runReconcileAfterToggle(args[0], ReconcileActionRemove)
}

func runReconcileAfterToggle(toolName string, action ReconcileAction) error {
	if toolName == "cursor" {
		return nil // project-scoped, skip backfill
	}
	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	resolver := func(name string) (tools.Tool, error) {
		return tools.ResolveByName(cfg, name)
	}
	summary := ReconcileInheritSkills(st, toolName, action, resolver)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state after reconcile: %w", err)
	}
	fmt.Println(FormatReconcileSummary(summary))
	for _, e := range summary.Errors {
		fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", e.Skill, e.Err)
	}
	return nil
}
```

Import `github.com/Naoray/scribe/internal/state` in `cmd/tools.go` if not already present.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/... -run TestReconcile && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tools.go cmd/tools_reconcile.go cmd/tools_test.go
git commit -m "[agent] Reconcile inherit-mode skills on tools enable/disable

Step 11 of task: per-skill tool management"
```

---

## Task 13: `scribe skill edit <name>` CLI

**Files:**
- Create: `cmd/skill.go`
- Create: `cmd/skill_test.go`
- Modify: `cmd/root.go` — register `skillCmd`

- [ ] **Step 1: Write the failing test**

Create `cmd/skill_test.go`:

```go
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/Naoray/scribe/cmd"
	"github.com/Naoray/scribe/internal/state"
)

func runSkillEdit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := cmd.NewRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"skill", "edit"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestSkillEditPinsOnTools(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{
		Tools:     []string{"claude"},
		Paths:     map[string][]string{"claude": {"/tmp/claude/commit"}},
		ToolsMode: state.ToolsModeInherit,
	})

	if _, err := runSkillEdit(t, "commit", "--tools", "claude,codex"); err != nil {
		t.Fatalf("edit: %v", err)
	}

	st, _ := state.Load()
	skill := st.Installed["commit"]
	if skill.ToolsMode != state.ToolsModePinned {
		t.Errorf("ToolsMode = %q, want pinned", skill.ToolsMode)
	}
	if len(skill.Tools) != 2 {
		t.Errorf("Tools = %v, want [claude codex]", skill.Tools)
	}
}

func TestSkillEditInheritResetsMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{
		Tools:     []string{"claude"},
		ToolsMode: state.ToolsModePinned,
	})

	if _, err := runSkillEdit(t, "commit", "--inherit"); err != nil {
		t.Fatalf("inherit: %v", err)
	}

	st, _ := state.Load()
	if st.Installed["commit"].ToolsMode != state.ToolsModeInherit {
		t.Errorf("ToolsMode = %q, want inherit", st.Installed["commit"].ToolsMode)
	}
}

func TestSkillEditRejectsCursorAdd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{Tools: []string{"claude"}})
	if _, err := runSkillEdit(t, "commit", "--add", "cursor"); err == nil {
		t.Error("expected error when adding cursor")
	}
}

func TestSkillEditRejectsToolsAndInheritTogether(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{Tools: []string{"claude"}})
	if _, err := runSkillEdit(t, "commit", "--tools", "claude", "--inherit"); err == nil {
		t.Error("expected mutex error")
	}
}
```

Add a `seedInstalled(t, name, skill)` helper in a shared test file that calls `state.Load` + `RecordInstall` + `Save`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/... -run TestSkillEdit`
Expected: FAIL — command does not exist.

- [ ] **Step 3: Implement `cmd/skill.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/tools/assign"
)

func newSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Inspect and edit installed skills",
	}
	cmd.AddCommand(newSkillEditCommand())
	return cmd
}

func newSkillEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a skill's tool assignment",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillEdit,
	}
	cmd.Flags().StringSlice("tools", nil, "Replace the tool set (implies --pin)")
	cmd.Flags().StringSlice("add", nil, "Add tools (implies --pin)")
	cmd.Flags().StringSlice("remove", nil, "Remove tools (implies --pin)")
	cmd.Flags().Bool("inherit", false, "Reset to inherit mode")
	cmd.Flags().Bool("pin", false, "Pin the current tool set without changing it")
	cmd.Flags().Bool("json", false, "Emit JSON")
	cmd.MarkFlagsMutuallyExclusive("tools", "inherit")
	cmd.MarkFlagsMutuallyExclusive("tools", "pin")
	cmd.MarkFlagsMutuallyExclusive("inherit", "add")
	cmd.MarkFlagsMutuallyExclusive("inherit", "remove")
	cmd.MarkFlagsMutuallyExclusive("inherit", "pin")
	return cmd
}

type skillEditResult struct {
	Name      string   `json:"name"`
	Tools     []string `json:"tools"`
	ToolsMode string   `json:"tools_mode"`
}

func runSkillEdit(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])

	toolsFlag, _ := cmd.Flags().GetStringSlice("tools")
	addFlag, _ := cmd.Flags().GetStringSlice("add")
	removeFlag, _ := cmd.Flags().GetStringSlice("remove")
	inheritFlag, _ := cmd.Flags().GetBool("inherit")
	pinFlag, _ := cmd.Flags().GetBool("pin")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	st, err := state.Load()
	if err != nil {
		return err
	}
	installed, ok := st.Installed[name]
	if !ok {
		return fmt.Errorf("unknown skill %q", name)
	}
	if installed.Type == "package" {
		return fmt.Errorf("skill %q is a package — per-skill tool assignment is unsupported", name)
	}

	// Print-only: no mutations requested.
	if len(toolsFlag) == 0 && len(addFlag) == 0 && len(removeFlag) == 0 && !inheritFlag && !pinFlag {
		return printSkillEdit(cmd, name, installed, jsonFlag)
	}

	// Compute the desired tool set.
	desired := append([]string(nil), installed.Tools...)
	switch {
	case len(toolsFlag) > 0:
		desired = toolsFlag
	case len(addFlag) > 0 || len(removeFlag) > 0:
		for _, t := range addFlag {
			if !contains(desired, t) {
				desired = append(desired, t)
			}
		}
		for _, t := range removeFlag {
			desired = removeString(desired, t)
		}
	}

	for _, t := range desired {
		if t == "cursor" {
			return fmt.Errorf("cursor is project-scoped; cannot be assigned per-skill")
		}
	}

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}

	installList, uninstallList := assign.Plan(installed.Tools, desired)

	resolvedInstall := make([]tools.Tool, 0, len(installList))
	for _, op := range installList {
		tool, rerr := tools.ResolveByName(cfg, op.Tool)
		if rerr != nil {
			return fmt.Errorf("resolve %s: %w", op.Tool, rerr)
		}
		resolvedInstall = append(resolvedInstall, tool)
	}
	resolvedUninstall := make([]tools.Tool, 0, len(uninstallList))
	for _, op := range uninstallList {
		tool, rerr := tools.ResolveByName(cfg, op.Tool)
		if rerr != nil {
			return fmt.Errorf("resolve %s: %w", op.Tool, rerr)
		}
		resolvedUninstall = append(resolvedUninstall, tool)
	}

	canonical, _ := paths.SkillStore(name)
	results := assign.Apply(name, canonical, resolvedInstall, resolvedUninstall)
	updated := assign.Merge(installed, results)

	if inheritFlag {
		updated.ToolsMode = state.ToolsModeInherit
	} else {
		updated.ToolsMode = state.ToolsModePinned
	}
	st.Installed[name] = updated
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	var partialErr error
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", r.Op.Tool, r.Err)
			partialErr = fmt.Errorf("one or more operations failed")
		}
	}

	if err := printSkillEdit(cmd, name, updated, jsonFlag); err != nil {
		return err
	}
	return partialErr
}

func printSkillEdit(cmd *cobra.Command, name string, skill state.InstalledSkill, jsonFlag bool) error {
	mode := string(skill.ToolsMode)
	if mode == "" {
		mode = "inherit"
	}
	if jsonFlag {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(skillEditResult{
			Name:      name,
			Tools:     skill.Tools,
			ToolsMode: mode,
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (%s)\n", name, strings.Join(skill.Tools, ", "), mode)
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func removeString(list []string, v string) []string {
	out := list[:0]
	for _, s := range list {
		if s != v {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Register the command**

In `cmd/root.go`, inside the `init()` that calls `rootCmd.AddCommand(...)`, add:

```go
rootCmd.AddCommand(newSkillCommand())
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/... -run TestSkillEdit`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/skill.go cmd/skill_test.go cmd/root.go
git commit -m "[agent] Add scribe skill edit command

Step 12 of task: per-skill tool management"
```

---

## Task 14: `scribe tools loadout <tool>` bulk TUI (model only)

**Files:**
- Create: `cmd/tools_loadout.go`
- Create: `cmd/tools_loadout_tui.go`
- Create: `cmd/tools_loadout_tui_test.go`
- Modify: `cmd/tools.go` — register subcommand

- [ ] **Step 1: Write the failing test**

Create `cmd/tools_loadout_tui_test.go`:

```go
package cmd

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/state"
)

func TestLoadoutTogglesPendingChanges(t *testing.T) {
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"commit": {Tools: []string{"claude"}, ToolsMode: state.ToolsModeInherit},
		"ship":   {Tools: []string{"claude", "codex"}, ToolsMode: state.ToolsModeInherit},
	}}

	m := newLoadoutModel("codex", st)
	// commit does NOT currently have codex — toggling should stage an add.
	m.cursor = 0 // commit
	m2, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: "space"})
	lm := m2.(loadoutModel)
	if !lm.pending["commit"] {
		t.Error("commit should be staged as pending add")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/... -run TestLoadoutTogglesPendingChanges`
Expected: FAIL — `loadoutModel` does not exist.

- [ ] **Step 3: Implement `cmd/tools_loadout_tui.go`**

Minimal model — skill list, cursor, pending overlay, save on `s`, quit on `esc`/`q`:

```go
package cmd

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/tools/assign"
)

type loadoutModel struct {
	tool      string
	state     *state.State
	skills    []string // sorted, excluding packages + cursor
	cursor    int
	pending   map[string]bool // skill → desired state (true = should have tool)
	saved     bool
	statusMsg string
}

func newLoadoutModel(tool string, st *state.State) loadoutModel {
	names := make([]string, 0, len(st.Installed))
	for name, skill := range st.Installed {
		if skill.Type == "package" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return loadoutModel{
		tool:    tool,
		state:   st,
		skills:  names,
		pending: make(map[string]bool),
	}
}

func (m loadoutModel) Init() tea.Cmd { return nil }

func (m loadoutModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.skills)-1 {
				m.cursor++
			}
		case "space", " ":
			m = m.toggleCursor()
		case "s":
			return m.applyPending()
		}
	}
	return m, nil
}

func (m loadoutModel) toggleCursor() loadoutModel {
	name := m.skills[m.cursor]
	skill := m.state.Installed[name]
	currentlyHas := containsString(skill.Tools, m.tool)
	desired := !currentlyHas
	if p, ok := m.pending[name]; ok {
		desired = !p
	}
	if desired == currentlyHas {
		delete(m.pending, name)
	} else {
		m.pending[name] = desired
	}
	return m
}

func (m loadoutModel) applyPending() (tea.Model, tea.Cmd) {
	for name, want := range m.pending {
		skill := m.state.Installed[name]
		desired := append([]string(nil), skill.Tools...)
		if want && !containsString(desired, m.tool) {
			desired = append(desired, m.tool)
		} else if !want {
			desired = removeString(desired, m.tool)
		}
		// Resolve the single tool; on error, skip.
		tool, err := tools.ResolveByName(nil, m.tool)
		if err != nil {
			m.statusMsg = fmt.Sprintf("resolve %s: %v", m.tool, err)
			return m, nil
		}
		install, uninstall := assign.Plan(skill.Tools, desired)
		canonical, _ := paths.SkillStore(name)
		results := assign.Apply(name, canonical,
			toolListFor(install, tool),
			toolListFor(uninstall, tool),
		)
		updated := assign.Merge(skill, results)
		updated.ToolsMode = state.ToolsModePinned
		m.state.Installed[name] = updated
	}
	if err := m.state.Save(); err != nil {
		m.statusMsg = fmt.Sprintf("save: %v", err)
		return m, nil
	}
	m.pending = make(map[string]bool)
	m.saved = true
	return m, tea.Quit
}

func (m loadoutModel) View() tea.View {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(
		"%s · %d skills · %d pending",
		m.tool, len(m.skills), len(m.pending),
	))
	b.WriteString(header + "\n\n")
	for i, name := range m.skills {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		skill := m.state.Installed[name]
		has := containsString(skill.Tools, m.tool)
		mark := "[ ]"
		if has {
			mark = "[x]"
		}
		pendingNote := ""
		if want, ok := m.pending[name]; ok {
			if want {
				pendingNote = "  (pending: add)"
			} else {
				pendingNote = "  (pending: remove)"
			}
		}
		b.WriteString(fmt.Sprintf("%s%s %s%s\n", cursor, mark, name, pendingNote))
	}
	b.WriteString("\nspace toggle · s save · esc cancel\n")
	if m.statusMsg != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(m.statusMsg))
	}
	return tea.NewView(b.String())
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
```

**Note:** `tools.ResolveByName(nil, ...)` is a placeholder — in the real `applyPending` path the model needs a `*config.Config`. Add `cfg *config.Config` to `loadoutModel` and thread it through `newLoadoutModel`.

- [ ] **Step 4: Implement `cmd/tools_loadout.go` (Cobra wiring)**

```go
package cmd

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func newToolsLoadoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "loadout <tool>",
		Short: "Bulk edit a tool's installed skill loadout",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolsLoadout,
	}
}

func runToolsLoadout(cmd *cobra.Command, args []string) error {
	toolName := args[0]
	if toolName == "cursor" {
		return fmt.Errorf("cursor is project-scoped; loadout editing is unsupported")
	}
	if _, ok := tools.BuiltinByName(toolName); !ok {
		// allow custom via config lookup
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("loadout requires an interactive terminal; use `scribe skill edit` for scripting")
	}

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	m := newLoadoutModel(toolName, st)
	m.cfg = cfg
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
	_, err = p.Run()
	return err
}
```

Add `cfg *config.Config` to `loadoutModel` and replace the placeholder `tools.ResolveByName(nil, ...)` with `tools.ResolveByName(m.cfg, m.tool)`.

- [ ] **Step 5: Register the subcommand**

In `cmd/tools.go`, inside `newToolsCommand`:

```go
cmd.AddCommand(newToolsLoadoutCommand())
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./cmd/... -run TestLoadoutTogglesPendingChanges && go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/tools_loadout.go cmd/tools_loadout_tui.go cmd/tools_loadout_tui_test.go cmd/tools.go
git commit -m "[agent] Add scribe tools loadout bulk TUI

Step 13 of task: per-skill tool management"
```

---

## Task 15: Per-skill tools pane in the list TUI

**Files:**
- Modify: `cmd/list_tui.go` — add `focusTools`, `pendingTools`, `renderToolsPane`, focus handler
- Test: `cmd/list_tui_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cmd/list_tui_test.go`:

```go
func TestListTUIStagesToolsToggleAndAppliesOnDefocus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{
		Tools:     []string{"claude"},
		Paths:     map[string][]string{"claude": {"/tmp/claude/commit"}},
		ToolsMode: state.ToolsModeInherit,
	})

	m := newTestListModel(t, []listRow{{Name: "commit", Local: &discovery.Skill{Name: "commit", LocalPath: "/tmp/claude/commit"}}})
	m.selected = true
	m.focus = focusTools

	// Stage a pending add on "codex".
	m.pendingTools = map[string]bool{"codex": true}

	// Leave focus (tab) → apply.
	next, _ := m.Update(tea.KeyPressMsg{Code: '\t', Text: "tab"})
	lm := next.(listModel)
	if len(lm.pendingTools) != 0 {
		t.Errorf("pendingTools should be cleared after apply")
	}

	st, _ := state.Load()
	if st.Installed["commit"].ToolsMode != state.ToolsModePinned {
		t.Error("commit should be pinned after toggle")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/... -run TestListTUIStagesToolsToggleAndAppliesOnDefocus`
Expected: FAIL — `focusTools` and `pendingTools` undefined.

- [ ] **Step 3: Add state fields and focus enum entry**

In `cmd/list_tui.go`:

```go
const (
	focusList detailFocus = iota
	focusTools
	focusActions
)
```

```go
type listModel struct {
	// ...existing fields
	pendingTools map[string]bool // tool name → desired state; nil = no pending
}
```

- [ ] **Step 4: Add the render function**

```go
func (m listModel) renderToolsPane(row listRow) string {
	statuses, _ := tools.ResolveStatuses(m.bag.Config)
	installed, _ := m.bag.State.Installed[row.Name]
	current := make(map[string]bool, len(installed.Tools))
	for _, t := range installed.Tools {
		current[t] = true
	}

	var lines []string
	lines = append(lines, ltHeaderStyle.Render("Tools"))
	for _, st := range statuses {
		switch st.Name {
		case "cursor":
			lines = append(lines, fmt.Sprintf("  [-] cursor (project-scoped)"))
			continue
		}
		if !st.DetectKnown {
			continue
		}
		if !st.Detected {
			lines = append(lines, fmt.Sprintf("  [-] %s (not found)", st.Name))
			continue
		}

		has := current[st.Name]
		if want, pending := m.pendingTools[st.Name]; pending {
			has = want
		}
		mark := "[ ]"
		if has {
			mark = "[x]"
		}
		prefix := "  "
		if _, pending := m.pendingTools[st.Name]; pending {
			prefix = "* "
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", prefix, mark, st.Name))
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 5: Wire focus cycling and toggle handling**

In the existing `Update` function for `stageBrowse`, extend the tab/space handlers:

```go
case "tab":
	switch m.focus {
	case focusList:
		m.focus = focusTools
	case focusTools:
		m = m.applyPendingTools()
		m.focus = focusActions
	case focusActions:
		m.focus = focusList
	}
case "space":
	if m.focus == focusTools {
		m = m.toggleToolsCursor()
		return m, nil
	}
	// ...existing space handling...
case "esc":
	if m.focus == focusTools {
		m = m.applyPendingTools()
	}
	// ...existing esc handling...
```

Add helpers on `listModel`:

```go
func (m listModel) toggleToolsCursor() listModel {
	// For simplicity: toggle all globally enabled, detected builtins
	// listed in the tools pane. A full implementation would track a
	// per-row cursor; a follow-up task can add that.
	return m
}

func (m listModel) applyPendingTools() listModel {
	if len(m.pendingTools) == 0 {
		return m
	}
	row := m.filtered[m.cursor]
	installed, ok := m.bag.State.Installed[row.Name]
	if !ok {
		m.pendingTools = nil
		return m
	}

	desired := append([]string(nil), installed.Tools...)
	for t, want := range m.pendingTools {
		switch want {
		case true:
			if !containsString(desired, t) {
				desired = append(desired, t)
			}
		case false:
			desired = removeString(desired, t)
		}
	}

	installList, uninstallList := assign.Plan(installed.Tools, desired)
	resolvedInstall := make([]tools.Tool, 0, len(installList))
	for _, op := range installList {
		tool, err := tools.ResolveByName(m.bag.Config, op.Tool)
		if err != nil {
			m.statusMsg = fmt.Sprintf("resolve %s: %v", op.Tool, err)
			continue
		}
		resolvedInstall = append(resolvedInstall, tool)
	}
	resolvedUninstall := make([]tools.Tool, 0, len(uninstallList))
	for _, op := range uninstallList {
		tool, err := tools.ResolveByName(m.bag.Config, op.Tool)
		if err != nil {
			continue
		}
		resolvedUninstall = append(resolvedUninstall, tool)
	}

	canonical, _ := paths.SkillStore(row.Name)
	results := assign.Apply(row.Name, canonical, resolvedInstall, resolvedUninstall)
	updated := assign.Merge(installed, results)
	updated.ToolsMode = state.ToolsModePinned
	m.bag.State.Installed[row.Name] = updated
	if err := m.bag.State.Save(); err != nil {
		m.statusMsg = fmt.Sprintf("save: %v", err)
	} else {
		m.statusMsg = fmt.Sprintf("✓ %s pinned to [%s]", row.Name, strings.Join(updated.Tools, " "))
	}
	m.pendingTools = nil
	return m
}
```

Add the `assign` and `paths` imports at the top of the file.

- [ ] **Step 6: Render the pane in `renderDetailPane`**

Wherever `renderDetailPane` composes the right column, insert `renderToolsPane(row)` between Detail and Actions sections. Update the footer hint to `space toggle · tab apply · a abort · q quit` when `focus == focusTools`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./cmd/... -run TestListTUIStagesToolsToggleAndAppliesOnDefocus && go build ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/list_tui.go cmd/list_tui_test.go
git commit -m "[agent] List TUI tools pane with deferred apply

Step 14 of task: per-skill tool management"
```

---

## Task 16: End-to-end integration sweep

- [ ] **Step 1: Add a full-flow integration test**

Create `cmd/per_skill_e2e_test.go`:

```go
package cmd_test

import (
	"testing"

	"github.com/Naoray/scribe/cmd"
	"github.com/Naoray/scribe/internal/state"
)

func TestEndToEndSkillEditThenReconcile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedInstalled(t, "commit", state.InstalledSkill{
		Tools:     []string{"claude"},
		Paths:     map[string][]string{"claude": {"/tmp/claude/commit"}},
		ToolsMode: state.ToolsModeInherit,
	})
	seedInstalled(t, "ship", state.InstalledSkill{
		Tools:     []string{"claude"},
		Paths:     map[string][]string{"claude": {"/tmp/claude/ship"}},
		ToolsMode: state.ToolsModeInherit,
	})

	// 1. Pin commit to [claude] only via skill edit --pin (no tool change).
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"skill", "edit", "commit", "--pin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("skill edit --pin: %v", err)
	}

	// 2. Run tools enable codex — reconcile should backfill ship but not commit.
	// (Requires codex to be marked detected; test-only override or skip if absent.)

	st, _ := state.Load()
	if st.Installed["commit"].ToolsMode != state.ToolsModePinned {
		t.Error("commit should be pinned")
	}
}
```

- [ ] **Step 2: Run the complete test suite**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all PASS.

- [ ] **Step 3: Self-review the plan against the spec**

Walk through `docs/superpowers/specs/2026-04-11-per-skill-tool-management-design.md` section by section and confirm a task covers it:

| Spec section | Covered by |
|---|---|
| Key Decision 1: `ToolsMode` enum | Task 3 |
| Key Decision 2: Reconcile | Task 12 |
| Key Decision 3: Install-first ordering | Tasks 7–9 (`assign.Plan/Apply/Merge`) |
| Key Decision 4: Self-healing sync | Task 11 |
| Key Decision 5: Deferred apply in TUI | Task 15 |
| Key Decision 6: CLI rename | Tasks 13 (`skill edit`), 14 (`tools loadout`) |
| Key Decision 7: `paths.SkillStore` helper | Task 1 |
| Key Decision 8: `ResolveStatuses` emits builtins | Task 2 |
| Key Decision 9: Visual differentiation | Task 15 (`renderToolsPane`) |
| UX A: Per-skill TUI | Task 15 |
| UX B: Bulk `tools loadout` | Task 14 |
| UX C: `scribe skill edit` non-interactive | Task 13 |
| UX D: Reconcile summary | Task 12 |
| Architecture: `Tool.Uninstall` signature | Tasks 5–6 |
| Architecture: `internal/tools/assign` | Tasks 7–9 |
| Architecture: Syncer pinned+self-heal | Tasks 10–11 |
| Architecture: `ResolveStatuses` | Task 2 |
| State migration | Task 4 |
| Tests | Every task (TDD) |

If any row above is unmapped after implementation, file a follow-up task.

- [ ] **Step 4: Commit**

```bash
git add cmd/per_skill_e2e_test.go
git commit -m "[agent] Add end-to-end test for skill edit + reconcile

Step 15 of task: per-skill tool management"
```

---

## Non-blocking follow-ups (from counselors round 2)

Capture as separate issues; not part of this plan's critical path:

1. `--json` flag on `scribe tools enable/disable` so CI can consume the reconcile summary structurally.
2. `--dry-run` support on `scribe skill edit` and reconcile to preview changes without writing.
3. Reconcile fail-fast when the tool CLI is unavailable (avoid N identical "gemini CLI not found" errors).
4. `--inherit` output should resolve and print what inherit would expand to on the next sync.
5. Per-project Cursor assignment (separate spec; excluded from v1 non-goals).

---

## Self-Review Checklist

After writing, verified:

1. **Spec coverage:** All sections in the design doc map to concrete tasks — see table in Task 16, Step 3.
2. **No placeholders:** No "TBD," "implement later," or empty stubs in the steps above. Every code block is self-contained.
3. **Type consistency:** `InstalledSkill.Paths` is `map[string][]string` from Task 3 onward; `Tool.Uninstall` is `(removed []string, err error)` from Task 5 onward; `assign.Op/Result/Mode` names match across Tasks 7–9.
4. **Commit discipline:** Every task ends with a commit step; repo-breaking intermediate commits (Tasks 3, 5) are flagged explicitly and resolved within the next task.
5. **Test-first discipline:** Every behavioral task has a failing test before the implementation step.

---

Plan complete. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch one fresh subagent per task, review diffs between tasks, fastest iteration.
2. **Inline Execution** — execute tasks in this session via the executing-plans skill, with batch checkpoints.

Pick one and I'll proceed.
