# List Detail View & Version Infrastructure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Evolve `scribe list --local` into a split-pane TUI with reactive detail preview, action menu, and content-hash version infrastructure.

**Architecture:** Four layers of change: (1) content hash computation in a new `internal/discovery/hash.go`, (2) metadata parsing upgrade in `internal/discovery/discovery.go`, (3) split-pane rendering + action menu in `cmd/list_tui.go`, (4) JSON output update in `internal/workflow/list.go`. Each layer builds on the previous.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea v2 (`charm.land/bubbletea/v2`), Lip Gloss v2 (`charm.land/lipgloss/v2`), `crypto/sha256`, `github.com/mattn/go-runewidth`

**Worktree:** `/Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view`

**Spec:** `docs/superpowers/specs/2026-04-03-list-detail-view-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/discovery/hash.go` | Create | Content hash computation — symlink-following, exclusions, CRLF normalization |
| `internal/discovery/hash_test.go` | Create | Tests for hash computation edge cases |
| `internal/discovery/discovery.go` | Modify | Rename `readSkillDescription` → `readSkillMetadata`, parse `version:` + `name:`, drop `Managed` field, add `ContentHash`, wire version resolution |
| `internal/discovery/discovery_test.go` | Create | Tests for metadata parsing and version resolution |
| `cmd/list_tui.go` | Modify | Split-pane layout, action menu phase, confirm substate, narrow fallback, InterruptMsg handling |
| `cmd/list.go` | Modify | Pass `cmd.Context()` to `tea.NewProgram` |
| `internal/workflow/list.go` | Modify | Update `printLocalJSON` to emit `content_hash`, `version` (resolved), derive `managed` from state |

---

### Task 1: Content Hash Computation

**Files:**
- Create: `internal/discovery/hash.go`
- Create: `internal/discovery/hash_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/discovery/hash_test.go`:

```go
package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContentHash(t *testing.T) {
	// Create a temp skill directory with known content.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---\n# Test"), 0o644)
	os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash\necho hello"), 0o644)

	hash, err := contentHash(dir)
	if err != nil {
		t.Fatalf("contentHash: %v", err)
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q (len %d)", hash, len(hash))
	}

	// Same content → same hash.
	hash2, err := contentHash(dir)
	if err != nil {
		t.Fatalf("contentHash second call: %v", err)
	}
	if hash != hash2 {
		t.Errorf("determinism: got %q then %q", hash, hash2)
	}
}

func TestContentHash_Excludes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)

	hashBefore, _ := contentHash(dir)

	// Add excluded files — hash should not change.
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("binary"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("module.exports = {}"), 0o644)

	hashAfter, _ := contentHash(dir)
	if hashBefore != hashAfter {
		t.Errorf("excluded files changed hash: %q → %q", hashBefore, hashAfter)
	}
}

func TestContentHash_CRLFNormalization(t *testing.T) {
	dirLF := t.TempDir()
	os.WriteFile(filepath.Join(dirLF, "file.md"), []byte("line1\nline2\n"), 0o644)

	dirCRLF := t.TempDir()
	os.WriteFile(filepath.Join(dirCRLF, "file.md"), []byte("line1\r\nline2\r\n"), 0o644)

	hashLF, _ := contentHash(dirLF)
	hashCRLF, _ := contentHash(dirCRLF)

	if hashLF != hashCRLF {
		t.Errorf("CRLF normalization failed: LF=%q CRLF=%q", hashLF, hashCRLF)
	}
}

func TestContentHash_GithubDirIncluded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)
	hashBefore, _ := contentHash(dir)

	// .github/ should be included (it's legitimate skill content).
	os.MkdirAll(filepath.Join(dir, ".github"), 0o755)
	os.WriteFile(filepath.Join(dir, ".github", "template.md"), []byte("template"), 0o644)

	hashAfter, _ := contentHash(dir)
	if hashBefore == hashAfter {
		t.Error(".github/ content should be included in hash but was ignored")
	}
}

func TestContentHash_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644)
	os.Symlink("/nonexistent/path", filepath.Join(dir, "broken.md"))

	// Should not error — just skip the broken symlink.
	hash, err := contentHash(dir)
	if err != nil {
		t.Fatalf("broken symlink should be skipped, got error: %v", err)
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q", hash)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go test ./internal/discovery/ -run TestContentHash -v`
Expected: FAIL — `contentHash` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/discovery/hash.go`:

```go
package discovery

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// contentHash computes a deterministic fingerprint of a skill directory's contents.
// Returns the first 8 hex chars of SHA256(sorted relative paths + file contents).
//
// Design choice: symlinks are resolved before reading, so two skills pointing to
// the same source directory produce the same hash. This is intentional — they
// represent the same content.
func contentHash(dir string) (string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(dir, path)

		// Exclude specific directories at any depth.
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Exclude .DS_Store files.
		if info.Name() == ".DS_Store" {
			return nil
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk %s: %w", dir, err)
	}

	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		absPath := filepath.Join(dir, rel)

		// Resolve symlinks before reading.
		resolved, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			continue // skip broken/circular symlinks
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			continue // skip unreadable files
		}

		// Normalize CRLF → LF for cross-platform determinism.
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

		h.Write([]byte(rel))
		h.Write(data)
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:8], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go test ./internal/discovery/ -run TestContentHash -v`
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/hash.go internal/discovery/hash_test.go
git commit -m "[agent] feat: add content hash computation for skill directories

Step 1 of list-detail-view plan"
```

---

### Task 2: Upgrade `readSkillDescription` → `readSkillMetadata`

**Files:**
- Modify: `internal/discovery/discovery.go:15-28` (Skill struct)
- Modify: `internal/discovery/discovery.go:180-246` (readSkillDescription → readSkillMetadata)
- Create: `internal/discovery/discovery_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/discovery/discovery_test.go`:

```go
package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSkillMetadata_FullFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: browse
version: 1.1.0
description: Fast headless browser for QA testing.
---

# Browse

Content here.
`), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Name != "browse" {
		t.Errorf("Name: got %q, want %q", meta.Name, "browse")
	}
	if meta.Version != "1.1.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "1.1.0")
	}
	if meta.Description != "Fast headless browser for QA testing." {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_NoVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: ascii
description: ASCII diagram generator
---
`), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "" {
		t.Errorf("Version should be empty, got %q", meta.Version)
	}
	if meta.Description != "ASCII diagram generator" {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`# My Skill

This is the first paragraph.
`), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "" {
		t.Errorf("Version should be empty, got %q", meta.Version)
	}
	if meta.Description != "This is the first paragraph." {
		t.Errorf("Description: got %q", meta.Description)
	}
}

func TestReadSkillMetadata_NoSkillMD(t *testing.T) {
	dir := t.TempDir()
	// No SKILL.md at all.

	meta := readSkillMetadata(dir)

	if meta.Name != "" || meta.Version != "" || meta.Description != "" {
		t.Errorf("expected empty meta, got %+v", meta)
	}
}

func TestReadSkillMetadata_MultilineDescription(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: test
version: 2.0.0
description: |
  A multiline description here.
---
`), 0o644)

	meta := readSkillMetadata(dir)

	if meta.Version != "2.0.0" {
		t.Errorf("Version: got %q, want %q", meta.Version, "2.0.0")
	}
	if meta.Description != "A multiline description here." {
		t.Errorf("Description: got %q", meta.Description)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go test ./internal/discovery/ -run TestReadSkillMetadata -v`
Expected: FAIL — `readSkillMetadata` undefined

- [ ] **Step 3: Implement the changes**

In `internal/discovery/discovery.go`:

**3a.** Add `SkillMeta` type and update `Skill` struct — replace the existing `Skill` struct:

```go
// SkillMeta holds metadata parsed from SKILL.md frontmatter.
type SkillMeta struct {
	Name        string
	Description string
	Version     string
}

// Skill represents a skill found on disk, optionally enriched with state info.
type Skill struct {
	Name        string
	Description string
	Package     string
	LocalPath   string
	Source      string
	Version     string
	ContentHash string
	Targets     []string
}
```

**3b.** Rename `readSkillDescription` → `readSkillMetadata` and add `version:` + `name:` parsing. Replace the function body:

```go
// readSkillMetadata extracts metadata from SKILL.md frontmatter.
// Parses name:, version:, and description: fields using line-by-line scanning
// (not a YAML library) to avoid type coercion issues.
func readSkillMetadata(skillDir string) SkillMeta {
	path := filepath.Join(skillDir, "SKILL.md")

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return SkillMeta{}
	}

	f, err := os.Open(resolved)
	if err != nil {
		return SkillMeta{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	pastTitle := false

	var meta SkillMeta

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontmatter && !frontmatterDone {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
		}

		if inFrontmatter {
			if strings.HasPrefix(line, "description:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				if val == "|" || val == ">" {
					for scanner.Scan() {
						next := strings.TrimSpace(scanner.Text())
						if next != "" {
							meta.Description = truncateDescription(next)
							break
						}
					}
				} else {
					meta.Description = truncateDescription(val)
				}
				continue
			}
			if strings.HasPrefix(line, "version:") {
				meta.Version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
				continue
			}
			if strings.HasPrefix(line, "name:") {
				meta.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				continue
			}
			continue
		}

		// Past frontmatter: look for first paragraph after # Title.
		if strings.HasPrefix(line, "# ") {
			pastTitle = true
			continue
		}
		if pastTitle && meta.Description == "" && strings.TrimSpace(line) != "" {
			meta.Description = truncateDescription(strings.TrimSpace(line))
		}
	}
	return meta
}
```

**3c.** Update `OnDisk()` to use `readSkillMetadata`, compute `ContentHash`, and resolve `Version`. In the skill enumeration loop, replace the `sk := Skill{...}` block:

```go
			meta := readSkillMetadata(skillDir)
			hash, _ := contentHash(skillDir)

			sk := Skill{
				Name:        name,
				Description: meta.Description,
				LocalPath:   skillDir,
				Package:     detectPackage(skillDir, dir.path),
				ContentHash: hash,
			}

			if installed, ok := st.Installed[name]; ok {
				sk.Source = installed.Source
				sk.Version = installed.DisplayVersion()
				sk.Targets = installed.Targets
			} else if dir.target != "" {
				sk.Targets = []string{dir.target}
			}

			// Version resolution: frontmatter → state → content hash.
			if meta.Version != "" {
				sk.Version = meta.Version
			}
			if sk.Version == "" && hash != "" {
				sk.Version = "#" + hash
			}
```

**3d.** Update the state-only skills block (skills in state but not on disk) to also set a version:

```go
		skills = append(skills, Skill{
			Name:    name,
			Source:  installed.Source,
			Version: installed.DisplayVersion(),
			Targets: installed.Targets,
		})
```

(This block doesn't change structurally — `Managed` is just no longer set since the field is removed.)

- [ ] **Step 4: Run all discovery tests**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go test ./internal/discovery/ -v`
Expected: all tests PASS

- [ ] **Step 5: Verify full build**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go build ./...`
Expected: clean build. If `Managed` is referenced elsewhere, fix those references (likely `internal/workflow/list.go:193` — addressed in Task 5).

- [ ] **Step 6: Commit**

```bash
git add internal/discovery/discovery.go internal/discovery/discovery_test.go
git commit -m "[agent] feat: upgrade readSkillDescription to readSkillMetadata with version parsing

Parses name:, version:, description: from SKILL.md frontmatter.
Drops Managed field, adds ContentHash, resolves version via
frontmatter → state → content hash chain.

Step 2 of list-detail-view plan"
```

---

### Task 3: Split-Pane Layout (rendering only, no action menu yet)

**Files:**
- Modify: `cmd/list_tui.go`

- [ ] **Step 1: Add `go-runewidth` dependency if not already present**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && grep -q runewidth go.mod && echo "already present" || go get github.com/mattn/go-runewidth`

- [ ] **Step 2: Rewrite `viewSkills()` with split-pane layout**

In `cmd/list_tui.go`, replace the `viewSkills()` method with the split-pane implementation. Keep the existing method as `viewSkillsSingleColumn()` for narrow terminal fallback:

```go
func (m listModel) viewSkills() string {
	if m.width < 80 {
		return m.viewSkillsSingleColumn()
	}
	return m.viewSkillsSplitPane()
}

func (m listModel) viewSkillsSingleColumn() string {
	var b strings.Builder

	label := m.groupKey
	if label == "" {
		label = "all"
	}
	title := ltHeaderStyle.Render("Installed Skills")
	group := ltCountStyle.Render(fmt.Sprintf("%s · %d skills", label, len(m.filtered)))
	b.WriteString(title + "  " + group + "\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", 40)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	contentHeight := m.contentHeight()
	linesUsed := 0

	if m.offset > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)) + "\n")
		linesUsed++
	}

	end := m.offset
	for i := m.offset; i < len(m.filtered); i++ {
		if linesUsed >= contentHeight {
			break
		}
		sk := m.filtered[i]
		isCursor := i == m.cursor

		line := m.formatSkillLine(sk, isCursor, m.width-4)
		b.WriteString(line + "\n")
		linesUsed++
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · type to search · esc back · q quit") + "\n")
	return b.String()
}

func (m listModel) viewSkillsSplitPane() string {
	var b strings.Builder

	// Header.
	label := m.groupKey
	if label == "" {
		label = "all"
	}
	title := ltHeaderStyle.Render("Installed Skills")
	group := ltCountStyle.Render(fmt.Sprintf("%s · %d skills", label, len(m.filtered)))
	b.WriteString(title + "  " + group + "\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", m.width)) + "\n")

	if m.search != "" {
		b.WriteString(fmt.Sprintf("> %s\n", m.search))
	}

	contentHeight := m.contentHeight()
	leftWidth, rightWidth := m.paneWidths()

	// Left pane: skill list.
	var leftLines []string
	if m.offset > 0 {
		leftLines = append(leftLines, ltDimStyle.Render(fmt.Sprintf("  ↑ %d more", m.offset)))
	}

	end := m.offset
	maxItems := contentHeight
	if m.offset > 0 {
		maxItems-- // scroll indicator takes a line
	}

	for i := m.offset; i < len(m.filtered) && len(leftLines) < maxItems; i++ {
		sk := m.filtered[i]
		isCursor := i == m.cursor
		leftLines = append(leftLines, m.formatSkillLine(sk, isCursor, leftWidth-2))
		end = i + 1
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		leftLines = append(leftLines, ltDimStyle.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	}

	// Pad left pane to contentHeight.
	for len(leftLines) < contentHeight {
		leftLines = append(leftLines, "")
	}
	leftContent := strings.Join(leftLines[:contentHeight], "\n")

	// Right pane: detail for cursor skill.
	rightContent := ""
	if m.cursor < len(m.filtered) {
		rightContent = m.renderDetail(m.filtered[m.cursor], rightWidth)
	}

	// Pad right pane to contentHeight.
	rightLines := strings.Split(rightContent, "\n")
	for len(rightLines) < contentHeight {
		rightLines = append(rightLines, "")
	}
	rightContent = strings.Join(rightLines[:contentHeight], "\n")

	// Join panes.
	leftRendered := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight).Render(leftContent)
	divider := strings.TrimRight(strings.Repeat("│\n", contentHeight), "\n")
	divRendered := lipgloss.NewStyle().Height(contentHeight).Foreground(lipgloss.Color("#555555")).Render(divider)
	rightRendered := lipgloss.NewStyle().Width(rightWidth).Height(contentHeight).Render(rightContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, divRendered, rightRendered)
	b.WriteString(body)

	// Footer.
	b.WriteString("\n\n")
	b.WriteString(ltDimStyle.Render("↑↓ navigate · enter actions · type to search · esc back · q quit") + "\n")
	return b.String()
}

// formatSkillLine renders a single skill as "▸ name — description" or "  name — description".
func (m listModel) formatSkillLine(sk discovery.Skill, isCursor bool, maxWidth int) string {
	prefix := "  "
	nameStyle := ltNameStyle
	descStyle := ltDescStyle
	if isCursor {
		prefix = ltCursorStyle.Render("▸") + " "
		nameStyle = ltCursorStyle
		descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0088aa"))
	}

	name := sk.Name
	// Calculate remaining space for description.
	// prefix (2) + name + " — " (3) = overhead
	descSpace := maxWidth - runewidth.StringWidth(name) - 5
	if sk.Description != "" && descSpace > 10 {
		desc := runewidth.Truncate(sk.Description, descSpace, "...")
		return prefix + nameStyle.Render(name) + " " + descStyle.Render("— "+desc)
	}
	return prefix + nameStyle.Render(name)
}

// renderDetail renders the right pane detail view for a skill.
func (m listModel) renderDetail(sk discovery.Skill, width int) string {
	var b strings.Builder

	b.WriteString(ltCursorStyle.Render(sk.Name) + "\n")

	if sk.Description != "" {
		// Word-wrap description to fit pane width.
		descStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("#aaaaaa"))
		b.WriteString(descStyle.Render(sk.Description) + "\n")
	}

	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	// Key-value metadata.
	type kv struct{ key, val string }
	var pairs []kv

	if sk.Version != "" {
		pairs = append(pairs, kv{"Version", sk.Version})
	}
	if sk.ContentHash != "" {
		pairs = append(pairs, kv{"Hash", sk.ContentHash})
	}
	if sk.Package != "" {
		pairs = append(pairs, kv{"Package", sk.Package})
	}
	if sk.Source != "" {
		pairs = append(pairs, kv{"Source", sk.Source})
	}
	if len(sk.Targets) > 0 {
		pairs = append(pairs, kv{"Targets", strings.Join(sk.Targets, ", ")})
	}
	if sk.LocalPath != "" {
		// Shorten home dir for display.
		path := sk.LocalPath
		if home, err := os.UserHomeDir(); err == nil {
			path = strings.Replace(path, home, "~", 1)
		}
		pairs = append(pairs, kv{"Path", path})
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(10)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))

	for _, p := range pairs {
		b.WriteString(keyStyle.Render(p.key) + valStyle.Render(p.val) + "\n")
	}

	return b.String()
}

// contentHeight returns the number of lines available for pane content.
func (m listModel) contentHeight() int {
	if m.height == 0 {
		return 20
	}
	headerHeight := 2 // title + divider
	searchHeight := 0
	if m.search != "" {
		searchHeight = 1
	}
	footerHeight := 2 // blank + help
	h := m.height - headerHeight - searchHeight - footerHeight
	if h < 5 {
		h = 5
	}
	return h
}

// paneWidths returns the left and right pane widths.
func (m listModel) paneWidths() (int, int) {
	left := m.width * 45 / 100
	if maxDynamic := m.width - 40; left > maxDynamic {
		left = maxDynamic
	}
	if left > 60 {
		left = 60
	}
	if left < 20 {
		left = 20
	}
	right := m.width - left - 3 // 3 for divider + padding
	if right < 20 {
		right = 20
	}
	return left, right
}
```

Add the import for `runewidth` and `os`:

```go
import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Naoray/scribe/internal/discovery"
)
```

- [ ] **Step 3: Update `ensureCursorVisible` to use `contentHeight`**

Replace the existing `ensureCursorVisible` and remove `maxContentLines`:

```go
func (m *listModel) ensureCursorVisible() {
	visible := m.contentHeight()
	if visible < 5 {
		visible = 5
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}
```

Delete the `maxContentLines()` method entirely.

- [ ] **Step 4: Verify build and run manually**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go build ./...`
Expected: clean build

Run manually to visually verify: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go run ./cmd/scribe list --local`
Expected: split-pane layout with skill list on left, detail on right. Cursor navigation updates right pane. Search works. Esc goes back to groups.

- [ ] **Step 5: Commit**

```bash
git add cmd/list_tui.go go.mod go.sum
git commit -m "[agent] feat: split-pane layout for list --local TUI

Left pane shows name + truncated description. Right pane shows
reactive detail preview with metadata. Falls back to single column
below 80 chars width.

Step 3 of list-detail-view plan"
```

---

### Task 4: Action Menu Phase

**Files:**
- Modify: `cmd/list_tui.go`

- [ ] **Step 1: Add phase constant and model fields**

Add `listPhaseActions` to the phase enum and new fields to `listModel`:

```go
const (
	listPhaseGroups listPhase = iota
	listPhaseSkills
	listPhaseActions
)

type listSubstate int

const (
	listSubstateNone listSubstate = iota
	listSubstateConfirm
)
```

Add to `listModel`:

```go
type listModel struct {
	phase         listPhase
	groups        []listGroupItem
	skills        []discovery.Skill
	filtered      []discovery.Skill
	state         *state.State  // needed for remove (state save) and managed derivation
	groupKey      string
	search        string
	cursor        int
	offset        int
	actionCursor  int       // cursor within action menu
	substate      listSubstate
	pendingTickID int       // for clipboard tick cancellation
	statusMsg     string    // ephemeral message in right pane
	quitting      bool
	width         int
	height        int
}
```

- [ ] **Step 2: Add action item type and list**

```go
type actionItem struct {
	label    string
	key      string // internal identifier
	disabled bool
	reason   string // shown when disabled
	style    lipgloss.Style
}

func actionsForSkill(sk discovery.Skill) []actionItem {
	isGhost := sk.LocalPath == ""
	return []actionItem{
		{
			label:    "update",
			key:      "update",
			disabled: true,
			reason:   "source unknown",
			style:    ltDimStyle,
		},
		{
			label:    "remove",
			key:      "remove",
			disabled: false,
			style:    lipgloss.NewStyle().Foreground(lipgloss.Color("#e06060")),
		},
		{
			label:    "add to category",
			key:      "category",
			disabled: true,
			reason:   "coming soon",
			style:    ltDimStyle,
		},
		{
			label:    "copy path",
			key:      "copy",
			disabled: isGhost,
			reason:   "no local path",
			style:    lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc")),
		},
		{
			label:    "open in $EDITOR",
			key:      "edit",
			disabled: isGhost,
			reason:   "no local path",
			style:    lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc")),
		},
	}
}
```

- [ ] **Step 3: Add `updateActions` handler**

```go
func (m listModel) updateActions(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.substate == listSubstateConfirm {
		return m.updateConfirm(msg)
	}

	actions := actionsForSkill(m.filtered[m.cursor])

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "escape":
		m.phase = listPhaseSkills
		m.actionCursor = 0
		m.statusMsg = ""
	case "up", "k":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down", "j":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		}
	case "enter":
		action := actions[m.actionCursor]
		if action.disabled {
			return m, nil // no-op on disabled items
		}
		return m.executeAction(action.key)
	}
	return m, nil
}

func (m listModel) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		return m.executeRemove()
	case "n":
		m.substate = listSubstateNone
		m.statusMsg = ""
	}
	// All other keys are no-ops in confirm substate.
	return m, nil
}
```

- [ ] **Step 4: Add `executeAction` dispatcher**

```go
func (m listModel) executeAction(key string) (tea.Model, tea.Cmd) {
	sk := m.filtered[m.cursor]
	switch key {
	case "copy":
		m.statusMsg = "Copied!"
		m.pendingTickID++
		tickID := m.pendingTickID
		return m, tea.Batch(
			tea.SetClipboard(sk.LocalPath),
			tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return clipboardTickMsg{id: tickID}
			}),
		)
	case "edit":
		editor := resolveEditor()
		skillMD := filepath.Join(sk.LocalPath, "SKILL.md")
		c := exec.Command(editor, skillMD)
		return m, tea.Exec(c, func(err error) tea.Msg {
			return editorDoneMsg{err: err}
		})
	case "remove":
		m.substate = listSubstateConfirm
		m.statusMsg = fmt.Sprintf("Remove %s? (y/n)", sk.Name)
		return m, nil
	}
	return m, nil
}

type clipboardTickMsg struct{ id int }
type editorDoneMsg struct{ err error }

func resolveEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}
```

Add imports: `"os/exec"`, `"path/filepath"`, `"time"`.

- [ ] **Step 5: Wire the new phase into `Update()`**

In the existing `Update` method, add the `listPhaseActions` case and `InterruptMsg` handling:

```go
func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
	case tea.InterruptMsg:
		m.quitting = true
		return m, tea.Quit
	case tea.ClipboardMsg:
		if msg.Error() != nil {
			m.statusMsg = "Clipboard failed"
		}
		return m, nil
	case clipboardTickMsg:
		if msg.id == m.pendingTickID {
			m.phase = listPhaseSkills
			m.actionCursor = 0
			m.statusMsg = ""
		}
		return m, nil
	case editorDoneMsg:
		// Re-read metadata for the current skill after editor closes.
		// This is a simple refresh — the full skill list is not re-scanned.
		if msg.err != nil {
			m.statusMsg = "Editor exited with error"
		}
		m.phase = listPhaseSkills
		m.actionCursor = 0
		return m, nil
	case tea.KeyPressMsg:
		switch m.phase {
		case listPhaseGroups:
			return m.updateGroups(msg)
		case listPhaseSkills:
			return m.updateSkills(msg)
		case listPhaseActions:
			return m.updateActions(msg)
		}
	}
	return m, nil
}
```

- [ ] **Step 6: Add enter key handling in `updateSkills`**

In the existing `updateSkills` method, add an `"enter"` case before the `default`:

```go
	case "enter":
		if len(m.filtered) > 0 {
			m.phase = listPhaseActions
			m.actionCursor = 0
			m.statusMsg = ""
		}
```

- [ ] **Step 7: Add action menu rendering in `viewSkillsSplitPane`**

Add an `renderActions` method and update `viewSkillsSplitPane` to use it when in action phase:

```go
// renderActions renders the action menu for the right pane.
func (m listModel) renderActions(sk discovery.Skill, width int) string {
	var b strings.Builder

	// Skill header.
	b.WriteString(ltCursorStyle.Render(sk.Name))
	meta := ""
	if sk.Package != "" {
		meta += sk.Package
	}
	if sk.Version != "" {
		if meta != "" {
			meta += " · "
		}
		meta += sk.Version
	}
	if meta != "" {
		b.WriteString(" " + ltCountStyle.Render(meta))
	}
	b.WriteString("\n")
	b.WriteString(ltDivStyle.Render(strings.Repeat("─", width-2)) + "\n")

	if m.statusMsg != "" {
		b.WriteString("\n" + m.statusMsg + "\n")
		return b.String()
	}

	actions := actionsForSkill(sk)
	for i, action := range actions {
		isCursor := i == m.actionCursor
		prefix := "  "
		if isCursor {
			prefix = ltCursorStyle.Render("▸") + " "
		}

		if action.disabled {
			label := ltDimStyle.Render(action.label)
			reason := ""
			if action.reason != "" {
				reason = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Italic(true).Render(action.reason)
			}
			b.WriteString(prefix + label + reason + "\n")
		} else {
			label := action.style.Render(action.label)
			if isCursor {
				label = ltCursorStyle.Render(action.label)
			}
			b.WriteString(prefix + label + "\n")
		}
	}

	return b.String()
}
```

In `viewSkillsSplitPane`, change the right pane content based on phase. Replace the `rightContent` block:

```go
	// Right pane: detail or action menu.
	rightContent := ""
	if m.cursor < len(m.filtered) {
		sk := m.filtered[m.cursor]
		if m.phase == listPhaseActions {
			rightContent = m.renderActions(sk, rightWidth)
		} else {
			rightContent = m.renderDetail(sk, rightWidth)
		}
	}
```

For the dimmed left pane in action phase, wrap the left pane rendering:

```go
	leftStyle := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight)
	if m.phase == listPhaseActions {
		leftStyle = leftStyle.Foreground(lipgloss.Color("#444444"))
	}
	leftRendered := leftStyle.Render(leftContent)
```

Update footer based on phase:

```go
	// Footer.
	b.WriteString("\n\n")
	if m.phase == listPhaseActions {
		b.WriteString(ltDimStyle.Render("↑↓ navigate · enter select · esc back to list") + "\n")
	} else {
		b.WriteString(ltDimStyle.Render("↑↓ navigate · enter actions · type to search · esc back · q quit") + "\n")
	}
```

- [ ] **Step 8: Implement `executeRemove`**

```go
func (m listModel) executeRemove() (tea.Model, tea.Cmd) {
	sk := m.filtered[m.cursor]

	// Safety check: validate path is within known skill directories.
	home, _ := os.UserHomeDir()
	allowedPrefixes := []string{
		filepath.Join(home, ".scribe", "skills"),
		filepath.Join(home, ".claude", "skills"),
	}

	pathAllowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(sk.LocalPath, prefix) {
			pathAllowed = true
			break
		}
	}

	if sk.LocalPath != "" && !pathAllowed {
		m.statusMsg = "Cannot remove: path outside managed directories"
		m.substate = listSubstateNone
		return m, nil
	}

	// Save state first (before disk deletion) to prevent orphaned state on interrupt.
	m.state.Remove(sk.Name)
	if err := m.state.Save(); err != nil {
		m.statusMsg = fmt.Sprintf("Save failed: %v", err)
		m.substate = listSubstateNone
		return m, nil
	}

	// Delete from disk.
	if sk.LocalPath != "" {
		info, err := os.Lstat(sk.LocalPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				os.Remove(sk.LocalPath) // symlink: remove link only
			} else {
				os.RemoveAll(sk.LocalPath) // directory: remove recursively
			}
		}
	}

	// Remove from filtered list and skills list.
	m.filtered = append(m.filtered[:m.cursor], m.filtered[m.cursor+1:]...)
	for i, s := range m.skills {
		if s.Name == sk.Name {
			m.skills = append(m.skills[:i], m.skills[i+1:]...)
			break
		}
	}

	// Adjust cursor.
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	m.phase = listPhaseSkills
	m.substate = listSubstateNone
	m.actionCursor = 0
	m.statusMsg = ""

	return m, nil
}
```

The `state` field on `listModel` must be passed via `newListModel`. Update `newListModel` signature to accept `*state.State` and store it. Update the `ListTUI` callback in `cmd/list.go` to pass state through. The `executeRemove` method calls `m.state.Remove(sk.Name)` then `m.state.Save()` before any disk deletion — if Save fails, show error and abort.

- [ ] **Step 9: Verify build and manual test**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go build ./...`
Expected: clean build

Manual test: `go run ./cmd/scribe list --local`, navigate to a skill, press enter → see action menu, esc → back to list, test copy path, test open in editor.

- [ ] **Step 10: Commit**

```bash
git add cmd/list_tui.go
git commit -m "[agent] feat: action menu phase for list --local TUI

Enter on a skill shows actions: update (disabled), remove, add to
category (disabled), copy path, open in editor. Left pane dims to
show focus shift. Confirm substate for remove with path safety check.

Step 4 of list-detail-view plan"
```

---

### Task 5: Update JSON Output

**Files:**
- Modify: `internal/workflow/list.go:166-199`

- [ ] **Step 1: Update `printLocalJSON` to use new fields**

Replace the `localSkillJSON` struct and mapping in `printLocalJSON`:

```go
func printLocalJSON(w io.Writer, skills []discovery.Skill, st *state.State) error {
	type localSkillJSON struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Version     string   `json:"version"`
		ContentHash string   `json:"content_hash,omitempty"`
		Source      string   `json:"source"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	out := make([]localSkillJSON, 0, len(skills))
	for _, sk := range skills {
		tgts := sk.Targets
		if tgts == nil {
			tgts = []string{}
		}

		_, managed := st.Installed[sk.Name]

		out = append(out, localSkillJSON{
			Name:        sk.Name,
			Description: sk.Description,
			Package:     sk.Package,
			Version:     sk.Version,
			ContentHash: sk.ContentHash,
			Source:      sk.Source,
			Targets:     tgts,
			Managed:     managed,
			Path:        sk.LocalPath,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

- [ ] **Step 2: Update `listLocal` to pass state to `printLocalJSON`**

Update the `listLocal` function signature and call site:

```go
func listLocal(w io.Writer, st *state.State, useJSON bool, tuiFn func([]discovery.Skill) error) error {
	skills, err := discovery.OnDisk(st)
	if err != nil {
		return err
	}

	if useJSON {
		return printLocalJSON(w, skills, st)
	}
	if tuiFn != nil {
		return tuiFn(skills)
	}
	return printLocalTable(w, skills)
}
```

The caller `StepBranchLocalOrRemote` already passes `b.State` — no change needed there since `listLocal` already receives `st *state.State`.

- [ ] **Step 3: Verify build**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go build ./...`
Expected: clean build

- [ ] **Step 4: Manual test JSON output**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go run ./cmd/scribe list --local --json | head -20`
Expected: JSON with `content_hash`, `version` (always populated), `managed` fields.

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/list.go
git commit -m "[agent] feat: add content_hash and resolved version to JSON output

Derives managed from state presence. Version always populated via
frontmatter → state → content hash resolution chain.

Step 5 of list-detail-view plan"
```

---

### Task 6: Wire `tea.WithContext` and Final Cleanup

**Files:**
- Modify: `cmd/list.go`
- Modify: `cmd/list_tui.go` (minor cleanup)

- [ ] **Step 1: Pass Cobra context to tea.NewProgram**

In `cmd/list.go`, update the `ListTUI` callback to pass state and context. Note: the `ListTUI` callback type in `workflow.Bag` needs updating to accept `*state.State`, or the state can be captured via closure:

```go
	if isTTY && !jsonFlag {
		bag.ListTUI = func(skills []discovery.Skill) error {
			m := newListModel(skills, groupFlag, bag.State)
			p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
			_, err := p.Run()
			if errors.Is(err, tea.ErrInterrupted) {
				os.Exit(130)
			}
			if err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		}
	}
```

- [ ] **Step 2: Full build and test**

Run: `cd /Users/krishankonig/.anvil/worktrees/virovet-diagnostik/feat-list-detail-view && go build ./... && go test ./...`
Expected: clean build, all tests pass

- [ ] **Step 3: Commit**

```bash
git add cmd/list.go cmd/list_tui.go
git commit -m "[agent] chore: wire tea.WithContext for cancellation support

Step 6 of list-detail-view plan"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Content hash computation | `internal/discovery/hash.go`, `hash_test.go` |
| 2 | `readSkillMetadata` + version resolution | `internal/discovery/discovery.go`, `discovery_test.go` |
| 3 | Split-pane layout rendering | `cmd/list_tui.go` |
| 4 | Action menu phase + remove/copy/edit | `cmd/list_tui.go` |
| 5 | JSON output update | `internal/workflow/list.go` |
| 6 | `tea.WithContext` + cleanup | `cmd/list.go` |
