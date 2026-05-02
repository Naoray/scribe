package projectmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/paths"
)

func BudgetForProjectChange(change ProjectChange, links []GlobalSymlink) (map[string]budget.Result, error) {
	storeDir, err := paths.StoreDir()
	if err != nil {
		return nil, fmt.Errorf("resolve store dir: %w", err)
	}
	pathBySkill := map[string]string{}
	for _, link := range links {
		if _, ok := pathBySkill[link.Skill]; !ok {
			pathBySkill[link.Skill] = link.CanonicalPath
		}
	}
	names := append([]string(nil), change.Skills...)
	sort.Strings(names)
	skills := make([]budget.Skill, 0, len(names))
	for _, name := range names {
		content, ok := readBudgetSkillContent(name, pathBySkill[name], storeDir)
		if !ok {
			continue
		}
		skills = append(skills, budget.Skill{Name: name, Content: content})
	}
	agents := make([]string, 0, len(budget.AgentBudgets))
	for agent := range budget.AgentBudgets {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	results := make(map[string]budget.Result, len(agents))
	for _, agent := range agents {
		results[agent] = budget.CheckBudget(skills, agent)
	}
	return results, nil
}

func readBudgetSkillContent(name, linkedDir, storeDir string) ([]byte, bool) {
	dirs := []string{}
	if linkedDir != "" {
		dirs = append(dirs, linkedDir)
	}
	dirs = append(dirs, filepath.Join(storeDir, name))
	for _, dir := range dirs {
		content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if err == nil {
			return content, true
		}
	}
	return nil, false
}
