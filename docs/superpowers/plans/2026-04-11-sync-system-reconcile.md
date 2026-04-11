# Sync System Reconcile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `scribe sync` reconcile managed filesystem-backed tool projections, preserve divergent unmanaged copies, and expose a repair command for managed drift.

**Architecture:** Add a dedicated reconcile pass driven by `state.Installed` and tool `SkillPath` inspection, then wire it into the sync workflow before and after registry reconciliation. Split desired tool intent from observed managed projections in state, surface conflicts through workflow formatter output and JSON, and add a focused `scribe skill repair` command for operator-driven resolution.

**Tech Stack:** Go, Cobra, existing `internal/workflow`, `internal/state`, `internal/tools`, and `internal/sync` packages

---

### Task 1: Add state support for managed projections and reconcile residue

**Files:**
- Modify: `internal/state/state.go`
- Modify: `internal/state/state_test.go`
- Modify: `internal/state/testdata/state.json`

- [ ] **Step 1: Write the failing state migration tests**

```go
func TestParseAndMigrateSeedsManagedPathsFromPaths(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 4,
	  "installed": {
	    "recap": {
	      "revision": 1,
	      "installed_hash": "abc",
	      "tools": ["codex"],
	      "paths": ["/tmp/.codex/skills/recap"]
	    }
	  }
	}`)

	st, err := parseAndMigrate(raw)
	if err != nil {
		t.Fatalf("parseAndMigrate: %v", err)
	}
	got := st.Installed["recap"]
	if diff := cmp.Diff(got.Paths, got.ManagedPaths); diff != "" {
		t.Fatalf("managed paths mismatch (-want +got):\n%s", diff)
	}
}
```

- [ ] **Step 2: Run the state test to verify it fails**

Run: `go test ./internal/state -run TestParseAndMigrateSeedsManagedPathsFromPaths`
Expected: FAIL because `ManagedPaths` does not exist yet

- [ ] **Step 3: Add the new state fields and migration behavior**

```go
type ProjectionConflict struct {
	Tool      string    `json:"tool"`
	Path      string    `json:"path"`
	FoundHash string    `json:"found_hash"`
	SeenAt    time.Time `json:"seen_at"`
}

type InstalledSkill struct {
	// ...
	Paths               []string             `json:"paths"`
	ManagedPaths        []string             `json:"managed_paths,omitempty"`
	ProjectionConflicts []ProjectionConflict `json:"projection_conflicts,omitempty"`
}
```

- [ ] **Step 4: Run the state package tests**

Run: `go test ./internal/state`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go internal/state/testdata/state.json
git commit -m "feat: track managed skill projections"
```

### Task 2: Add reconcile engine coverage and events

**Files:**
- Create: `internal/reconcile/reconcile.go`
- Create: `internal/reconcile/reconcile_test.go`
- Modify: `internal/tools/tool.go`
- Modify: `internal/sync/events.go`

- [ ] **Step 1: Write failing reconcile tests for missing, same-hash, divergent, and stale paths**

```go
func TestReconcileRepairsMissingCodexProjection(t *testing.T) {}
func TestReconcileNormalizesSameHashDirectory(t *testing.T) {}
func TestReconcilePreservesDivergentDirectoryAsConflict(t *testing.T) {}
func TestReconcileRemovesStaleManagedProjection(t *testing.T) {}
```

- [ ] **Step 2: Run the reconcile tests to verify they fail**

Run: `go test ./internal/reconcile -run TestReconcile`
Expected: FAIL because the package and reconcile types do not exist yet

- [ ] **Step 3: Implement inspectable filesystem-backed tool behavior and reconcile results**

```go
type Tool interface {
	Name() string
	Install(skillName, canonicalDir string) ([]string, error)
	Uninstall(skillName string) error
	Detect() bool
	SkillPath(skillName string) (string, error)
	SupportsProjectionInspect() bool
}
```

```go
type ActionKind string

const (
	ActionInstalled ActionKind = "installed"
	ActionRelinked  ActionKind = "relinked"
	ActionRemoved   ActionKind = "removed"
	ActionConflict  ActionKind = "conflict"
	ActionUnchanged ActionKind = "unchanged"
)
```

- [ ] **Step 4: Run reconcile and tool tests**

Run: `go test ./internal/reconcile ./internal/tools`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reconcile/reconcile.go internal/reconcile/reconcile_test.go internal/tools/tool.go internal/sync/events.go
git commit -m "feat: add sync projection reconcile engine"
```

### Task 3: Wire reconcile into sync workflow and output

**Files:**
- Modify: `internal/workflow/sync.go`
- Modify: `internal/workflow/formatter.go`
- Modify: `internal/workflow/formatter_text.go`
- Modify: `internal/workflow/formatter_json.go`
- Modify: `internal/workflow/sync_test.go`
- Modify: `internal/workflow/sync_adopt_test.go`
- Modify: `internal/workflow/formatter_test.go`

- [ ] **Step 1: Write failing workflow tests for ordering and summary output**

```go
func TestSyncStepsIncludeReconcileBeforeAndAfterRegistrySync(t *testing.T) {}
func TestJSONFormatterIncludesReconcileSummaryAndConflicts(t *testing.T) {}
func TestTextFormatterPrintsRepairAndConflictSummary(t *testing.T) {}
```

- [ ] **Step 2: Run the workflow tests to verify they fail**

Run: `go test ./internal/workflow -run 'TestSyncStepsIncludeReconcileBeforeAndAfterRegistrySync|TestJSONFormatterIncludesReconcileSummaryAndConflicts|TestTextFormatterPrintsRepairAndConflictSummary'`
Expected: FAIL because reconcile phases and formatter methods are missing

- [ ] **Step 3: Implement reconcile workflow steps before and after registry sync**

```go
return []Step{
	{"LoadConfig", StepLoadConfig},
	{"LoadState", StepLoadState},
	{"CheckConnected", StepCheckConnected},
	{"FilterRegistries", StepFilterRegistries},
	{"ResolveFormatter", StepResolveFormatter},
	{"ResolveTools", StepResolveTools},
	{"Adopt", StepAdopt},
	{"ReconcileSystem", StepReconcileSystem},
	{"SyncSkills", StepSyncSkills},
	{"ReconcileSystem", StepReconcileSystem},
}
```

- [ ] **Step 4: Run the workflow test package**

Run: `go test ./internal/workflow`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/sync.go internal/workflow/formatter.go internal/workflow/formatter_text.go internal/workflow/formatter_json.go internal/workflow/sync_test.go internal/workflow/sync_adopt_test.go internal/workflow/formatter_test.go
git commit -m "feat: run projection reconcile during sync"
```

### Task 4: Add managed drift repair and safe remove cleanup

**Files:**
- Modify: `cmd/skill.go`
- Modify: `cmd/skill_test.go`
- Modify: `cmd/remove.go`
- Modify: `cmd/remove_test.go`

- [ ] **Step 1: Write failing command tests for `scribe skill repair` and conflict-safe remove**

```go
func TestSkillRepairCanonicalWins(t *testing.T) {}
func TestSkillRepairPromotesToolCopyToCanonical(t *testing.T) {}
func TestRemoveLeavesProjectionConflictResidue(t *testing.T) {}
```

- [ ] **Step 2: Run the command tests to verify they fail**

Run: `go test ./cmd -run 'TestSkillRepairCanonicalWins|TestSkillRepairPromotesToolCopyToCanonical|TestRemoveLeavesProjectionConflictResidue'`
Expected: FAIL because the subcommand and cleanup rules do not exist yet

- [ ] **Step 3: Implement the repair subcommand and remove cleanup updates**

```go
cmd.AddCommand(newSkillEditCommand(), newSkillRepairCommand())
```

```go
for _, p := range installed.ManagedPaths {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("managed path %s: %v", p, err))
	}
}
```

- [ ] **Step 4: Run the command package tests**

Run: `go test ./cmd`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/skill.go cmd/skill_test.go cmd/remove.go cmd/remove_test.go
git commit -m "feat: add managed skill repair flow"
```

### Task 5: Final verification and branch handoff

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-04-11-sync-system-reconcile-design.md` (only if implementation requires clarifying notes)

- [ ] **Step 1: Update docs for the new sync meaning and repair command**

```md
`scribe sync` keeps connected registries, local skills, and installed filesystem-backed tool projections in sync.
```

- [ ] **Step 2: Run focused verification**

Run: `go test ./internal/state ./internal/reconcile ./internal/workflow ./cmd`
Expected: PASS

- [ ] **Step 3: Run full verification**

Run: `go test ./...`
Expected: PASS, or the same pre-existing `internal/logo` failures observed at baseline and no new failures in sync/state/command packages

- [ ] **Step 4: Inspect git diff and prepare PR**

Run: `git status --short && git diff --stat`
Expected: only intended reconcile-related changes

- [ ] **Step 5: Commit**

```bash
git add README.md docs/superpowers/specs/2026-04-11-sync-system-reconcile-design.md docs/superpowers/plans/2026-04-11-sync-system-reconcile.md
git commit -m "docs: describe sync system reconcile flow"
```
