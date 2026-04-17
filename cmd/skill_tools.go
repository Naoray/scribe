package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func availableToolNames(statuses []tools.Status) []string {
	names := make([]string, 0, len(statuses))
	for _, st := range statuses {
		if !st.Enabled {
			continue
		}
		names = append(names, st.Name)
	}
	return names
}

func applySkillToolSelection(cfg *config.Config, st *state.State, name string, mode state.ToolsMode, desired []string) (skillEditResult, error) {
	if st == nil {
		return skillEditResult{}, fmt.Errorf("load state: missing")
	}

	installed, ok := st.Installed[name]
	if !ok {
		return skillEditResult{}, fmt.Errorf("skill %q is not installed (run `scribe list` to see managed skills)", name)
	}
	if installed.IsPackage() {
		return skillEditResult{}, fmt.Errorf("skill %q is a package — per-skill tool pinning does not apply", name)
	}

	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		return skillEditResult{}, fmt.Errorf("resolve tools: %w", err)
	}
	availableNames := availableToolNames(statuses)

	availableByName := make(map[string]tools.Tool, len(availableNames))
	for _, toolName := range availableNames {
		tool, err := tools.ResolveByName(cfg, toolName)
		if err != nil {
			return skillEditResult{}, err
		}
		availableByName[toolName] = tool
	}

	desired = state.NormalizeToolSelection(desired)
	if mode == state.ToolsModeInherit {
		desired = append([]string(nil), availableNames...)
	} else {
		var unknown []string
		for _, toolName := range desired {
			if _, ok := availableByName[toolName]; !ok {
				unknown = append(unknown, toolName)
			}
		}
		if len(unknown) > 0 {
			return skillEditResult{}, fmt.Errorf("unknown or unavailable tool(s): %s", strings.Join(unknown, ", "))
		}
		if len(desired) == 0 {
			return skillEditResult{}, fmt.Errorf("cannot pin skill %q to zero tools — switch to inherit or enable at least one tool", name)
		}
	}

	currentTools := append([]string(nil), installed.Tools...)
	currentSet := setOf(currentTools)
	desiredSet := setOf(desired)

	var added, removed []string
	for _, toolName := range desired {
		if !currentSet[toolName] {
			added = append(added, toolName)
		}
	}
	for _, toolName := range currentTools {
		if !desiredSet[toolName] {
			removed = append(removed, toolName)
		}
	}

	canonicalDir := filepath.Join(mustStoreDir(), name)
	if _, err := os.Stat(canonicalDir); err != nil {
		return skillEditResult{}, fmt.Errorf("canonical store for %q missing: %w", name, err)
	}

	existingManagedPaths := installed.ManagedPaths
	if len(existingManagedPaths) == 0 {
		existingManagedPaths = installed.Paths
	}
	newPathSet := make(map[string]bool, len(existingManagedPaths))
	for _, p := range existingManagedPaths {
		newPathSet[p] = true
	}

	for _, toolName := range removed {
		tool, ok := availableByName[toolName]
		if !ok {
			var resolveErr error
			tool, resolveErr = tools.ResolveByName(cfg, toolName)
			if resolveErr != nil {
				continue
			}
		}
		if err := tool.Uninstall(name); err != nil {
			fmt.Fprintf(os.Stderr, "warning: uninstall from %s: %v\n", toolName, err)
		}
		skillPath, _ := tool.SkillPath(name)
		if skillPath != "" {
			for p := range newPathSet {
				if strings.HasPrefix(p, skillPath) || p == skillPath {
					delete(newPathSet, p)
				}
			}
		}
	}

	for _, toolName := range added {
		tool := availableByName[toolName]
		paths, err := tool.Install(name, canonicalDir)
		if err != nil {
			return skillEditResult{}, fmt.Errorf("install into %s: %w", toolName, err)
		}
		for _, p := range paths {
			newPathSet[p] = true
		}
	}

	newPaths := make([]string, 0, len(newPathSet))
	for p := range newPathSet {
		newPaths = append(newPaths, p)
	}
	sort.Strings(newPaths)

	installed.Tools = desired
	installed.ToolsMode = mode
	installed.Paths = newPaths
	installed.ManagedPaths = append([]string(nil), newPaths...)
	st.Installed[name] = installed
	if err := st.Save(); err != nil {
		return skillEditResult{}, fmt.Errorf("save state: %w", err)
	}

	result := skillEditResult{
		Name:      name,
		ToolsMode: string(mode),
		Tools:     desired,
		Added:     added,
		Removed:   removed,
	}
	if mode == state.ToolsModeInherit {
		result.ToolsMode = "inherit"
	}
	return result, nil
}
