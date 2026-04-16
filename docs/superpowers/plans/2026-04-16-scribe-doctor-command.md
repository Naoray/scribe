# Scribe Doctor Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add `scribe doctor` so Scribe can detect and safely fix canonical `SKILL.md` metadata issues, then repair affected tool projections without leaving state drift behind.

**Architecture:** Split pure `SKILL.md` normalization into a leaf `internal/skillmd` package so both Codex install logic and doctor inspections can share it without import cycles. Keep `internal/doctor` focused on inspection/reporting only, and keep `--fix` orchestration in `cmd/doctor.go` so it can reuse existing command-level repair helpers, update `InstalledHash`, and clear projection conflicts after canonical rewrites.

**Tech Stack:** Go, Cobra, existing `cmd`, `internal/state`, `internal/tools`, and a new `internal/skillmd` plus `internal/doctor` package

---

### Task 1: Add a leaf SKILL.md parser and deterministic normalizer

**Files:**
- Create: `internal/skillmd/normalize.go`
- Create: `internal/skillmd/normalize_test.go`
- Modify: `internal/tools/codex.go`

- [x] **Step 1: Write the failing parser and normalization tests**

```go
func TestNormalizeAddsMissingFrontmatterFromDirectoryName(t *testing.T) {}
func TestNormalizeFillsMissingDescriptionFromFirstParagraph(t *testing.T) {}
func TestNormalizeSkipsHeadingsListsAndCodeFences(t *testing.T) {}
func TestNormalizeRejectsUnrecoverableFrontmatter(t *testing.T) {}
```

- [x] **Step 2: Run the skillmd tests to verify they fail**

Run: `go test ./internal/skillmd -run 'TestNormalize'`
Expected: FAIL because the `internal/skillmd` package does not exist yet

- [x] **Step 3: Implement the leaf parser and normalizer**

```go
type Doc struct {
	Name        string
	Description string
	Body        string
	Changed     bool
}

func Normalize(dirName string, content []byte) (Doc, []byte, error)
func ExtractFallbackDescription(body string) string
```

```go
func ExtractFallbackDescription(body string) string {
	// Skip headings, list items, blockquotes, tables, fenced code blocks.
	// Return the first non-empty prose paragraph with collapsed whitespace.
}
```

- [x] **Step 4: Tighten Codex compatibility checks to use `internal/skillmd`**

```go
func ensureCodexCompatibleSkillMD(skillName, canonicalDir string) error {
	content, err := os.ReadFile(filepath.Join(canonicalDir, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("read codex skill %q: %w", skillName, err)
	}
	_, normalized, err := skillmd.Normalize(skillName, content)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, normalized) {
		return WriteCanonicalSkill(canonicalDir, normalized)
	}
	return nil
}
```

- [x] **Step 5: Run package tests**

Run: `go test ./internal/skillmd ./internal/tools -run 'TestNormalize|TestCodex'`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add internal/skillmd/normalize.go internal/skillmd/normalize_test.go internal/tools/codex.go
git commit -m "feat: normalize skill metadata in a shared package"
```

### Task 2: Add doctor inspection types for managed canonical skills

**Files:**
- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`

- [x] **Step 1: Write failing inspection tests for canonical metadata and projection drift**

```go
func TestInspectSkillReportsMissingDescription(t *testing.T) {}
func TestInspectSkillReportsInvalidFrontmatter(t *testing.T) {}
func TestInspectSkillReportsBrokenProjectionState(t *testing.T) {}
func TestInspectCanLimitToOneSkill(t *testing.T) {}
```

- [x] **Step 2: Run the inspection tests to verify they fail**

Run: `go test ./internal/doctor -run 'TestInspect'`
Expected: FAIL because the inspection types do not exist yet

- [x] **Step 3: Implement doctor inspection types and canonical health checks**

```go
type IssueKind string

const (
	IssueCanonicalMetadata IssueKind = "canonical_metadata"
	IssueProjectionDrift   IssueKind = "projection_drift"
)

type Issue struct {
	Skill   string
	Tool    string
	Kind    IssueKind
	Status  string
	Message string
}

type Report struct {
	Issues []Issue
}
```

```go
func InspectManagedSkills(cfg *config.Config, st *state.State, name string) (Report, error)
```

- [x] **Step 4: Run the doctor package tests**

Run: `go test ./internal/doctor`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/doctor/doctor.go internal/doctor/doctor_test.go
git commit -m "feat: inspect managed skill health"
```

### Task 3: Add `scribe doctor` command wiring with explicit flags

**Files:**
- Create: `cmd/doctor.go`
- Create: `cmd/doctor_test.go`
- Modify: `cmd/root.go`

- [x] **Step 1: Write failing command tests for default, per-skill, and JSON output**

```go
func TestDoctorCommandReportsIssues(t *testing.T) {}
func TestDoctorCommandLimitsToNamedSkill(t *testing.T) {}
func TestDoctorJSONOutput(t *testing.T) {}
func TestDoctorRejectsUnknownFlagsCombination(t *testing.T) {}
```

- [x] **Step 2: Run the command tests to verify they fail**

Run: `go test ./cmd -run 'TestDoctor'`
Expected: FAIL because the `doctor` command does not exist yet

- [x] **Step 3: Implement the Cobra command and root registration**

```go
func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect managed skill health",
		Args:  cobra.NoArgs,
		RunE:  runDoctor,
	}
	cmd.Flags().Bool("fix", false, "Normalize canonical skill metadata and repair affected projections")
	cmd.Flags().String("skill", "", "Inspect a single managed skill")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}
```

```go
cmd.AddCommand(newDoctorCommand())
```

- [x] **Step 4: Run the command package tests**

Run: `go test ./cmd`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add cmd/doctor.go cmd/doctor_test.go cmd/root.go
git commit -m "feat: add scribe doctor command"
```

### Task 4: Implement `doctor --fix` in `cmd` and repair state hygiene

**Files:**
- Modify: `cmd/doctor.go`
- Modify: `cmd/doctor_test.go`
- Modify: `cmd/skill_projection_repair.go`

- [x] **Step 1: Write failing tests for canonical rewrite plus projection repair**

```go
func TestDoctorFixRepairsAffectedProjections(t *testing.T) {}
func TestDoctorFixUpdatesInstalledHash(t *testing.T) {}
func TestDoctorFixClearsProjectionConflicts(t *testing.T) {}
func TestDoctorFixIsIdempotent(t *testing.T) {}
```

- [x] **Step 2: Run the command tests to verify they fail**

Run: `go test ./cmd -run 'TestDoctorFix'`
Expected: FAIL because `doctor --fix` does not rewrite canonical skills or update state yet

- [x] **Step 3: Implement fix mode in `cmd/doctor.go`**

```go
type doctorFixResult struct {
	Name             string   `json:"name"`
	UpdatedCanonical bool     `json:"updated_canonical"`
	RepairedTools    []string `json:"repaired_tools,omitempty"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Inspect managed skills via internal/doctor.
	// For --fix, normalize canonical SKILL.md using internal/skillmd.
	// If canonical content changed:
	//   1. WriteCanonicalSkill(...)
	//   2. refresh InstalledHash
	//   3. call repairSkillProjections(cfg, st, name)
	//   4. clear related projection conflicts
	//   5. save state
}
```

- [x] **Step 4: Extend projection repair helpers only as needed for doctor orchestration**

```go
func repairSkillProjections(cfg *config.Config, st *state.State, name string) (skillProjectionRepairResult, error) {
	// Preserve existing behavior, but make sure doctor can rely on the
	// returned effective tool list and saved state after repair.
}
```

- [x] **Step 5: Run the command test packages**

Run: `go test ./cmd`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add cmd/doctor.go cmd/doctor_test.go cmd/skill_projection_repair.go
git commit -m "feat: repair projections after doctor fixes"
```

### Task 5: Document the command and v1 scope

**Files:**
- Modify: `README.md`
- Modify: `SKILL.md`

- [x] **Step 1: Write a failing doc expectation check by locating the command list and examples to update**

Run: `rg -n "skill repair|doctor|Commands|Daily Use" README.md SKILL.md`
Expected: Output shows `doctor` is not documented yet

- [x] **Step 2: Update README command tables and examples**

```md
| `scribe doctor` | Inspect managed skills and projections for repairable issues |
| `scribe doctor --fix` | Normalize canonical skill metadata and repair affected projections |
| `scribe doctor --skill recap --fix` | Repair a single managed skill and its projections |
```

- [x] **Step 3: Update the agent-facing SKILL.md so setup/help text includes `doctor`**

```md
- `scribe doctor` audits managed skills and projection health.
- `scribe doctor --fix` applies safe metadata normalization and then repairs affected tool projections.
```

- [x] **Step 4: Call out the v1 scope explicitly**

```md
`scribe doctor` v1 does not attempt to rewrite mixed package layouts for Codex.
It focuses on canonical metadata health plus projection repair.
```

- [x] **Step 5: Run targeted doc sanity checks**

Run: `rg -n "scribe doctor" README.md SKILL.md`
Expected: PASS with the new command documented in both files

- [x] **Step 6: Commit**

```bash
git add README.md SKILL.md
git commit -m "docs: add scribe doctor command guidance"
```

### Task 6: Run focused and full verification passes

**Files:**
- Modify: `docs/superpowers/plans/2026-04-16-scribe-doctor-command.md`

- [x] **Step 1: Run the focused doctor-related test pass**

Run: `go test ./internal/skillmd ./internal/doctor ./internal/tools ./cmd -run 'TestDoctor|TestNormalize|TestInspect'`
Expected: PASS

- [x] **Step 2: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [x] **Step 3: Update this plan with any implementation reality changes discovered during execution**

```md
- Implementation ran in an isolated worktree on branch `feat/scribe-doctor-command-anvil` to avoid unrelated checkout noise.
- Adjust file lists or commands only if the implementation uncovered a real naming/path difference.
- Do not leave stale steps in the finished plan.
```

- [x] **Step 4: Commit the final verification sweep**

```bash
git add docs/superpowers/plans/2026-04-16-scribe-doctor-command.md
git commit -m "test: verify scribe doctor implementation"
```

---

## Self-Review

- Spec coverage: the plan now covers deterministic canonical normalization, `scribe doctor` command UX, `--fix` orchestration in `cmd`, `InstalledHash` refresh, projection conflict cleanup, docs, and verification.
- Placeholder scan: no `TODO`/`TBD` placeholders remain; each task names concrete files and commands.
- Type consistency: the plan uses `skillmd.Normalize`, `doctor.InspectManagedSkills`, and `repairSkillProjections` consistently without requiring `internal/*` packages to depend on `cmd/*`.
- Scope check: Codex mixed-package exposure auditing is intentionally deferred from v1 because the plan does not yet define a safe remediation path.
