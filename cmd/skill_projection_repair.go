package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type skillProjectionRepairResult struct {
	Name  string
	Tools []string
}

func repairSkillProjections(cfg *config.Config, st *state.State, name string) (skillProjectionRepairResult, error) {
	if st == nil {
		return skillProjectionRepairResult{}, fmt.Errorf("load state: missing")
	}

	installed, ok := st.Installed[name]
	if !ok {
		return skillProjectionRepairResult{}, fmt.Errorf("skill %q is not installed", name)
	}
	if installed.Type == "package" {
		return skillProjectionRepairResult{}, fmt.Errorf("skill %q is a package — tool projection repair does not apply", name)
	}

	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		return skillProjectionRepairResult{}, fmt.Errorf("resolve tools: %w", err)
	}
	effective := installed.EffectiveTools(availableToolNames(statuses))
	if len(effective) == 0 {
		return skillProjectionRepairResult{}, fmt.Errorf("skill %q has no active tool targets to repair", name)
	}

	canonicalDir := filepath.Join(mustStoreDir(), name)
	newPaths := make(map[string]bool)
	for _, toolName := range effective {
		tool, err := tools.ResolveByName(cfg, toolName)
		if err != nil {
			return skillProjectionRepairResult{}, err
		}
		_ = tool.Uninstall(name)
		links, err := tool.Install(name, canonicalDir)
		if err != nil {
			return skillProjectionRepairResult{}, fmt.Errorf("repair %s/%s: %w", toolName, name, err)
		}
		for _, link := range links {
			newPaths[link] = true
		}
	}

	managedPaths := make([]string, 0, len(newPaths))
	for path := range newPaths {
		managedPaths = append(managedPaths, path)
	}
	sort.Strings(managedPaths)

	installed.ManagedPaths = managedPaths
	installed.Paths = append([]string(nil), managedPaths...)
	st.Installed[name] = installed
	if err := st.Save(); err != nil {
		return skillProjectionRepairResult{}, err
	}

	return skillProjectionRepairResult{Name: name, Tools: effective}, nil
}
