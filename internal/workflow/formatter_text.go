package workflow

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

// skillNameWidth pads skill names to a fixed visual column so status verbs
// align across rows. Picked to match the website mockup.
const skillNameWidth = 20

// textStyles bundles the styles used in text-mode output. Lipgloss honors
// NO_COLOR / TERM=dumb at render time; for predictable test output we
// short-circuit to zero-styles when those env vars are set.
type textStyles struct {
	bold lipgloss.Style
	dim  lipgloss.Style
	ok   lipgloss.Style
	warn lipgloss.Style
}

func newTextStyles() textStyles {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return textStyles{}
	}
	return textStyles{
		bold: lipgloss.NewStyle().Bold(true),
		dim:  lipgloss.NewStyle().Faint(true),
		ok:   lipgloss.NewStyle().Foreground(lipgloss.Color("#60E890")),
		warn: lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")),
	}
}

// textFormatter writes human-readable output to stdout/stderr as events arrive.
type textFormatter struct {
	out               io.Writer
	errOut            io.Writer
	multiRegistry     bool
	totalSummary      sync.SyncCompleteMsg
	styles            textStyles
	pendingDeferred   []string // adoption conflicts buffered for the apply summary
	deferredFlushed   bool
	adoptionCompleted bool
}

func newTextFormatter(out, errOut io.Writer, multiRegistry bool) *textFormatter {
	return &textFormatter{
		out:           out,
		errOut:        errOut,
		multiRegistry: multiRegistry,
		styles:        newTextStyles(),
	}
}

func (f *textFormatter) OnSyncStart(repoCount int) {
	if repoCount <= 0 {
		return
	}
	noun := "registries"
	if repoCount == 1 {
		noun = "registry"
	}
	line := fmt.Sprintf("→ checking %d connected %s ...", repoCount, noun)
	fmt.Fprintln(f.errOut, f.styles.dim.Render(line))
	fmt.Fprintln(f.errOut)
}

func (f *textFormatter) OnRegistryStart(repo string) {
	if f.multiRegistry {
		fmt.Fprintf(f.errOut, "── %s ──\n", repo)
	} else {
		fmt.Fprintf(f.errOut, "── %s ──\n", repo)
	}
}

func (f *textFormatter) OnSkillResolved(_ string, _ sync.SkillStatus) {
	// Text mode doesn't need the resolved event — it prints as actions happen.
}

func (f *textFormatter) OnSkillDownloading(_ string) {
	// Mock target shows no transient downloading line; suppress in text mode.
}

func (f *textFormatter) OnSkillInstalled(name string, updated bool, revision int) {
	verb := "installed"
	if updated {
		verb = "updated"
	}
	rev := ""
	if revision > 0 {
		rev = fmt.Sprintf(" (rev %d)", revision)
	}
	f.writeSkillRow(name, f.styles.ok.Render(verb), f.styles.dim.Render(rev))
}

func (f *textFormatter) OnSkillSkipped(name string, status sync.SkillStatus) {
	rev := ""
	if status.Installed != nil && status.Installed.Revision > 0 {
		rev = fmt.Sprintf(" (rev %d)", status.Installed.Revision)
	}
	// "ok" + " (rev N)" both dim — render as one block so substring matching works.
	suffix := f.styles.dim.Render("ok" + rev)
	f.writeSkillRow(name, suffix, "")
}

// writeSkillRow prints `  <bold-name padded-to-skillNameWidth><status><suffix>`.
func (f *textFormatter) writeSkillRow(name, status, suffix string) {
	pad := skillNameWidth - len([]rune(name))
	if pad < 1 {
		pad = 1
	}
	fmt.Fprintf(f.out, "  %s%s%s%s\n",
		f.styles.bold.Render(name),
		strings.Repeat(" ", pad),
		status,
		suffix,
	)
}

func (f *textFormatter) OnSkillSkippedByDenyList(name, registry string) {
	fmt.Fprintf(f.out, "  %-*s skipped (removed by user from %s)\n", skillNameWidth, name, registry)
}

func (f *textFormatter) OnSkillError(name string, err error) {
	fmt.Fprintf(f.errOut, "  %-*s error: %v\n", skillNameWidth, name, err)
}

func (f *textFormatter) OnBudgetWarning(_, message string) {
	if message == "" {
		return
	}
	// Format from ibudget.FormatResult: "Codex budget: NN% (X / Y bytes)".
	fmt.Fprintf(f.errOut, "%s %s\n", f.styles.warn.Render("!"), message)
}

func (f *textFormatter) OnNameConflictResolved(conflict sync.NameConflict, resolution sync.NameConflictResolution) {
	switch resolution.Action {
	case sync.NameConflictActionAlias:
		fmt.Fprintf(f.out, "  %-*s installed as %s\n", skillNameWidth, conflict.Name, resolution.Alias)
	case sync.NameConflictActionSkip:
		fmt.Fprintf(f.out, "  %-*s skipped (name conflict)\n", skillNameWidth, conflict.Name)
	}
}

func (f *textFormatter) OnSyncComplete(summary sync.SyncCompleteMsg) {
	f.totalSummary.Installed += summary.Installed
	f.totalSummary.Updated += summary.Updated
	f.totalSummary.Skipped += summary.Skipped
	f.totalSummary.Failed += summary.Failed

	if f.multiRegistry {
		fmt.Fprintln(f.out)
	}
}

func (f *textFormatter) OnReconcileConflict(name string, conflict state.ProjectionConflict) {
	tool := conflict.Tool
	if tool == "" {
		tool = "tool"
	}
	fmt.Fprintf(f.errOut, "conflict: %s in %s differs from managed copy\n", name, tool)
	fmt.Fprintf(f.errOut, "run `scribe skill repair %s --tool %s --from managed|tool` to resolve\n", name, tool)
}

func (f *textFormatter) OnReconcileComplete(msg sync.ReconcileCompleteMsg) {
	if msg.Summary.Installed == 0 && msg.Summary.Relinked == 0 && msg.Summary.Removed == 0 && len(msg.Summary.Conflicts) == 0 {
		return
	}
	if msg.Summary.Installed+msg.Summary.Relinked+msg.Summary.Removed > 0 {
		fmt.Fprintf(f.out, "repaired %d tool installs\n", msg.Summary.Installed+msg.Summary.Relinked+msg.Summary.Removed)
	}
	if len(msg.Summary.Conflicts) > 0 {
		fmt.Fprintf(f.errOut, "%d conflict(s) skipped\n", len(msg.Summary.Conflicts))
	}
}

func (f *textFormatter) OnPackageInstallPrompt(name, command, source string) {
	fmt.Fprintf(f.errOut, "  %-*s requires approval\n", skillNameWidth, name)
	fmt.Fprintf(f.errOut, "    source:  %s\n", source)
	fmt.Fprintf(f.errOut, "    command: %s\n", command)
}

func (f *textFormatter) OnPackageApproved(name string) {
	fmt.Fprintf(f.out, "  %-*s approved\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageDenied(name string) {
	fmt.Fprintf(f.out, "  %-*s denied, skipping\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageSkipped(name, reason string) {
	fmt.Fprintf(f.out, "  %-*s skipped (%s)\n", skillNameWidth, name, reason)
}

func (f *textFormatter) OnPackageInstalling(name string) {
	fmt.Fprintf(f.out, "  %-*s installing...\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageInstalled(name string) {
	fmt.Fprintf(f.out, "  %-*s installed\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageUpdating(name string) {
	fmt.Fprintf(f.out, "  %-*s updating...\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageUpdated(name string) {
	fmt.Fprintf(f.out, "  %-*s updated\n", skillNameWidth, name)
}

func (f *textFormatter) OnPackageError(name string, err error, stderr string) {
	msg := err.Error()
	if stderr != "" {
		msg += ": " + stderr
	}
	fmt.Fprintf(f.errOut, "  %-*s error: %s\n", skillNameWidth, name, msg)
}

func (f *textFormatter) OnPackageHashMismatch(name, oldCmd, newCmd, source string) {
	fmt.Fprintf(f.errOut, "  %-*s command changed:\n", skillNameWidth, name)
	fmt.Fprintf(f.errOut, "    was: %s\n", oldCmd)
	fmt.Fprintf(f.errOut, "    now: %s\n", newCmd)
}

func (f *textFormatter) OnConnectDuplicate(repo string) {
	fmt.Fprintf(f.out, "Already connected to %s\n", repo)
}

func (f *textFormatter) OnConnectSaved(repo string) {
	fmt.Fprintf(f.out, "Connected to %s\n", repo)
}

func (f *textFormatter) OnConnectSyncing() {
	fmt.Fprintf(f.out, "\nsyncing skills...\n\n")
}

func (f *textFormatter) OnConnectSyncWarning(repo string, err error) {
	fmt.Fprintf(f.errOut, "warning: sync failed for %s: %v\n", repo, err)
	fmt.Fprintf(f.errOut, "run `scribe sync` to retry\n")
}

func (f *textFormatter) OnConnectAvailable(_ string, count int) {
	fmt.Fprintf(f.out, "%d skills available — run `scribe add` to install\n", count)
}

func (f *textFormatter) OnLegacyFormat(repo string) {
	fmt.Fprintf(f.errOut, "note: %s uses legacy scribe.toml — consider migrating to scribe.yaml\n", repo)
}

func (f *textFormatter) OnAdoptionSkipped(reason string) {
	fmt.Fprintf(f.errOut, "warning: %s\n", reason)
}

func (f *textFormatter) OnAdoptionStarted(_ int) {
	// No per-batch header — summary line in OnAdoptionComplete is sufficient.
}

func (f *textFormatter) OnAdopted(name string, _ []string) {
	fmt.Fprintf(f.out, "  %-*s adopted\n", skillNameWidth, name)
}

func (f *textFormatter) OnAdoptionError(name string, err error) {
	fmt.Fprintf(f.errOut, "  %-*s adoption error: %v\n", skillNameWidth, name, err)
}

func (f *textFormatter) OnAdoptionConflictsDeferred(names []string) {
	if len(names) == 0 {
		return
	}
	f.pendingDeferred = append(f.pendingDeferred, names...)
}

func (f *textFormatter) OnAdoptionComplete(adopted, _, failed int) {
	f.adoptionCompleted = true
	if adopted == 0 && failed == 0 && len(f.pendingDeferred) == 0 {
		return
	}
	if adopted > 0 {
		noun := "skills"
		if adopted == 1 {
			noun = "skill"
		}
		check := f.styles.ok.Render("✓")
		fmt.Fprintf(f.out, "%s adopted %s via symlink %s\n",
			check,
			f.styles.bold.Render(fmt.Sprintf("%d %s", adopted, noun)),
			f.styles.dim.Render("(originals untouched)"),
		)
	}
	if failed > 0 {
		fmt.Fprintf(f.errOut, "%d adoption(s) failed\n", failed)
	}
	f.flushDeferred()
}

func (f *textFormatter) flushDeferred() {
	if f.deferredFlushed || len(f.pendingDeferred) == 0 {
		return
	}
	f.deferredFlushed = true
	count := len(f.pendingDeferred)
	noun := "skipped"
	if count == 1 {
		noun = "skipped"
	}
	bang := f.styles.warn.Render("!")
	prefix := fmt.Sprintf("%d %s", count, noun)
	detail := strings.Join(f.pendingDeferred, ", ") + " · use --force to override"
	fmt.Fprintf(f.errOut, "%s %s %s\n",
		bang,
		f.styles.bold.Render(prefix),
		f.styles.dim.Render("("+detail+")"),
	)
}

func (f *textFormatter) Flush() error {
	// If adoption deferred names were buffered but OnAdoptionComplete never
	// fired (e.g. only-conflicts case where Apply is skipped), still surface
	// the warning so the user knows what got skipped.
	f.flushDeferred()

	// Sync summary footer — only meaningful when sync actually ran. We detect
	// "sync ran" via the totalSummary being non-zero or the multi-registry
	// flag being set; fall back to printing zeros for backwards compat with
	// existing tests that exercise the JSON path indirectly.
	fmt.Fprintf(f.out, "\ndone: %d installed, %d updated, %d current, %d failed\n",
		f.totalSummary.Installed, f.totalSummary.Updated,
		f.totalSummary.Skipped, f.totalSummary.Failed)
	return nil
}
