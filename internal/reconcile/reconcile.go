package reconcile

import (
	"bytes"
	"crypto/sha256"
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
	inspectable := make(map[string]tools.Tool, len(e.Tools))
	for _, tool := range e.Tools {
		activeNames = append(activeNames, tool.Name())
		if supportsProjectionInspect(tool) {
			inspectable[tool.Name()] = tool
		}
	}

	for name, skill := range st.Installed {
		if skill.Type == "package" {
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
			tool, ok := inspectable[toolName]
			if !ok {
				continue
			}
			path, err := tool.SkillPath(name)
			if err != nil {
				continue
			}
			expectedPaths[path] = toolName

			info, err := os.Lstat(path)
			if err != nil {
				if errorsIsNotExist(err) {
					links, installErr := tool.Install(name, canonicalDir)
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

			if pathPointsToCanonical(path, canonicalDir, toolName) {
				newManaged[path] = true
				actions = append(actions, Action{Kind: ActionUnchanged, Name: name, Tool: toolName, Path: path})
				_ = info
				continue
			}

			foundHash, hashErr := projectionHash(path)
			if hashErr == nil {
				wantHash, wantErr := canonicalProjectionHash(canonicalDir, toolName)
				if wantErr == nil && foundHash == wantHash {
					if err := os.RemoveAll(path); err != nil && !errorsIsNotExist(err) {
						return summary, actions, err
					}
					links, installErr := tool.Install(name, canonicalDir)
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
			if pathPointsToCanonical(path, canonicalDir, inferToolName(path, inspectable, name)) {
				if err := os.Remove(path); err != nil && !errorsIsNotExist(err) {
					return summary, actions, err
				}
				summary.Removed++
				actions = append(actions, Action{Kind: ActionRemoved, Name: name, Path: path})
				continue
			}
			foundHash, _ := projectionHash(path)
			conflict := state.ProjectionConflict{
				Tool:      inferToolName(path, inspectable, name),
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
		skill.Paths = append([]string(nil), managedPaths...)
		skill.Conflicts = conflicts
		st.Installed[name] = skill
	}

	return summary, actions, nil
}

func supportsProjectionInspect(tool tools.Tool) bool {
	switch tool.Name() {
	case "claude", "cursor", "codex":
		return true
	default:
		return false
	}
}

func projectionPaths(skill state.InstalledSkill) []string {
	if len(skill.ManagedPaths) > 0 {
		return append([]string(nil), skill.ManagedPaths...)
	}
	return append([]string(nil), skill.Paths...)
}

func inferToolName(path string, inspectable map[string]tools.Tool, skillName string) string {
	for name, tool := range inspectable {
		toolPath, err := tool.SkillPath(skillName)
		if err == nil && toolPath == path {
			return name
		}
	}
	return ""
}

func pathPointsToCanonical(path, canonicalDir, toolName string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	canonicalResolved, err := filepath.EvalSymlinks(canonicalDir)
	if err == nil && resolved == canonicalResolved {
		return true
	}
	if err == nil && resolved == filepath.Join(canonicalResolved, "SKILL.md") {
		return true
	}
	if err == nil && resolved == filepath.Join(canonicalResolved, ".cursor.mdc") {
		return true
	}
	if toolName == "" {
		return false
	}
	want, err := canonicalProjectionTarget(canonicalDir, toolName)
	if err != nil {
		return false
	}
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		wantResolved = want
	}
	return resolved == wantResolved
}

func canonicalProjectionTarget(canonicalDir, toolName string) (string, error) {
	switch toolName {
	case "claude":
		return filepath.Join(canonicalDir, "SKILL.md"), nil
	case "cursor":
		return filepath.Join(canonicalDir, ".cursor.mdc"), nil
	case "codex":
		return canonicalDir, nil
	default:
		return "", fmt.Errorf("tool %q has no inspectable projection target", toolName)
	}
}

func canonicalProjectionHash(canonicalDir, toolName string) (string, error) {
	target, err := canonicalProjectionTarget(canonicalDir, toolName)
	if err != nil {
		return "", err
	}
	return projectionHash(target)
}

func projectionHash(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		skillFile := filepath.Join(path, "SKILL.md")
		return fileHash(skillFile)
	}
	return fileHash(path)
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

func errorsIsNotExist(err error) bool {
	return err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), fs.ErrNotExist.Error()))
}

func CopyProjectionToCanonical(path, toolName, canonicalDir string) error {
	switch toolName {
	case "codex":
		return copyDir(path, canonicalDir)
	case "claude":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return tools.WriteCanonicalSkill(canonicalDir, data)
	default:
		return fmt.Errorf("tool %q cannot be promoted into canonical store", toolName)
	}
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
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
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
