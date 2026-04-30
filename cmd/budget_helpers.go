package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type resolvedBudgetSet struct {
	ProjectRoot string
	Skills      []budget.Skill
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

func enforceCurrentBudget(factory *app.Factory, force bool) error {
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
		return err
	}
	for _, agent := range budgetAgents(cfg) {
		result := budget.CheckBudget(set.Skills, agent)
		switch result.Status {
		case budget.StatusRefuse:
			if !force {
				return fmt.Errorf("%s\nTry removing one kit, or rerun with --force to project anyway.", budget.FormatOverflow(result))
			}
		case budget.StatusWarn:
			fmt.Fprintf(os.Stderr, "warning: %s\n", budget.FormatResult(result))
		}
	}
	return nil
}
