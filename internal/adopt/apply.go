package adopt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// Apply adopts each candidate into the canonical Scribe store.
// Per-candidate failures are non-fatal. Returns an aggregate Result.
func (a *Adopter) Apply(candidates []Candidate) Result {
	result := Result{
		Failed: make(map[string]error),
	}

	for _, cand := range candidates {
		if err := a.applyOne(cand, &result); err != nil {
			a.emit(AdoptErrorMsg{Name: cand.Name, Err: err})
			result.Failed[cand.Name] = err
		}
	}

	a.emit(AdoptCompleteMsg{
		Adopted: len(result.Adopted),
		Skipped: len(result.Skipped),
		Failed:  len(result.Failed),
	})

	return result
}

// applyOne handles adoption of a single candidate. Returns the first fatal error.
func (a *Adopter) applyOne(cand Candidate, result *Result) error {
	var canonicalDir string

	if cand.reLinkOnly {
		// Re-link: canonical store already exists; just refresh tool symlinks.
		storeDir, err := tools.StoreDir()
		if err != nil {
			return fmt.Errorf("store dir: %w", err)
		}
		canonicalDir = filepath.Join(storeDir, cand.Name)
		if _, err := os.Stat(canonicalDir); err != nil {
			return fmt.Errorf("canonical dir missing for re-link: %w", err)
		}
	} else {
		// Normal adoption: collect files and write to store.
		files, err := collectSkillFiles(cand.LocalPath)
		if err != nil {
			return fmt.Errorf("collect files: %w", err)
		}

		cd, err := tools.WriteToStore(cand.Name, files)
		if err != nil {
			return fmt.Errorf("write to store: %w", err)
		}
		canonicalDir = cd
	}

	// Determine which tools to install into.
	targetTools := a.resolveTargetTools(cand.Targets)

	// Pre-remove: delete the tool-facing path for each target so that
	// replaceSymlink (called inside Install) never hits ENOTEMPTY on a real
	// directory. This is the primary failure mode when adopting a skill that
	// was previously a plain directory rather than a symlink.
	// os.RemoveAll on a symlink removes the symlink, not the target — canonical
	// store content written above is safe.
	for _, tool := range targetTools {
		skillPath, err := tool.SkillPath(cand.Name)
		if err != nil || skillPath == "" {
			// Tool doesn't expose a predictable path (e.g. Gemini); skip.
			continue
		}
		if err := os.RemoveAll(skillPath); err != nil {
			return fmt.Errorf("pre-remove %s for %s: %w", skillPath, tool.Name(), err)
		}
	}

	// Install into each target tool.
	var installedToolNames []string
	var allPaths []string

	for _, tool := range targetTools {
		paths, err := tool.Install(cand.Name, canonicalDir)
		if err != nil {
			return fmt.Errorf("install into %s: %w", tool.Name(), err)
		}
		installedToolNames = append(installedToolNames, tool.Name())
		allPaths = append(allPaths, paths...)
	}

	if cand.reLinkOnly {
		// State entry already exists — don't overwrite it.
		a.emit(AdoptedMsg{Name: cand.Name, Tools: installedToolNames})
		result.Adopted = append(result.Adopted, cand.Name)
		return nil
	}

	// Record in state.
	a.State.RecordInstall(cand.Name, state.InstalledSkill{
		Revision:      1,
		InstalledHash: cand.Hash,
		Sources:       nil,
		Tools:         installedToolNames,
		Paths:         allPaths,
		Origin:        state.OriginLocal,
		// TODO: set ToolsMode: state.ToolsModeInherit when per-skill tool mgmt lands
	})

	if err := a.State.Save(); err != nil {
		// State save failed: remove the in-memory record so the next run
		// can retry. Count as failed so the caller knows persistence didn't happen.
		a.State.Remove(cand.Name)
		return fmt.Errorf("save state: %w", err)
	}

	a.emit(AdoptedMsg{Name: cand.Name, Tools: installedToolNames})
	result.Adopted = append(result.Adopted, cand.Name)
	return nil
}

// resolveTargetTools returns the subset of a.Tools matching targetNames.
// If targetNames is empty, returns all tools in a.Tools.
func (a *Adopter) resolveTargetTools(targetNames []string) []tools.Tool {
	if len(targetNames) == 0 {
		return a.Tools
	}
	nameSet := make(map[string]bool, len(targetNames))
	for _, n := range targetNames {
		nameSet[n] = true
	}
	var out []tools.Tool
	for _, t := range a.Tools {
		if nameSet[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

// collectSkillFiles walks skillDir recursively and returns a []tools.SkillFile.
// Skips reserved top-level entries and guards against path traversal.
func collectSkillFiles(skillDir string) ([]tools.SkillFile, error) {
	var files []tools.SkillFile

	err := filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(skillDir, path)
		if relErr != nil {
			return relErr
		}

		// Root dir itself: continue.
		if rel == "." {
			return nil
		}

		// Reject traversal attempts.
		clean := filepath.Clean(rel)
		if strings.Contains(clean, "..") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip reserved top-level entries.
		topLevel := strings.SplitN(clean, string(filepath.Separator), 2)[0]
		if reservedName(topLevel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil // recurse
		}

		// Symlinks: WalkDir reports them via Lstat, so IsDir() is false even when
		// the target is a directory. Resolve with Stat; skip dir targets (a sibling
		// dir inside skillDir is walked on its own; external dirs would leak content)
		// and also skip dangling links. File targets fall through to ReadFile, which
		// dereferences naturally.
		if d.Type()&fs.ModeSymlink != 0 {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
		}

		// Read file content.
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // race: file disappeared
			}
			return fmt.Errorf("read %s: %w", rel, err)
		}

		files = append(files, tools.SkillFile{Path: rel, Content: content})
		return nil
	})

	return files, err
}
