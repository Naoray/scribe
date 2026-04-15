package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/reconcile"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func applySkillRepair(cfg *config.Config, st *state.State, name, toolName, source string) (skillRepairResult, error) {
	if source != "managed" && source != "tool" {
		return skillRepairResult{}, fmt.Errorf("skill repair: --from must be one of managed, tool")
	}
	if st == nil {
		return skillRepairResult{}, fmt.Errorf("load state: missing")
	}

	installed, ok := st.Installed[name]
	if !ok {
		return skillRepairResult{}, fmt.Errorf("skill %q is not installed", name)
	}

	tool, err := tools.ResolveByName(cfg, toolName)
	if err != nil {
		return skillRepairResult{}, err
	}
	path, err := tool.SkillPath(name)
	if err != nil {
		return skillRepairResult{}, err
	}
	canonicalDir := filepath.Join(mustStoreDir(), name)

	if source == "tool" {
		if err := reconcile.CopyProjectionToCanonical(tool, path, canonicalDir); err != nil {
			return skillRepairResult{}, err
		}
		skillMD, err := os.ReadFile(filepath.Join(canonicalDir, "SKILL.md"))
		if err == nil {
			installed.InstalledHash = sync.ComputeFileHash(skillMD)
		}
	}

	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return skillRepairResult{}, err
	}
	links, err := tool.Install(name, canonicalDir)
	if err != nil {
		return skillRepairResult{}, err
	}

	newManaged := make(map[string]bool)
	existingManagedPaths := installed.ManagedPaths
	if len(existingManagedPaths) == 0 {
		existingManagedPaths = installed.Paths
	}
	for _, p := range existingManagedPaths {
		if p != path {
			newManaged[p] = true
		}
	}
	for _, p := range links {
		newManaged[p] = true
	}

	managedPaths := make([]string, 0, len(newManaged))
	for p := range newManaged {
		managedPaths = append(managedPaths, p)
	}
	sort.Strings(managedPaths)

	filteredConflicts := make([]state.ProjectionConflict, 0, len(installed.Conflicts))
	for _, conflict := range installed.Conflicts {
		if conflict.Tool == toolName && conflict.Path == path {
			continue
		}
		filteredConflicts = append(filteredConflicts, conflict)
	}
	installed.ManagedPaths = managedPaths
	installed.Paths = append([]string(nil), managedPaths...)
	installed.Conflicts = filteredConflicts
	st.Installed[name] = installed
	if err := st.Save(); err != nil {
		return skillRepairResult{}, err
	}

	return skillRepairResult{Name: name, Tool: toolName, Source: source}, nil
}
