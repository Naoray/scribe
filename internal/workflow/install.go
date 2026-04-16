package workflow

import (
	"context"
	"fmt"
	"os"
	"sort"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/sync"
)

// InstallSteps returns the step list for the install command.
func InstallSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"CheckConnected", StepCheckConnected},
		{"FilterRegistries", StepFilterRegistries},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"SelectSkills", StepSelectSkills},
		{"SyncSkills", StepSyncSkills},
	}
}

// StepSelectSkills resolves which skills to install and sets b.SkillFilter.
//
//   - Skill names in b.Args: install exactly those skills.
//   - b.InstallAllFlag: install everything available (no filter).
//   - Interactive TTY: show a multi-select picker of available skills.
//   - Non-TTY, no args, no --all: error with a hint.
func StepSelectSkills(ctx context.Context, b *Bag) error {
	// Explicit names — use them directly without fetching the full catalog.
	if len(b.Args) > 0 {
		b.SkillFilter = b.Args
		return nil
	}

	// --all: no filter; syncer installs every StatusMissing skill.
	if b.InstallAllFlag {
		return nil
	}

	// No args — need to fetch what's available.
	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
	if !isTTY {
		return fmt.Errorf("no skill names given — pass skill names, --all, or run in a terminal for interactive selection")
	}

	// Collect StatusMissing skills across all connected registries.
	type option struct {
		name     string
		registry string
	}
	seen := map[string]bool{}
	var available []option

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
	}

	for _, repo := range b.Repos {
		statuses, _, err := syncer.Diff(ctx, repo, b.State)
		if err != nil {
			// Non-fatal: skip unreachable registries, user can fix and re-run.
			fmt.Fprintf(os.Stderr, "warning: could not reach %s: %v\n", repo, err)
			continue
		}
		for _, sk := range statuses {
			if sk.Status == sync.StatusMissing && !seen[sk.Name] {
				seen[sk.Name] = true
				available = append(available, option{name: sk.Name, registry: repo})
			}
		}
	}

	if len(available) == 0 {
		fmt.Fprintln(os.Stdout, "All available skills are already installed.")
		return errSkip
	}

	// Sort alphabetically for a stable picker list.
	sort.Slice(available, func(i, j int) bool {
		return available[i].name < available[j].name
	})

	opts := make([]huh.Option[string], len(available))
	for i, o := range available {
		opts[i] = huh.NewOption(o.name, o.name)
	}

	var selected []string
	err := huh.NewMultiSelect[string]().
		Title(fmt.Sprintf("Select skills to install (%d available)", len(available))).
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		return errSkip
	}

	b.SkillFilter = selected
	return nil
}
