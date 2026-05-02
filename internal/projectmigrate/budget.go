package projectmigrate

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/budget"
)

func BudgetForProjectChange(change ProjectChange, links []GlobalSymlink) (map[string]budget.Result, error) {
	pathBySkill := map[string]string{}
	for _, link := range links {
		if _, ok := pathBySkill[link.Skill]; !ok {
			pathBySkill[link.Skill] = link.CanonicalPath
		}
	}
	skills := make([]budget.Skill, 0, len(change.Skills))
	for _, name := range change.Skills {
		dir, ok := pathBySkill[name]
		if !ok {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if err != nil {
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
