package reconcile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type ActionKind string

const (
	ActionInstalled ActionKind = "installed"
	ActionRelinked  ActionKind = "relinked"
	ActionRemoved   ActionKind = "removed"
	ActionConflict  ActionKind = "conflict"
	ActionUnchanged ActionKind = "unchanged"
)

type Action struct {
	Kind ActionKind
	Name string
	Tool string
	Path string
}

type Summary struct {
	Installed int
	Relinked  int
	Removed   int
	Conflicts []state.ProjectionConflict
}

type Engine struct {
	Tools []tools.Tool
	Now   func() time.Time
}

func (e *Engine) Run(st *state.State) (Summary, []Action, error) {
	var summary Summary
	var actions []Action

	storeDir, err := tools.StoreDir()
	if err != nil {
		return summary, nil, err
	}
	now := time.Now().UTC
	if e.Now != nil {
		now = e.Now
	}

	activeNames := make([]string, 0, len(e.Tools))
	byName := make(map[string]tools.Tool, len(e.Tools))
	for _, tool := range e.Tools {
		activeNames = append(activeNames, tool.Name())
		byName[tool.Name()] = tool
	}

	pkgsDir, pkgErr := tools.PackagesDir()
	if pkgErr != nil {
		return summary, nil, pkgErr
	}

	for name, skill := range st.Installed {
		if skill.IsPackage() {
			// Non-projection invariant: packages own their own wiring.
			// Still scan for stale tool symlinks that resolve back to the
			// package dir so a legacy skills/<name> → packages/<name> flip
			// cleans up any prior projections on the next sync.
			pkgDir := filepath.Join(pkgsDir, name)
			for _, path := range projectionPaths(skill) {
				if path == "" {
					continue
				}
				if isManagedProjection(path, pkgDir) {
					if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
						return summary, actions, err
					}
					summary.Removed++
					actions = append(actions, Action{Kind: ActionRemoved, Name: name, Tool: inferToolName(path, byName, name), Path: path})
				}
			}
			// Wipe tracked paths — packages never project.
			if len(skill.Paths) > 0 || len(skill.ManagedPaths) > 0 {
				skill.Paths = nil
				skill.ManagedPaths = nil
				st.Installed[name] = skill
			}
			continue
		}

		canonicalDir := filepath.Join(storeDir, name)
		expectedTools := skill.EffectiveTools(activeNames)
		expectedPaths := make(map[string]string, len(expectedTools))
		newManaged := make(map[string]bool)
		var conflicts []state.ProjectionConflict

		for _, conflict := range skill.Conflicts {
			if conflictStillPresent(conflict.Path, conflict.FoundHash) {
				conflicts = append(conflicts, conflict)
			}
		}

		for _, toolName := range expectedTools {
			tool, ok := byName[toolName]
			if !ok {
				continue
			}
			target, inspectable := tool.CanonicalTarget(canonicalDir)
			if !inspectable {
				continue
			}
			path, err := tool.SkillPath(name)
			if err != nil {
				continue
			}
			expectedPaths[path] = toolName

			if _, err := os.Lstat(path); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					links, installErr := tool.Install(name, canonicalDir, "")
					if installErr != nil {
						return summary, actions, fmt.Errorf("install %s/%s: %w", toolName, name, installErr)
					}
					for _, link := range links {
						newManaged[link] = true
					}
					summary.Installed++
					actions = append(actions, Action{Kind: ActionInstalled, Name: name, Tool: toolName, Path: path})
					continue
				}
				return summary, actions, err
			}

			if pathPointsToCanonical(path, target) {
				newManaged[path] = true
				actions = append(actions, Action{Kind: ActionUnchanged, Name: name, Tool: toolName, Path: path})
				continue
			}

			foundHash, hashErr := projectionHash(path)
			if hashErr == nil {
				wantHash, wantErr := projectionHash(target)
				if wantErr == nil && foundHash == wantHash {
					if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
						return summary, actions, err
					}
					links, installErr := tool.Install(name, canonicalDir, "")
					if installErr != nil {
						return summary, actions, fmt.Errorf("relink %s/%s: %w", toolName, name, installErr)
					}
					for _, link := range links {
						newManaged[link] = true
					}
					summary.Relinked++
					actions = append(actions, Action{Kind: ActionRelinked, Name: name, Tool: toolName, Path: path})
					continue
				}
			}

			conflict := state.ProjectionConflict{
				Tool:      toolName,
				Path:      path,
				FoundHash: foundHash,
				SeenAt:    now(),
			}
			conflicts = upsertConflict(conflicts, conflict)
			summary.Conflicts = append(summary.Conflicts, conflict)
			actions = append(actions, Action{Kind: ActionConflict, Name: name, Tool: toolName, Path: path})
		}

		for _, path := range projectionPaths(skill) {
			if _, stillExpected := expectedPaths[path]; stillExpected {
				continue
			}
			if path == "" {
				continue
			}
			toolName := inferToolName(path, byName, name)
			// A stale projection is safe to remove whenever it resolves
			// back into the canonical store — that guarantees it was Scribe
			// who put it there. Requiring a matching Tool in byName would
			// miss the case where the tool has been globally disabled but
			// its old projection still needs cleanup.
			if isManagedProjection(path, canonicalDir) {
				if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return summary, actions, err
				}
				summary.Removed++
				actions = append(actions, Action{Kind: ActionRemoved, Name: name, Tool: toolName, Path: path})
				continue
			}
			foundHash, _ := projectionHash(path)
			conflict := state.ProjectionConflict{
				Tool:      toolName,
				Path:      path,
				FoundHash: foundHash,
				SeenAt:    now(),
			}
			conflicts = upsertConflict(conflicts, conflict)
			summary.Conflicts = append(summary.Conflicts, conflict)
			actions = append(actions, Action{Kind: ActionConflict, Name: name, Tool: conflict.Tool, Path: path})
		}

		managedPaths := make([]string, 0, len(newManaged))
		for path := range newManaged {
			managedPaths = append(managedPaths, path)
		}
		sort.Strings(managedPaths)
		skill.ManagedPaths = managedPaths
		// Paths is clobbered on every reconcile pass because reconcile is the
		// source of truth for what projections Scribe currently manages. Any
		// entries in the previous Paths that are still valid are re-added via
		// newManaged above; anything missing is legitimately gone.
		skill.Paths = append([]string(nil), managedPaths...)
		skill.Conflicts = conflicts
		st.Installed[name] = skill
	}

	return summary, actions, nil
}

func projectionPaths(skill state.InstalledSkill) []string {
	if len(skill.ManagedPaths) > 0 {
		return append([]string(nil), skill.ManagedPaths...)
	}
	return append([]string(nil), skill.Paths...)
}

func inferToolName(path string, byName map[string]tools.Tool, skillName string) string {
	for name, tool := range byName {
		toolPath, err := tool.SkillPath(skillName)
		if err == nil && toolPath == path {
			return name
		}
	}
	return ""
}

// pathPointsToCanonical reports whether path already resolves to the canonical
// target (by symlink, bind mount, or direct equality). Returning true means
// reconcile can treat the projection as managed and not touch it.
func pathPointsToCanonical(path, target string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	targetResolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		targetResolved = target
	}
	return resolved == targetResolved
}

// isManagedProjection reports whether path resolves back into the skill's
// canonical store directory. Used when cleaning up stale projections whose
// owning tool may no longer be in the active set but whose link clearly
// originated from Scribe.
func isManagedProjection(path, canonicalDir string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	canonicalResolved, err := filepath.EvalSymlinks(canonicalDir)
	if err != nil {
		canonicalResolved = canonicalDir
	}
	if resolved == canonicalResolved {
		return true
	}
	rel, err := filepath.Rel(canonicalResolved, resolved)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// projectionHash returns a stable content hash for a projection target. Files
// are hashed directly; directories are walked and a manifest-style hash
// (relative path + contents) is computed so drift in any subfile is detected.
func projectionHash(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return fileHash(path)
	}
	return treeHash(path)
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])[:8], nil
}

// treeHash walks a directory and produces a hash that covers every regular
// file's relative path, executable bit, and normalized content. This means a
// drift buried in e.g. scripts/foo.sh is detected instead of silently
// relinked away.
func treeHash(root string) (string, error) {
	h := sha256.New()
	var entries []treeEntry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip Scribe internals that live alongside SKILL.md in the
		// canonical store but are never part of a tool projection.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == ".scribe-base.md" || rel == ".cursor.mdc" {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		entries = append(entries, treeEntry{rel: rel, mode: info.Mode().Perm(), path: path})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	for _, e := range entries {
		data, err := os.ReadFile(e.path)
		if err != nil {
			return "", err
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%s\x00%o\x00%d\x00", e.rel, e.mode, len(data))
		h.Write(data)
	}
	sum := h.Sum(nil)
	return fmt.Sprintf("%x", sum)[:8], nil
}

type treeEntry struct {
	rel  string
	mode os.FileMode
	path string
}

func upsertConflict(conflicts []state.ProjectionConflict, next state.ProjectionConflict) []state.ProjectionConflict {
	for i := range conflicts {
		if conflicts[i].Tool == next.Tool && conflicts[i].Path == next.Path {
			conflicts[i] = next
			return conflicts
		}
	}
	return append(conflicts, next)
}

func conflictStillPresent(path, wantHash string) bool {
	if path == "" {
		return false
	}
	if wantHash == "" {
		_, err := os.Lstat(path)
		return err == nil
	}
	got, err := projectionHash(path)
	return err == nil && got == wantHash
}

// CopyProjectionToCanonical promotes a tool's on-disk projection back into
// the canonical store, used by `scribe skill repair --from tool`. The shape
// is inferred from the tool's CanonicalTarget: if the target is a file,
// promotion only succeeds when that file is canonicalDir/SKILL.md (Claude);
// otherwise the projection is derived (e.g. Cursor's generated .cursor.mdc)
// and promoting it would be overwritten on the next install. Directory
// targets are copied wholesale.
func CopyProjectionToCanonical(tool tools.Tool, path, canonicalDir string) error {
	target, ok := tool.CanonicalTarget(canonicalDir)
	if !ok {
		return fmt.Errorf("tool %q cannot be promoted into canonical store", tool.Name())
	}
	// Directory projection → copy the whole tree.
	if filepath.Clean(target) == filepath.Clean(canonicalDir) {
		return copyDir(path, canonicalDir)
	}
	// Single-file projection → only SKILL.md is safe to promote.
	if filepath.Base(target) == "SKILL.md" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return tools.WriteCanonicalSkill(canonicalDir, data)
	}
	return fmt.Errorf("tool %q projection is derived from canonical store and cannot be promoted back", tool.Name())
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
