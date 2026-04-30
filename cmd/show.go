package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/budget"
)

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the resolved project skill set and agent budgets",
		RunE:  runShow,
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	factory := newCommandFactory()
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

	if set.ProjectRoot == "" {
		fmt.Fprintln(os.Stdout, "Scope: legacy global projection")
	} else {
		fmt.Fprintf(os.Stdout, "Scope: %s\n", set.ProjectRoot)
	}

	fmt.Fprintf(os.Stdout, "Skills: %d resolved\n", len(set.Skills))
	for _, skill := range set.Skills {
		fmt.Fprintf(os.Stdout, "  %s\n", skill.Name)
	}
	if len(set.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "warning: missing SKILL.md for %s\n", strings.Join(set.Missing, ", "))
	}

	fmt.Fprintln(os.Stdout, "Budgets:")
	for _, agent := range budgetAgents(cfg) {
		result := budget.CheckBudget(set.Skills, agent)
		line := budget.FormatResult(result)
		if line == "" {
			continue
		}
		switch result.Status {
		case budget.StatusWarn:
			fmt.Fprintf(os.Stdout, "  %s — warning\n", line)
		case budget.StatusRefuse:
			fmt.Fprintf(os.Stdout, "  %s — exceeded\n", line)
		default:
			fmt.Fprintf(os.Stdout, "  %s\n", line)
		}
	}
	return nil
}
