package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/snippet"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/spf13/cobra"
)

type resolvedBudgetSet struct {
	ProjectRoot string
	Skills      []budget.Skill
	Snippets    []snippet.Snippet
	Missing     []string
}

func resolveBudgetSet(st *state.State) (resolvedBudgetSet, error) {
	storeDir, err := tools.StoreDir()
	if err != nil {
		return resolvedBudgetSet{}, fmt.Errorf("resolve store dir: %w", err)
	}

	names, projectRoot, err := resolveBudgetSkillNames(st)
	if err != nil {
		return resolvedBudgetSet{}, err
	}

	var out resolvedBudgetSet
	out.ProjectRoot = projectRoot
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(storeDir, name, "SKILL.md"))
		if err != nil {
			out.Missing = append(out.Missing, name)
			continue
		}
		out.Skills = append(out.Skills, budget.Skill{Name: name, Content: content})
	}
	if projectRoot != "" {
		projectPath := filepath.Join(projectRoot, projectfile.Filename)
		pf, err := projectfile.Load(projectPath)
		if err == nil && len(pf.Snippets) > 0 {
			home, herr := os.UserHomeDir()
			if herr == nil {
				snippets, serr := snippet.LoadProject(snippet.Dir(home), pf.Snippets)
				if serr != nil {
					return out, serr
				}
				out.Snippets = snippets
			}
		}
	}
	return out, nil
}

func resolveBudgetSkillNames(st *state.State) ([]string, string, error) {
	installedNames := installedSkillNames(st)

	wd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("get working directory: %w", err)
	}
	projectPath, err := projectfile.Find(wd)
	if err != nil {
		return nil, "", err
	}
	if projectPath == "" {
		return installedNames, "", nil
	}

	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return nil, "", err
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return nil, "", err
	}
	kits, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return nil, "", err
	}
	resolved, err := kit.Resolve(pf, kits, installedNames)
	if err != nil {
		return nil, "", err
	}
	return resolved, filepath.Dir(projectPath), nil
}

func installedSkillNames(st *state.State) []string {
	if st == nil {
		return nil
	}
	names := make([]string, 0, len(st.Installed))
	for name, installed := range st.Installed {
		if installed.Kind == state.KindPackage {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func budgetAgents(_ *config.Config) []string {
	agents := make([]string, 0, len(budget.AgentBudgets))
	for agent := range budget.AgentBudgets {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	return agents
}

func budgetSkillsForAgent(set resolvedBudgetSet, st *state.State, agent string) []budget.Skill {
	if st == nil {
		return append([]budget.Skill(nil), set.Skills...)
	}
	skills := make([]budget.Skill, 0, len(set.Skills))
	for _, skill := range set.Skills {
		installed, ok := st.Installed[skill.Name]
		if ok && !budgetSkillTargetsAgent(installed, set.ProjectRoot, agent) {
			continue
		}
		skills = append(skills, skill)
	}
	for _, sn := range set.Snippets {
		if !snippet.TargetsAgent(sn.Targets, agent) {
			continue
		}
		skills = append(skills, budget.Skill{Name: "snippet:" + sn.Name, Content: snippet.ContentForBudget(sn)})
	}
	return skills
}

func budgetSkillTargetsAgent(installed state.InstalledSkill, projectRoot, agent string) bool {
	if projectRoot != "" {
		for _, projection := range installed.Projections {
			if projection.Project != projectRoot {
				continue
			}
			return containsBudgetTool(projection.Tools, agent) && !containsBudgetTool(projection.ExcludedTools, agent)
		}
	}
	if installed.ToolsMode == state.ToolsModePinned {
		return containsBudgetTool(installed.Tools, agent)
	}
	return true
}

func containsBudgetTool(tools []string, agent string) bool {
	for _, tool := range tools {
		if tool == agent {
			return true
		}
	}
	return false
}

const deprecatedForceBudgetWarning = "warning: --force is deprecated; budget guardrails are now warn-only"

func warnDeprecatedForceBudget(cmd *cobra.Command) {
	if cmd.Flags().Changed("force") {
		fmt.Fprintln(cmd.ErrOrStderr(), deprecatedForceBudgetWarning)
	}
}

func enforceCurrentBudget(factory *app.Factory) error {
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	set, err := resolveBudgetSet(st)
	if err != nil {
		// Budget preflight should not mask the sync workflow's authoritative
		// validation, fetch, or auth errors.
		return nil
	}
	var warnings []string
	for _, agent := range budgetAgents(cfg) {
		result := budget.CheckProjectionBudget(budgetSkillsForAgent(set, st, agent), agent)
		switch result.Status {
		case budget.StatusRefuse:
			warnings = append(warnings, budget.FormatOverflowCompact(result))
		case budget.StatusWarn:
			warnings = append(warnings, budget.FormatResult(result))
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %s\n", strings.Join(warnings, "\n\n"))
	}
	return nil
}
