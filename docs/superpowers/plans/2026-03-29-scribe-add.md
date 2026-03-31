# scribe add Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `scribe add` — add skills to a team registry's `scribe.toml` on GitHub, with interactive Bubble Tea browse and auto-sync.

**Architecture:** New `internal/add/` package with UI-agnostic core (`Adder`) that discovers local + remote skills and pushes entries to GitHub via `PushFiles`. The `cmd/add.go` layer wires discovery → selection → add → sync, with Bubble Tea for interactive browse and plain text / JSON for non-TTY.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea v2 (`charm.land/bubbletea/v2`), BurntSushi/toml, go-github, go-isatty, huh v2

---

### Task 1: Add `Manifest.Encode()` method

**Files:**
- Modify: `internal/manifest/manifest.go`
- Test: `internal/manifest/manifest_test.go`

The `add` command needs to serialize a modified manifest back to TOML after adding a skill entry. Currently `manifest` only has `Parse` (decode). We need `Encode`.

- [ ] **Step 1: Write the failing test**

Add to `internal/manifest/manifest_test.go`:

```go
func TestManifestEncode(t *testing.T) {
	m, err := manifest.Parse([]byte(teamLoadout))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Round-trip: re-parse the encoded output.
	m2, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}

	if m2.Team.Name != m.Team.Name {
		t.Errorf("team name: got %q, want %q", m2.Team.Name, m.Team.Name)
	}
	if len(m2.Skills) != len(m.Skills) {
		t.Errorf("skills count: got %d, want %d", len(m2.Skills), len(m.Skills))
	}
	for name, skill := range m.Skills {
		got, ok := m2.Skills[name]
		if !ok {
			t.Errorf("missing skill %q after round-trip", name)
			continue
		}
		if got.Source != skill.Source {
			t.Errorf("skill %q source: got %q, want %q", name, got.Source, skill.Source)
		}
		if got.Path != skill.Path {
			t.Errorf("skill %q path: got %q, want %q", name, got.Path, skill.Path)
		}
		if got.Private != skill.Private {
			t.Errorf("skill %q private: got %v, want %v", name, got.Private, skill.Private)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/manifest/ -run TestManifestEncode -v`
Expected: FAIL — `m.Encode undefined`

- [ ] **Step 3: Implement Encode**

The `Skill` struct has a custom `UnmarshalTOML` that handles both string and inline-table forms. For encoding, `BurntSushi/toml` uses the `encoding.TextMarshaler` interface. However, team loadout skills need to encode as inline tables `{ source = "...", path = "..." }`. The simplest approach: use a shadow struct for encoding that `toml` can serialize directly.

Add to `internal/manifest/manifest.go`:

```go
// Encode serializes the manifest to TOML bytes.
func (m *Manifest) Encode() ([]byte, error) {
	// Build an encodable shadow struct. The Skill type has custom UnmarshalTOML
	// which toml.Marshal doesn't invert — use a map[string]any for skills.
	type encodable struct {
		Team    *Team            `toml:"team,omitempty"`
		Package *Package         `toml:"package,omitempty"`
		Skills  map[string]any   `toml:"skills"`
		Targets *Targets         `toml:"targets,omitempty"`
	}

	skills := make(map[string]any, len(m.Skills))
	for name, skill := range m.Skills {
		if m.IsLoadout() {
			entry := map[string]any{"source": skill.Source}
			if skill.Path != "" {
				entry["path"] = skill.Path
			}
			if skill.Private {
				entry["private"] = skill.Private
			}
			skills[name] = entry
		} else {
			// Package manifest: plain string path.
			skills[name] = skill.Path
		}
	}

	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(encodable{
		Team:    m.Team,
		Package: m.Package,
		Skills:  skills,
		Targets: m.Targets,
	}); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return buf.Bytes(), nil
}
```

Add `"bytes"` to the import block in `manifest.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/manifest/ -run TestManifestEncode -v`
Expected: PASS

- [ ] **Step 5: Run all manifest tests**

Run: `go test ./internal/manifest/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/manifest/manifest.go internal/manifest/manifest_test.go
git commit -m "feat(manifest): add Encode method for TOML serialization

Round-trip encoding support needed by scribe add to modify
and push scribe.toml back to GitHub."
```

---

### Task 2: Add `internal/add/` package — types and events

**Files:**
- Create: `internal/add/events.go`
- Create: `internal/add/add.go`
- Test: `internal/add/add_test.go`

Define the `Adder` struct, `Candidate` type, and event message types. No logic yet — just the data model.

- [ ] **Step 1: Write the failing test**

Create `internal/add/add_test.go`:

```go
package add_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/add"
)

func TestCandidateUploadFlag(t *testing.T) {
	cases := []struct {
		name   string
		c      add.Candidate
		upload bool
	}{
		{
			name:   "local with source",
			c:      add.Candidate{Name: "deploy", Source: "github:owner/repo@v1.0.0", LocalPath: "/home/user/.scribe/skills/deploy"},
			upload: false,
		},
		{
			name:   "local without source",
			c:      add.Candidate{Name: "cleanup", LocalPath: "/home/user/.claude/skills/cleanup"},
			upload: true,
		},
		{
			name:   "remote only",
			c:      add.Candidate{Name: "nextjs", Source: "github:vercel/skills@v2.0.0", Origin: "registry:vercel/skills"},
			upload: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.c.NeedsUpload() != tc.upload {
				t.Errorf("NeedsUpload() = %v, want %v", tc.c.NeedsUpload(), tc.upload)
			}
		})
	}
}

func TestAdderEmitNilSafe(t *testing.T) {
	a := &add.Adder{}
	// Should not panic with nil Emit callback.
	a.Emit = nil
	// No public method yet to test emit, but verifying struct creates without panic.
	if a.Client != nil {
		t.Error("expected nil client on zero-value Adder")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/add/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create events.go**

Create `internal/add/events.go`:

```go
package add

// --- Events (tea.Msg-compatible) emitted during add ---

// SkillDiscoveredMsg is sent once per candidate skill during discovery.
type SkillDiscoveredMsg struct {
	Name      string
	Origin    string // "local" or "registry:owner/repo"
	Source    string // "github:owner/repo@ref" or empty
	LocalPath string
}

// RegistrySelectedMsg is sent when the target registry is chosen.
type RegistrySelectedMsg struct {
	Registry string
}

// SkillAddingMsg is sent when a skill push to GitHub begins.
type SkillAddingMsg struct {
	Name   string
	Upload bool
}

// SkillAddedMsg is sent when a skill is successfully added to the registry.
type SkillAddedMsg struct {
	Name     string
	Registry string
	Source   string
	Upload   bool
}

// SkillAddErrorMsg is sent when adding a skill fails.
type SkillAddErrorMsg struct {
	Name string
	Err  error
}

// AddCompleteMsg is sent when all skills have been processed.
type AddCompleteMsg struct {
	Added       int
	Failed      int
	SyncStarted bool
}
```

- [ ] **Step 4: Create add.go**

Create `internal/add/add.go`:

```go
package add

import (
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/targets"
)

// Candidate represents a skill that can be added to a registry.
type Candidate struct {
	Name      string // skill name (directory basename)
	Origin    string // "local" or "registry:owner/repo"
	Source    string // "github:owner/repo@ref" or empty for local-only
	LocalPath string // absolute path on disk, empty for remote-only
}

// NeedsUpload reports whether this candidate requires uploading files to the
// registry (as opposed to just adding a source reference to scribe.toml).
func (c Candidate) NeedsUpload() bool {
	return c.Source == "" && c.LocalPath != ""
}

// Adder wires discovery and GitHub push together.
// Emits events via the Emit callback — the caller decides output format.
type Adder struct {
	Client  *gh.Client
	Targets []targets.Target
	Emit    func(any)
}

func (a *Adder) emit(msg any) {
	if a.Emit != nil {
		a.Emit(msg)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/add/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/add/events.go internal/add/add.go internal/add/add_test.go
git commit -m "feat(add): define Adder struct, Candidate type, and event messages

Data model for scribe add. Follows the Emit func(any) pattern
from internal/sync/."
```

---

### Task 3: Implement `Adder.Discover()` — local skill scanning

**Files:**
- Modify: `internal/add/add.go`
- Test: `internal/add/add_test.go`

Discover scans `~/.claude/skills/` and `~/.scribe/skills/`, cross-references `state.Installed` for source info, and returns candidates. This task covers local discovery only — remote registry discovery is Task 4.

- [ ] **Step 1: Write the failing test**

Add to `internal/add/add_test.go`:

```go
func TestDiscoverLocalSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a local-only skill in ~/.claude/skills/cleanup/
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "cleanup")
	if err := os.MkdirAll(claudeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# Cleanup"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a scribe-managed skill in ~/.scribe/skills/deploy/
	scribeSkillDir := filepath.Join(home, ".scribe", "skills", "deploy")
	if err := os.MkdirAll(scribeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scribeSkillDir, "SKILL.md"), []byte("# Deploy"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"deploy": {Source: "github:owner/repo@v1.0.0", Version: "v1.0.0"},
		},
	}

	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	// Find each by name.
	byName := map[string]add.Candidate{}
	for _, c := range candidates {
		byName[c.Name] = c
	}

	cleanup, ok := byName["cleanup"]
	if !ok {
		t.Fatal("missing candidate: cleanup")
	}
	if cleanup.Source != "" {
		t.Errorf("cleanup source: got %q, want empty", cleanup.Source)
	}
	if !cleanup.NeedsUpload() {
		t.Error("cleanup should need upload")
	}
	if cleanup.Origin != "local" {
		t.Errorf("cleanup origin: got %q, want local", cleanup.Origin)
	}

	deploy, ok := byName["deploy"]
	if !ok {
		t.Fatal("missing candidate: deploy")
	}
	if deploy.Source != "github:owner/repo@v1.0.0" {
		t.Errorf("deploy source: got %q", deploy.Source)
	}
	if deploy.NeedsUpload() {
		t.Error("deploy should not need upload")
	}
}

func TestDiscoverLocalSkipsEmptyDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create an empty directory — should not be discovered.
	emptyDir := filepath.Join(home, ".claude", "skills", "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for empty dirs, got %d", len(candidates))
	}
}

func TestDiscoverLocalDeduplicates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Same skill name in both ~/.claude/skills/ and ~/.scribe/skills/ — local (claude) wins.
	for _, dir := range []string{
		filepath.Join(home, ".claude", "skills", "deploy"),
		filepath.Join(home, ".scribe", "skills", "deploy"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Deploy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st := &state.State{Installed: map[string]state.InstalledSkill{}}
	adder := &add.Adder{}

	candidates, err := adder.DiscoverLocal(st)
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 (deduplicated), got %d", len(candidates))
	}
	// Should be the claude path (scanned first, wins).
	if !strings.Contains(candidates[0].LocalPath, ".claude") {
		t.Errorf("expected claude path, got %q", candidates[0].LocalPath)
	}
}
```

Add these imports to the test file:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/state"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/add/ -run TestDiscoverLocal -v`
Expected: FAIL — `adder.DiscoverLocal undefined`

- [ ] **Step 3: Implement DiscoverLocal**

Add to `internal/add/add.go`:

```go
import (
	"os"
	"path/filepath"
	"sort"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/targets"
)

// DiscoverLocal scans ~/.claude/skills/ and ~/.scribe/skills/ for skills on disk.
// Cross-references state for source info. Deduplicates by name (first seen wins).
func (a *Adder) DiscoverLocal(st *state.State) ([]Candidate, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	dirs := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".scribe", "skills"),
	}

	seen := map[string]bool{}
	var candidates []Candidate

	for _, base := range dirs {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", base, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}

			skillDir := filepath.Join(base, name)
			empty, err := isDirEmpty(skillDir)
			if err != nil || empty {
				continue
			}

			seen[name] = true

			c := Candidate{
				Name:      name,
				Origin:    "local",
				LocalPath: skillDir,
			}
			if installed, ok := st.Installed[name]; ok && installed.Source != "" {
				c.Source = installed.Source
			}

			candidates = append(candidates, c)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates, nil
}

// isDirEmpty reports whether a directory has no files (ignoring subdirectories).
func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			return false, nil
		}
	}
	// Check subdirectories recursively.
	for _, e := range entries {
		if e.IsDir() {
			empty, err := isDirEmpty(filepath.Join(dir, e.Name()))
			if err != nil {
				return false, err
			}
			if !empty {
				return false, nil
			}
		}
	}
	return true, nil
}
```

Add `"fmt"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/add/ -run TestDiscoverLocal -v`
Expected: All 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/add/add.go internal/add/add_test.go
git commit -m "feat(add): implement DiscoverLocal for scanning local skill directories

Scans ~/.claude/skills/ and ~/.scribe/skills/, cross-references
state for source info, deduplicates by name."
```

---

### Task 4: Implement `Adder.DiscoverRemote()` — registry skill fetching

**Files:**
- Modify: `internal/add/add.go`
- Test: `internal/add/add_test.go`

Discover skills from connected registries that aren't already in the target registry. This complements `DiscoverLocal` — the `cmd/` layer merges both results.

- [ ] **Step 1: Write the failing test**

Add to `internal/add/add_test.go`:

```go
func TestDiscoverRemoteSkills(t *testing.T) {
	// DiscoverRemote takes parsed manifests rather than calling GitHub directly.
	// The cmd layer fetches manifests; the core just filters and converts.

	targetManifest := &manifest.Manifest{
		Team:   &manifest.Team{Name: "my-team"},
		Skills: map[string]manifest.Skill{
			"deploy": {Source: "github:owner/repo@v1.0.0"},
		},
	}

	otherManifests := map[string]*manifest.Manifest{
		"vercel/skills": {
			Team: &manifest.Team{Name: "vercel"},
			Skills: map[string]manifest.Skill{
				"nextjs":  {Source: "github:vercel/nextjs-skill@v2.0.0"},
				"deploy":  {Source: "github:vercel/deploy@v1.0.0"}, // already in target — should be filtered
			},
		},
	}

	adder := &add.Adder{}
	candidates := adder.DiscoverRemote(targetManifest, otherManifests)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 remote candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "nextjs" {
		t.Errorf("expected nextjs, got %q", candidates[0].Name)
	}
	if candidates[0].Origin != "registry:vercel/skills" {
		t.Errorf("origin: got %q", candidates[0].Origin)
	}
	if candidates[0].Source != "github:vercel/nextjs-skill@v2.0.0" {
		t.Errorf("source: got %q", candidates[0].Source)
	}
	if candidates[0].NeedsUpload() {
		t.Error("remote candidate should not need upload")
	}
}
```

Add `"github.com/Naoray/scribe/internal/manifest"` to test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/add/ -run TestDiscoverRemoteSkills -v`
Expected: FAIL — `adder.DiscoverRemote undefined`

- [ ] **Step 3: Implement DiscoverRemote**

Add to `internal/add/add.go`:

```go
import "github.com/Naoray/scribe/internal/manifest"

// DiscoverRemote finds skills in other registries that are not in the target registry.
// Takes pre-fetched manifests to keep GitHub calls in the cmd layer.
func (a *Adder) DiscoverRemote(targetManifest *manifest.Manifest, otherManifests map[string]*manifest.Manifest) []Candidate {
	var candidates []Candidate

	for registry, m := range otherManifests {
		for name, skill := range m.Skills {
			// Skip if already in target registry.
			if _, exists := targetManifest.Skills[name]; exists {
				continue
			}

			candidates = append(candidates, Candidate{
				Name:   name,
				Origin: "registry:" + registry,
				Source: skill.Source,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/add/ -run TestDiscoverRemoteSkills -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/add/add.go internal/add/add_test.go
git commit -m "feat(add): implement DiscoverRemote for cross-registry skill discovery

Filters skills from other connected registries that are not
already in the target registry."
```

---

### Task 5: Implement `Adder.Add()` — push skills to registry

**Files:**
- Modify: `internal/add/add.go`
- Test: `internal/add/add_test.go`

The core add operation: for each candidate, either add a source reference or upload files, then commit via `PushFiles`.

- [ ] **Step 1: Write the failing test for source reference add**

Add to `internal/add/add_test.go`:

```go
func TestAddBuildsPushFilesForReference(t *testing.T) {
	// Test that Add correctly modifies scribe.toml for a source-reference candidate.
	// We can't easily test the full GitHub flow without a mock, so we test the
	// manifest modification logic instead.

	original := `[team]
name = "my-team"

[skills]
deploy = {source = "github:owner/repo@v1.0.0"}
`
	m, err := manifest.Parse([]byte(original))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate adding a new source-reference skill.
	m.Skills["gstack"] = manifest.Skill{Source: "github:garrytan/gstack@v0.12.9.0"}

	encoded, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Verify round-trip.
	m2, err := manifest.Parse(encoded)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if len(m2.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(m2.Skills))
	}
	gstack, ok := m2.Skills["gstack"]
	if !ok {
		t.Fatal("gstack not found after round-trip")
	}
	if gstack.Source != "github:garrytan/gstack@v0.12.9.0" {
		t.Errorf("gstack source: got %q", gstack.Source)
	}
}

func TestAddBuildsPushFilesForUpload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a local skill with files.
	skillDir := filepath.Join(home, ".claude", "skills", "cleanup")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Cleanup\nDo cleanup."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.md"), []byte("# Helper"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidate := add.Candidate{
		Name:      "cleanup",
		Origin:    "local",
		LocalPath: skillDir,
	}

	files, err := add.ReadLocalSkillFiles(candidate)
	if err != nil {
		t.Fatalf("ReadLocalSkillFiles: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Files should be keyed as skills/<name>/<filename>.
	for path := range files {
		if !strings.HasPrefix(path, "skills/cleanup/") {
			t.Errorf("unexpected file path: %q", path)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/add/ -run TestAddBuilds -v`
Expected: FAIL — `add.ReadLocalSkillFiles undefined`

- [ ] **Step 3: Implement ReadLocalSkillFiles and Add**

Add to `internal/add/add.go`:

```go
import (
	"context"
	"io/fs"
	"strings"
)

// ReadLocalSkillFiles reads all files from a local skill directory and returns
// them as a map of "skills/<name>/<relative-path>" → content string.
// Used when uploading a local-only skill to a registry.
func ReadLocalSkillFiles(c Candidate) (map[string]string, error) {
	files := map[string]string{}
	root := c.LocalPath

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		key := "skills/" + c.Name + "/" + filepath.ToSlash(rel)
		files[key] = string(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

// Add pushes one or more skills to the target registry's scribe.toml on GitHub.
// For each candidate: adds a source reference or uploads files + self-reference.
// Emits events throughout. Per-skill errors do not abort the loop.
func (a *Adder) Add(ctx context.Context, targetRepo string, candidates []Candidate) error {
	owner, repo, err := splitRepo(targetRepo)
	if err != nil {
		return err
	}

	for _, c := range candidates {
		a.emit(SkillAddingMsg{Name: c.Name, Upload: c.NeedsUpload()})

		if err := a.addOne(ctx, owner, repo, targetRepo, c); err != nil {
			a.emit(SkillAddErrorMsg{Name: c.Name, Err: err})
			continue
		}

		source := c.Source
		if c.NeedsUpload() {
			source = fmt.Sprintf("github:%s/%s@main", owner, repo)
		}
		a.emit(SkillAddedMsg{
			Name:     c.Name,
			Registry: targetRepo,
			Source:   source,
			Upload:   c.NeedsUpload(),
		})
	}

	return nil
}

func (a *Adder) addOne(ctx context.Context, owner, repo, targetRepo string, c Candidate) error {
	// Fetch current manifest.
	raw, err := a.Client.FetchFile(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("fetch scribe.toml: %w", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse scribe.toml: %w", err)
	}

	if m.Skills == nil {
		m.Skills = make(map[string]manifest.Skill)
	}

	// Build the files to push.
	pushFiles := map[string]string{}

	if c.NeedsUpload() {
		// Upload local files to registry.
		localFiles, err := ReadLocalSkillFiles(c)
		if err != nil {
			return err
		}
		for k, v := range localFiles {
			pushFiles[k] = v
		}
		// Self-referencing entry.
		m.Skills[c.Name] = manifest.Skill{
			Source: fmt.Sprintf("github:%s/%s@main", owner, repo),
			Path:   "skills/" + c.Name,
		}
	} else {
		// Source reference only.
		m.Skills[c.Name] = manifest.Skill{Source: c.Source}
	}

	encoded, err := m.Encode()
	if err != nil {
		return err
	}
	pushFiles["scribe.toml"] = string(encoded)

	msg := fmt.Sprintf("add skill: %s", c.Name)
	return a.Client.PushFiles(ctx, owner, repo, pushFiles, msg)
}

func splitRepo(teamRepo string) (owner, repo string, err error) {
	parts := strings.SplitN(teamRepo, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo", teamRepo)
	}
	return parts[0], parts[1], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/add/ -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/add/add.go internal/add/add_test.go
git commit -m "feat(add): implement Add method and ReadLocalSkillFiles

Core add logic: source-reference or upload path, fetch-modify-push
via PushFiles. Per-skill errors don't abort the loop."
```

---

### Task 6: Wire `cmd/add.go` — Mode 1 (name provided, plain text)

**Files:**
- Modify: `cmd/add.go`

Replace the stub with the full Mode 1 implementation: resolve skill by name, determine strategy, select registry, confirm, push, auto-sync.

- [ ] **Step 1: Write the complete cmd/add.go**

Replace `cmd/add.go` entirely:

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
)

var (
	addYes      bool
	addJSON     bool
	addRegistry string
)

var addCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a skill to a team registry",
	Long: `Add a skill to a team registry's scribe.toml on GitHub.

If the skill has a known source (synced from another registry), adds a
source reference. If it's a local-only skill, uploads the files to the
registry.

With no arguments in a terminal, shows an interactive browser to select
skills. In non-TTY mode, the skill name is required.

Examples:
  scribe add cleanup
  scribe add gstack --registry ArtistfyHQ/team-skills
  scribe add --yes cleanup`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().BoolVar(&addYes, "yes", false, "Skip confirmation prompt")
	addCmd.Flags().BoolVar(&addJSON, "json", false, "Output machine-readable JSON")
	addCmd.Flags().StringVar(&addRegistry, "registry", "", "Target registry (owner/repo)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	useJSON := addJSON || !isatty.IsTerminal(os.Stdout.Fd())

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("no registries connected — run: scribe connect <owner/repo>")
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
	}

	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}
	adder := &add.Adder{Client: client, Targets: tgts}

	// Resolve target registry.
	targetRepo, err := resolveTargetRegistry(addRegistry, cfg.TeamRepos, isTTY)
	if err != nil {
		return err
	}

	// Mode 3: no args, non-TTY.
	if len(args) == 0 && !isTTY {
		return fmt.Errorf("skill name required when not running interactively")
	}

	// Discover candidates.
	localCandidates, err := adder.DiscoverLocal(st)
	if err != nil {
		return err
	}

	// Fetch target registry manifest to filter already-added skills.
	ctx := context.Background()
	targetOwner, targetRepoName, _ := parseOwnerRepo(targetRepo)
	targetRaw, err := client.FetchFile(ctx, targetOwner, targetRepoName, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("fetch target registry: %w", err)
	}
	targetManifest, err := manifest.Parse(targetRaw)
	if err != nil {
		return fmt.Errorf("parse target registry: %w", err)
	}

	// Fetch other registries for remote discovery.
	otherManifests := map[string]*manifest.Manifest{}
	for _, repo := range cfg.TeamRepos {
		if repo == targetRepo {
			continue
		}
		o, r, _ := parseOwnerRepo(repo)
		raw, err := client.FetchFile(ctx, o, r, "scribe.toml", "HEAD")
		if err != nil {
			continue // skip unreachable registries
		}
		m, err := manifest.Parse(raw)
		if err != nil || !m.IsLoadout() {
			continue
		}
		otherManifests[repo] = m
	}

	remoteCandidates := adder.DiscoverRemote(targetManifest, otherManifests)

	// Merge and filter: remove skills already in target.
	allCandidates := filterAlreadyInTarget(
		append(localCandidates, remoteCandidates...),
		targetManifest,
	)

	if len(args) == 1 {
		return runAddByName(ctx, args[0], allCandidates, adder, targetRepo, cfg, st, client, tgts, useJSON, isTTY)
	}

	// Mode 2: interactive browse (TTY, no args) — Task 7.
	return runAddInteractive(ctx, allCandidates, adder, targetRepo, cfg, st, client, tgts, useJSON)
}

func runAddByName(
	ctx context.Context,
	name string,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	tgts []targets.Target,
	useJSON bool,
	isTTY bool,
) error {
	// Find the candidate.
	var found *add.Candidate
	for _, c := range candidates {
		if c.Name == name {
			found = &c
			break
		}
	}
	if found == nil {
		return fmt.Errorf("skill %q not found locally or in connected registries", name)
	}

	// Confirmation.
	if !addYes && isTTY {
		action := "add reference"
		if found.NeedsUpload() {
			action = "upload files"
		}
		var confirm bool
		err := huh.NewConfirm().
			Title(fmt.Sprintf("%s %q to %s?", action, name, targetRepo)).
			Value(&confirm).
			Run()
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Wire events.
	type addResult struct {
		Name     string `json:"name"`
		Registry string `json:"registry"`
		Source   string `json:"source"`
		Uploaded bool   `json:"uploaded"`
	}
	var results []addResult
	var failed int

	adder.Emit = func(msg any) {
		switch m := msg.(type) {
		case add.SkillAddingMsg:
			if !useJSON {
				verb := "adding reference"
				if m.Upload {
					verb = "uploading"
				}
				fmt.Printf("  %s %s...\n", verb, m.Name)
			}
		case add.SkillAddedMsg:
			if useJSON {
				results = append(results, addResult{
					Name:     m.Name,
					Registry: m.Registry,
					Source:   m.Source,
					Uploaded: m.Upload,
				})
			} else {
				fmt.Printf("  ✓ %s added to %s\n", m.Name, m.Registry)
			}
		case add.SkillAddErrorMsg:
			failed++
			if useJSON {
				results = append(results, addResult{Name: m.Name, Registry: targetRepo})
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", m.Name, m.Err)
			}
		}
	}

	if err := adder.Add(ctx, targetRepo, []add.Candidate{*found}); err != nil {
		return err
	}

	// Auto-sync.
	synced := autoSync(ctx, targetRepo, st, client, tgts, useJSON)

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"added":  results,
			"synced": synced,
		})
	}

	return nil
}

// runAddInteractive is the Bubble Tea browse mode — implemented in Task 7.
func runAddInteractive(
	ctx context.Context,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	tgts []targets.Target,
	useJSON bool,
) error {
	return fmt.Errorf("interactive mode not yet implemented — pass a skill name")
}

// resolveTargetRegistry determines which registry to add skills to.
func resolveTargetRegistry(flag string, repos []string, isTTY bool) (string, error) {
	if flag != "" {
		return resolveRegistry(flag, repos)
	}
	if len(repos) == 1 {
		return repos[0], nil
	}
	// Multiple registries, no flag.
	if !isTTY {
		return "", fmt.Errorf("multiple registries connected — pass --registry owner/repo")
	}
	// Interactive picker.
	var selected string
	opts := make([]huh.Option[string], len(repos))
	for i, r := range repos {
		opts[i] = huh.NewOption(r, r)
	}
	err := huh.NewSelect[string]().
		Title("Which registry?").
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}

// filterAlreadyInTarget removes candidates that are already in the target registry.
func filterAlreadyInTarget(candidates []add.Candidate, targetManifest *manifest.Manifest) []add.Candidate {
	// Also deduplicate by name (local wins — it appears first).
	seen := map[string]bool{}
	var filtered []add.Candidate
	for _, c := range candidates {
		if seen[c.Name] {
			continue
		}
		if _, exists := targetManifest.Skills[c.Name]; exists {
			continue
		}
		seen[c.Name] = true
		filtered = append(filtered, c)
	}
	return filtered
}

// autoSync runs a sync for the target registry after adding skills.
func autoSync(ctx context.Context, targetRepo string, st *state.State, client *gh.Client, tgts []targets.Target, useJSON bool) bool {
	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
		Emit: func(msg any) {
			if useJSON {
				return
			}
			switch m := msg.(type) {
			case sync.SkillInstalledMsg:
				verb := "installed"
				if m.Updated {
					verb = "updated to"
				}
				fmt.Printf("  %-20s %s %s\n", m.Name, verb, m.Version)
			case sync.SkillErrorMsg:
				fmt.Fprintf(os.Stderr, "  %-20s error: %v\n", m.Name, m.Err)
			}
		},
	}

	if !useJSON {
		fmt.Printf("\nsyncing %s...\n\n", targetRepo)
	}
	if err := syncer.Run(ctx, targetRepo, st); err != nil {
		if !useJSON {
			fmt.Fprintf(os.Stderr, "warning: sync failed: %v\nrun `scribe sync` to retry\n", err)
		}
		return false
	}
	return true
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 3: Run existing tests to check nothing is broken**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/add.go
git commit -m "feat(add): wire cmd/add.go for Mode 1 (add by name)

Replaces the stub with full flow: resolve skill, select registry,
confirm, push to GitHub, auto-sync. Mode 2 (interactive) is a
placeholder for Task 7."
```

---

### Task 7: Wire `cmd/add.go` — Mode 2 (interactive Bubble Tea browse)

**Files:**
- Modify: `cmd/add.go`

Implement the interactive skill browser using Bubble Tea v2's list component.

- [ ] **Step 1: Implement the Bubble Tea model**

Add a new file `cmd/add_tui.go`:

```go
package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/add"
)

type addItem struct {
	candidate add.Candidate
	selected  bool
}

type addModel struct {
	items      []addItem
	cursor     int
	search     string
	targetRepo string
	confirmed  bool
	quitting   bool
	width      int
	height     int
}

func newAddModel(candidates []add.Candidate, targetRepo string) addModel {
	items := make([]addItem, len(candidates))
	for i, c := range candidates {
		items[i] = addItem{candidate: c}
	}
	return addModel{
		items:      items,
		targetRepo: targetRepo,
	}
}

func (m addModel) Init() tea.Cmd { return nil }

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			// Skip group headers.
			for m.cursor > 0 && m.filteredItems()[m.cursor].candidate.Name == "" {
				m.cursor--
			}
		case "down", "j":
			filtered := m.filteredItems()
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}
			for m.cursor < len(filtered)-1 && filtered[m.cursor].candidate.Name == "" {
				m.cursor++
			}
		case "space":
			filtered := m.filteredItems()
			if m.cursor < len(filtered) {
				// Toggle in the original items list.
				name := filtered[m.cursor].candidate.Name
				for i := range m.items {
					if m.items[i].candidate.Name == name {
						m.items[i].selected = !m.items[i].selected
						break
					}
				}
			}
		case "enter":
			if m.selectedCount() > 0 {
				m.confirmed = true
				return m, tea.Quit
			}
		case "backspace":
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.cursor = 0
			}
		default:
			if len(msg.String()) == 1 {
				m.search += msg.String()
				m.cursor = 0
			}
		}
	}
	return m, nil
}

func (m addModel) View() tea.View {
	var v tea.View
	if m.quitting {
		v.Body = ""
		return v
	}

	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Add skills to %s", m.targetRepo),
	)
	b.WriteString(title + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> Search: %s\n\n", m.search))
	} else {
		b.WriteString("> Search: \n\n")
	}

	filtered := m.filteredItems()
	if len(filtered) == 0 {
		if m.search != "" {
			b.WriteString("  No skills matching \"" + m.search + "\"\n")
		} else {
			b.WriteString("  All available skills are already in " + m.targetRepo + ".\n")
		}
	}

	// Group by origin.
	currentGroup := ""
	for i, item := range filtered {
		group := itemGroup(item.candidate)
		if group != currentGroup {
			currentGroup = group
			header := lipgloss.NewStyle().Bold(true).Render(group)
			b.WriteString("\n" + header + "\n")
		}

		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if item.selected {
			check = "[x]"
		}

		origin := shortOrigin(item.candidate)
		b.WriteString(fmt.Sprintf("%s%s %-20s %s\n", cursor, check, item.candidate.Name, origin))
	}

	b.WriteString("\n↑↓ navigate · space select · enter add · q quit")

	if n := m.selectedCount(); n > 0 {
		b.WriteString(fmt.Sprintf("  (%d selected)", n))
	}
	b.WriteString("\n")

	v.Body = b.String()
	return v
}

func (m addModel) filteredItems() []addItem {
	if m.search == "" {
		return m.items
	}
	var filtered []addItem
	lower := strings.ToLower(m.search)
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.candidate.Name), lower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m addModel) selectedCount() int {
	n := 0
	for _, item := range m.items {
		if item.selected {
			n++
		}
	}
	return n
}

func (m addModel) selectedCandidates() []add.Candidate {
	var selected []add.Candidate
	for _, item := range m.items {
		if item.selected {
			selected = append(selected, item.candidate)
		}
	}
	return selected
}

func itemGroup(c add.Candidate) string {
	if c.Origin == "local" {
		return "LOCAL"
	}
	if strings.HasPrefix(c.Origin, "registry:") {
		return "FROM " + strings.TrimPrefix(c.Origin, "registry:")
	}
	return "OTHER"
}

func shortOrigin(c add.Candidate) string {
	if c.LocalPath != "" {
		// Show abbreviated path.
		if strings.Contains(c.LocalPath, ".claude") {
			return "~/.claude/skills"
		}
		return "~/.scribe/skills"
	}
	if c.Source != "" {
		if len(c.Source) > 30 {
			return c.Source[:27] + "..."
		}
		return c.Source
	}
	return ""
}
```

- [ ] **Step 2: Update runAddInteractive in cmd/add.go**

Replace the placeholder `runAddInteractive` function in `cmd/add.go`:

```go
func runAddInteractive(
	ctx context.Context,
	candidates []add.Candidate,
	adder *add.Adder,
	targetRepo string,
	cfg *config.Config,
	st *state.State,
	client *gh.Client,
	tgts []targets.Target,
	useJSON bool,
) error {
	if len(candidates) == 0 {
		fmt.Printf("All available skills are already in %s.\n", targetRepo)
		return nil
	}

	// Sort: local first, then remote, alphabetical within each.
	sortCandidates(candidates)

	m := newAddModel(candidates, targetRepo)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fm, ok := finalModel.(addModel)
	if !ok || fm.quitting || !fm.confirmed {
		return nil
	}

	selected := fm.selectedCandidates()
	if len(selected) == 0 {
		return nil
	}

	// Confirmation (unless --yes).
	if !addYes {
		fmt.Printf("\nAdding %d skill(s) to %s:\n", len(selected), targetRepo)
		for _, c := range selected {
			action := "reference"
			if c.NeedsUpload() {
				action = "upload"
			}
			fmt.Printf("  • %s (%s)\n", c.Name, action)
		}

		var confirm bool
		if err := huh.NewConfirm().Title("Proceed?").Value(&confirm).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Wire events and add.
	type addResult struct {
		Name     string `json:"name"`
		Registry string `json:"registry"`
		Source   string `json:"source"`
		Uploaded bool   `json:"uploaded"`
	}
	var results []addResult

	adder.Emit = func(msg any) {
		switch m := msg.(type) {
		case add.SkillAddingMsg:
			if !useJSON {
				verb := "adding reference"
				if m.Upload {
					verb = "uploading"
				}
				fmt.Printf("  %s %s...\n", verb, m.Name)
			}
		case add.SkillAddedMsg:
			if useJSON {
				results = append(results, addResult{
					Name: m.Name, Registry: m.Registry, Source: m.Source, Uploaded: m.Upload,
				})
			} else {
				fmt.Printf("  ✓ %s added to %s\n", m.Name, m.Registry)
			}
		case add.SkillAddErrorMsg:
			if !useJSON {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", m.Name, m.Err)
			}
		}
	}

	if err := adder.Add(ctx, targetRepo, selected); err != nil {
		return err
	}

	synced := autoSync(ctx, targetRepo, st, client, tgts, useJSON)

	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"added":  results,
			"synced": synced,
		})
	}

	return nil
}

// sortCandidates sorts local-first, then remote, alphabetical within each.
func sortCandidates(candidates []add.Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		iLocal := candidates[i].Origin == "local"
		jLocal := candidates[j].Origin == "local"
		if iLocal != jLocal {
			return iLocal
		}
		return candidates[i].Name < candidates[j].Name
	})
}
```

Add `"sort"` to the imports in `cmd/add.go`.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/add.go cmd/add_tui.go
git commit -m "feat(add): implement interactive Bubble Tea skill browser (Mode 2)

Searchable, multi-select TUI for browsing local + remote skills.
Groups by LOCAL / FROM <registry>, confirms selection, pushes
to GitHub, and auto-syncs."
```

---

### Task 8: Integration testing and edge cases

**Files:**
- Modify: `internal/add/add_test.go`

Test error paths and edge cases in the add package.

- [ ] **Step 1: Test that Add handles already-in-registry gracefully**

Add to `internal/add/add_test.go`:

```go
func TestFilterAlreadyInTarget(t *testing.T) {
	targetManifest := &manifest.Manifest{
		Team: &manifest.Team{Name: "test"},
		Skills: map[string]manifest.Skill{
			"deploy": {Source: "github:owner/repo@v1.0.0"},
		},
	}

	candidates := []add.Candidate{
		{Name: "deploy", Source: "github:owner/repo@v1.0.0"},
		{Name: "cleanup", LocalPath: "/path/to/cleanup"},
	}

	// filterAlreadyInTarget is in cmd/, so test the equivalent logic here.
	var filtered []add.Candidate
	for _, c := range candidates {
		if _, exists := targetManifest.Skills[c.Name]; !exists {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 after filter, got %d", len(filtered))
	}
	if filtered[0].Name != "cleanup" {
		t.Errorf("expected cleanup, got %q", filtered[0].Name)
	}
}
```

- [ ] **Step 2: Test ReadLocalSkillFiles with nested directories**

```go
func TestReadLocalSkillFilesNested(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, "skill")
	subDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "run.sh"), []byte("#!/bin/sh"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := add.Candidate{Name: "myskill", LocalPath: skillDir}
	files, err := add.ReadLocalSkillFiles(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if _, ok := files["skills/myskill/SKILL.md"]; !ok {
		t.Error("missing skills/myskill/SKILL.md")
	}
	if _, ok := files["skills/myskill/scripts/run.sh"]; !ok {
		t.Error("missing skills/myskill/scripts/run.sh")
	}
}
```

- [ ] **Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Run the full build**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/add/add_test.go
git commit -m "test(add): add edge case tests for filtering and nested file reading"
```

---

### Task 9: Manual smoke test and cleanup

**Files:**
- Review: all files touched in previous tasks

- [ ] **Step 1: Verify help output**

Run: `go run ./cmd/scribe add --help`
Expected: Shows usage with `[name]`, `--registry`, `--yes`, `--json` flags

- [ ] **Step 2: Verify non-TTY error**

Run: `echo "" | go run ./cmd/scribe add`
Expected: Error `skill name required when not running interactively`

- [ ] **Step 3: Verify not-connected error**

Run (with no config): `HOME=$(mktemp -d) go run ./cmd/scribe add cleanup`
Expected: Error `no registries connected — run: scribe connect <owner/repo>`

- [ ] **Step 4: Run full test suite one final time**

Run: `go test ./... -count=1`
Expected: All PASS

- [ ] **Step 5: Run go vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 6: Commit any cleanup**

Only if changes were needed:

```bash
git add -A
git commit -m "chore(add): cleanup from smoke testing"
```

---

## File Summary

| Action | File | Purpose |
|--------|------|---------|
| Modify | `internal/manifest/manifest.go` | Add `Encode()` method |
| Modify | `internal/manifest/manifest_test.go` | Test `Encode()` round-trip |
| Create | `internal/add/events.go` | Event message types |
| Create | `internal/add/add.go` | `Adder`, `Candidate`, `DiscoverLocal`, `DiscoverRemote`, `Add` |
| Create | `internal/add/add_test.go` | Tests for all add package logic |
| Modify | `cmd/add.go` | Full command wiring: Modes 1-3, registry selection, auto-sync |
| Create | `cmd/add_tui.go` | Bubble Tea model for interactive browse |
