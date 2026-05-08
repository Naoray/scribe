package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

func newAdoptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt [<name>]",
		Short: "Import unmanaged local skills into Scribe",
		Long: `Detect skills that exist in tool-facing directories but are not yet managed
by Scribe, and import them into the canonical store (~/.scribe/skills/).

Without arguments, adopts all candidates (subject to conflict resolution).
With a name argument, adopts only the matching skill.

Examples:
  scribe adopt                  # adopt all unmanaged skills (respects config mode)
  scribe adopt commit           # adopt one skill by name
  scribe adopt --no-interaction # force auto-adopt (skip prompts)
  scribe adopt --dry-run        # preview what would be adopted
  scribe adopt --json           # machine output`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAdopt,
	}
	addNoInteractionFlag(cmd, "Force auto-adopt: adopt clean candidates, skip conflicts", true)
	cmd.Flags().Bool("dry-run", false, "Print plan without writing anything")
	cmd.Flags().Bool("force", false, "Override conflicts by replacing the managed copy with the unmanaged one")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().Bool("verbose", false, "Include paths and hashes in plan output")
	return markJSONSupported(cmd)
}

func runAdopt(cmd *cobra.Command, args []string) error {
	yes := noInteractionFlagPassed(cmd)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	jsonFlag := jsonFlagPassed(cmd)

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())

	factory := commandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Non-TTY without --no-interaction: cannot prompt. Exit with guidance.
	if !isTTY && !yes && !dryRun && !jsonFlag {
		return fmt.Errorf("adopt: non-interactive terminal detected — pass --no-interaction to force auto-adopt, or run 'scribe config adoption --mode off' to disable")
	}

	candidates, conflicts, err := adopt.FindCandidates(st, cfg.Adoption)
	if err != nil {
		return fmt.Errorf("scan for candidates: %w", err)
	}

	plan := adopt.Plan{
		Adopt:     candidates,
		Conflicts: conflicts,
	}

	// Apply name filter if positional arg given.
	if len(args) > 0 {
		name := args[0]
		plan = filterPlanByName(plan, name)
		if len(plan.Adopt) == 0 && len(plan.Conflicts) == 0 {
			return fmt.Errorf("no candidate named %q found", name)
		}
	}

	scanPaths, _ := cfg.AdoptionPaths()

	// Bypasses formatter — one-shot output, not an event stream.
	if dryRun {
		verbose, _ := cmd.Flags().GetBool("verbose")
		return printDryRun(cmd, plan, scanPaths, useJSON, verbose)
	}

	// Non-TTY + --json without --no-interaction: also print dry-run style plan.
	if useJSON && !yes {
		verbose, _ := cmd.Flags().GetBool("verbose")
		return printDryRun(cmd, plan, scanPaths, true, verbose)
	}

	resolvedTools, err := tools.ResolveActive(cfg)
	if err != nil {
		return fmt.Errorf("resolve tools: %w", err)
	}

	formatter := workflow.NewFormatterForContext(cmd.Context(), useJSON, false)

	decision := decideAdoptPlan(plan, adoptPlanOptions{
		Force: force,
		Yes:   yes,
		IsTTY: isTTY,
	})
	if len(decision.DeferredConflicts) > 0 {
		formatter.OnAdoptionConflictsDeferred(decision.DeferredConflicts)
	}

	finalCandidates := decision.Candidates
	if decision.NeedsPrompt {
		var err error
		finalCandidates, err = promptAdoptPlan(plan)
		if err != nil {
			return err
		}
	}

	if len(finalCandidates) == 0 {
		if !useJSON {
			fmt.Fprintln(os.Stderr, "Nothing to adopt.")
		}
		return nil
	}

	formatter.OnAdoptionStarted(len(finalCandidates))

	adopter := &adopt.Adopter{
		State: st,
		Tools: resolvedTools,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case adopt.AdoptedMsg:
				formatter.OnAdopted(m.Name, m.Tools)
			case adopt.AdoptErrorMsg:
				formatter.OnAdoptionError(m.Name, m.Err)
			case adopt.AdoptCompleteMsg:
				formatter.OnAdoptionComplete(m.Adopted, m.Skipped, m.Failed)
			}
		},
	}

	result := adopter.Apply(finalCandidates)

	if err := formatter.Flush(); err != nil {
		return err
	}

	if len(result.Failed) > 0 {
		return clierrors.Wrap(
			fmt.Errorf("adoption completed with %d failure(s)", len(result.Failed)),
			"ADOPT_PARTIAL",
			clierrors.ExitPartial,
			clierrors.WithRendered(true),
		)
	}

	return nil
}

type adoptPlanOptions struct {
	Force bool
	Yes   bool
	IsTTY bool
}

type adoptPlanDecision struct {
	Candidates        []adopt.Candidate
	DeferredConflicts []string
	NeedsPrompt       bool
}

func decideAdoptPlan(plan adopt.Plan, opts adoptPlanOptions) adoptPlanDecision {
	if opts.Force {
		decisions := make(map[string]adopt.Decision, len(plan.Conflicts))
		for _, c := range plan.Conflicts {
			decisions[c.Name] = adopt.DecisionOverwriteManaged
		}
		return adoptPlanDecision{Candidates: adopt.Resolve(plan, decisions)}
	}

	if opts.Yes || !opts.IsTTY {
		decision := adoptPlanDecision{
			Candidates: adopt.Resolve(plan, nil),
		}
		if len(plan.Conflicts) > 0 {
			decision.DeferredConflicts = make([]string, 0, len(plan.Conflicts))
			for _, c := range plan.Conflicts {
				decision.DeferredConflicts = append(decision.DeferredConflicts, c.Name)
			}
		}
		return decision
	}

	return adoptPlanDecision{NeedsPrompt: true}
}

// filterPlanByName returns a plan containing only the candidate or conflict with the given name.
func filterPlanByName(p adopt.Plan, name string) adopt.Plan {
	var filtered adopt.Plan
	for _, c := range p.Adopt {
		if c.Name == name {
			filtered.Adopt = append(filtered.Adopt, c)
		}
	}
	for _, c := range p.Conflicts {
		if c.Name == name {
			filtered.Conflicts = append(filtered.Conflicts, c)
		}
	}
	return filtered
}

// dryRunPlan is the JSON shape emitted by --dry-run --json.
type dryRunPlan struct {
	DryRun    bool              `json:"dry_run"`
	Adopt     []dryRunCandidate `json:"adopt"`
	Conflicts []dryRunConflict  `json:"conflicts"`
}

type dryRunCandidate struct {
	Name      string   `json:"name"`
	LocalPath string   `json:"local_path,omitempty"`
	Targets   []string `json:"targets,omitempty"`
	Hash      string   `json:"hash,omitempty"`
}

type dryRunConflict struct {
	Name          string `json:"name"`
	ManagedHash   string `json:"managed_hash,omitempty"`
	UnmanagedPath string `json:"unmanaged_path,omitempty"`
	UnmanagedHash string `json:"unmanaged_hash,omitempty"`
}

// printDryRun outputs the plan without making any writes.
func printDryRun(cmd *cobra.Command, plan adopt.Plan, scanPaths []string, useJSON, verbose bool) error {
	if useJSON {
		p := dryRunPlan{
			DryRun:    true,
			Adopt:     make([]dryRunCandidate, 0, len(plan.Adopt)),
			Conflicts: make([]dryRunConflict, 0, len(plan.Conflicts)),
		}
		for _, c := range plan.Adopt {
			entry := dryRunCandidate{Name: c.Name}
			if verbose {
				entry.LocalPath = c.LocalPath
				entry.Targets = c.Targets
				entry.Hash = c.Hash
			}
			p.Adopt = append(p.Adopt, entry)
		}
		for _, c := range plan.Conflicts {
			entry := dryRunConflict{Name: c.Name}
			if verbose {
				entry.ManagedHash = c.Managed.InstalledHash
				entry.UnmanagedPath = c.Unmanaged.LocalPath
				entry.UnmanagedHash = c.Unmanaged.Hash
			}
			p.Conflicts = append(p.Conflicts, entry)
		}
		return renderMutatorEnvelope(cmd, p, envelope.StatusOK)
	}

	writeDryRunPlan(cmd.OutOrStdout(), plan, scanPaths)
	return nil
}

// writeDryRunPlan renders the human-readable adoption plan.
func writeDryRunPlan(w io.Writer, plan adopt.Plan, scanPaths []string) {
	st := newAdoptStyles()
	home, _ := os.UserHomeDir()

	if pretty := tildifyPaths(scanPaths, home); len(pretty) > 0 {
		fmt.Fprintln(w, st.dim.Render("→ scanning "+strings.Join(pretty, ", ")))
		fmt.Fprintln(w)
	}

	if len(plan.Adopt) == 0 && len(plan.Conflicts) == 0 {
		fmt.Fprintln(w, "Nothing to adopt.")
		return
	}

	fmt.Fprintln(w, st.bold.Render("Plan:"))
	colWidth := planColumnWidth(plan)
	for _, c := range plan.Adopt {
		src := tildify(c.LocalPath, home)
		fmt.Fprintf(w, "  %s %s  %s\n",
			st.ok.Render("+"),
			st.bold.Render(padName(c.Name, colWidth)),
			st.dim.Render("from "+src),
		)
	}
	for _, c := range plan.Conflicts {
		fmt.Fprintf(w, "  %s %s  %s\n",
			st.warn.Render("!"),
			st.bold.Render(padName(c.Name, colWidth)),
			st.warn.Render("conflict (use --force)"),
		)
	}
}

func planColumnWidth(plan adopt.Plan) int {
	w := 0
	for _, c := range plan.Adopt {
		if l := len([]rune(c.Name)); l > w {
			w = l
		}
	}
	for _, c := range plan.Conflicts {
		if l := len([]rune(c.Name)); l > w {
			w = l
		}
	}
	return w
}

func padName(name string, width int) string {
	pad := width - len([]rune(name))
	if pad < 0 {
		pad = 0
	}
	return name + strings.Repeat(" ", pad)
}

func tildify(p, home string) string {
	if home == "" || p == "" {
		return p
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	if p == home {
		return "~"
	}
	return p
}

func tildifyPaths(paths []string, home string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		out = append(out, tildify(p, home))
	}
	return out
}

type adoptStyles struct {
	bold lipgloss.Style
	dim  lipgloss.Style
	ok   lipgloss.Style
	warn lipgloss.Style
}

func newAdoptStyles() adoptStyles {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return adoptStyles{}
	}
	return adoptStyles{
		bold: lipgloss.NewStyle().Bold(true),
		dim:  lipgloss.NewStyle().Faint(true),
		ok:   lipgloss.NewStyle().Foreground(lipgloss.Color("#60E890")),
		warn: lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")),
	}
}

// promptAdoptPlan shows a TTY prompt for each conflict and returns the resolved candidate list.
func promptAdoptPlan(plan adopt.Plan) ([]adopt.Candidate, error) {
	// Show summary.
	fmt.Printf("Found %d candidate(s) to adopt", len(plan.Adopt))
	if len(plan.Conflicts) > 0 {
		fmt.Printf(", %d conflict(s)", len(plan.Conflicts))
	}
	fmt.Println(".")

	// Confirm adopting clean candidates if any.
	var cleanToAdopt []adopt.Candidate
	if len(plan.Adopt) > 0 {
		var confirmed bool
		names := make([]string, len(plan.Adopt))
		for i, c := range plan.Adopt {
			names[i] = c.Name
		}
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Adopt %d clean skill(s): %s?", len(plan.Adopt), strings.Join(names, ", "))).
			Value(&confirmed).
			Run(); err != nil {
			return nil, err
		}
		if confirmed {
			cleanToAdopt = plan.Adopt
		}
	}

	// Resolve each conflict interactively.
	decisions := make(map[string]adopt.Decision, len(plan.Conflicts))
	for _, c := range plan.Conflicts {
		d, err := promptConflict(c)
		if err != nil {
			return nil, err
		}
		if d == -1 { // abort sentinel
			return nil, fmt.Errorf("aborted")
		}
		decisions[c.Name] = d
	}

	resolvedPlan := adopt.Plan{
		Adopt:     cleanToAdopt,
		Conflicts: plan.Conflicts,
	}
	return adopt.Resolve(resolvedPlan, decisions), nil
}

// promptConflict asks the user how to resolve a single conflict.
// Returns -1 (as Decision) to signal abort.
func promptConflict(c adopt.Conflict) (adopt.Decision, error) {
	const (
		optSkip      = "skip"
		optOverwrite = "overwrite"
		optReplace   = "replace"
		optDiff      = "diff"
		optAbort     = "abort"
	)

	for {
		var choice string
		err := huh.NewSelect[string]().
			Title(fmt.Sprintf("Conflict: %s\n  managed:   rev %d (hash %s)\n  unmanaged: %s",
				c.Name, c.Managed.Revision, shortHash(c.Managed.InstalledHash),
				c.Unmanaged.LocalPath)).
			Options(
				huh.NewOption("Skip (keep managed copy)", optSkip),
				huh.NewOption("Overwrite managed with unmanaged", optOverwrite),
				huh.NewOption("Replace unmanaged with managed (re-link)", optReplace),
				huh.NewOption("Show diff", optDiff),
				huh.NewOption("Abort", optAbort),
			).
			Value(&choice).
			Run()
		if err != nil {
			return adopt.DecisionSkip, err
		}

		switch choice {
		case optSkip:
			return adopt.DecisionSkip, nil
		case optOverwrite:
			return adopt.DecisionOverwriteManaged, nil
		case optReplace:
			return adopt.DecisionReplaceUnmanaged, nil
		case optAbort:
			return -1, nil
		case optDiff:
			showDiff(c)
			// loop: re-prompt after showing diff
		}
	}
}

// showDiff pipes a unified diff of the two SKILL.md files to $PAGER or less,
// falling back to inline print if no pager is available.
func showDiff(c adopt.Conflict) {
	managedPath := c.Managed.Paths
	unmanagedPath := c.Unmanaged.LocalPath + "/SKILL.md"

	// Use a managed path if available; fall back to canonical store path.
	var managed string
	if len(managedPath) > 0 {
		managed = managedPath[0]
	} else {
		storeDir, err := paths.StoreDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot show diff: store path unavailable")
			return
		}
		managed = filepath.Join(storeDir, c.Name, "SKILL.md")
	}

	diffBytes, err := exec.Command("diff", "-u", managed, unmanagedPath).Output()
	if err != nil && len(diffBytes) == 0 {
		fmt.Fprintf(os.Stderr, "diff unavailable: %v\n", err)
		return
	}

	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	parts := strings.Fields(pager)
	if len(parts) == 0 {
		fmt.Print(string(diffBytes))
		return
	}

	pagerCmd := exec.Command(parts[0], parts[1:]...)
	pagerCmd.Stdin = strings.NewReader(string(diffBytes))
	pagerCmd.Stdout = os.Stdout
	pagerCmd.Stderr = os.Stderr
	if err := pagerCmd.Run(); err != nil {
		// Pager not available; print inline.
		fmt.Print(string(diffBytes))
	}
}

// shortHash returns the first 7 chars of a hash, or the full string if shorter.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}
