package workflow

import (
	"fmt"
	"io"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

// textFormatter writes human-readable output to stdout/stderr as events arrive.
type textFormatter struct {
	out           io.Writer
	errOut        io.Writer
	multiRegistry bool
	totalSummary  sync.SyncCompleteMsg
}

func newTextFormatter(out, errOut io.Writer, multiRegistry bool) *textFormatter {
	return &textFormatter{
		out:           out,
		errOut:        errOut,
		multiRegistry: multiRegistry,
	}
}

func (f *textFormatter) OnRegistryStart(repo string) {
	if f.multiRegistry {
		fmt.Fprintf(f.errOut, "── %s ──\n", repo)
	} else {
		fmt.Fprintf(f.errOut, "syncing %s...\n\n", repo)
	}
}

func (f *textFormatter) OnSkillResolved(_ string, _ sync.SkillStatus) {
	// Text mode doesn't need the resolved event — it prints as actions happen.
}

func (f *textFormatter) OnSkillDownloading(name string) {
	fmt.Fprintf(f.out, "  %-20s downloading...\n", name)
}

func (f *textFormatter) OnSkillInstalled(name string, updated bool) {
	verb := "installed"
	if updated {
		verb = "updated"
	}
	fmt.Fprintf(f.out, "  %-20s %s\n", name, verb)
}

func (f *textFormatter) OnSkillSkipped(name string, status sync.SkillStatus) {
	ver := ""
	if status.Installed != nil {
		ver = status.Installed.DisplayVersion()
	}
	fmt.Fprintf(f.out, "  %-20s ok (%s)\n", name, ver)
}

func (f *textFormatter) OnSkillError(name string, err error) {
	fmt.Fprintf(f.errOut, "  %-20s error: %v\n", name, err)
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
	fmt.Fprintf(f.errOut, "  %-20s requires approval\n", name)
	fmt.Fprintf(f.errOut, "    source:  %s\n", source)
	fmt.Fprintf(f.errOut, "    command: %s\n", command)
}

func (f *textFormatter) OnPackageApproved(name string) {
	fmt.Fprintf(f.out, "  %-20s approved\n", name)
}

func (f *textFormatter) OnPackageDenied(name string) {
	fmt.Fprintf(f.out, "  %-20s denied, skipping\n", name)
}

func (f *textFormatter) OnPackageSkipped(name, reason string) {
	fmt.Fprintf(f.out, "  %-20s skipped (%s)\n", name, reason)
}

func (f *textFormatter) OnPackageInstalling(name string) {
	fmt.Fprintf(f.out, "  %-20s installing...\n", name)
}

func (f *textFormatter) OnPackageInstalled(name string) {
	fmt.Fprintf(f.out, "  %-20s installed\n", name)
}

func (f *textFormatter) OnPackageUpdating(name string) {
	fmt.Fprintf(f.out, "  %-20s updating...\n", name)
}

func (f *textFormatter) OnPackageUpdated(name string) {
	fmt.Fprintf(f.out, "  %-20s updated\n", name)
}

func (f *textFormatter) OnPackageError(name string, err error, stderr string) {
	msg := err.Error()
	if stderr != "" {
		msg += ": " + stderr
	}
	fmt.Fprintf(f.errOut, "  %-20s error: %s\n", name, msg)
}

func (f *textFormatter) OnPackageHashMismatch(name, oldCmd, newCmd, source string) {
	fmt.Fprintf(f.errOut, "  %-20s command changed:\n", name)
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
	fmt.Fprintf(f.out, "  %-20s adopted\n", name)
}

func (f *textFormatter) OnAdoptionError(name string, err error) {
	fmt.Fprintf(f.errOut, "  %-20s adoption error: %v\n", name, err)
}

func (f *textFormatter) OnAdoptionConflictsDeferred(count int) {
	fmt.Fprintf(f.errOut, "  %d conflict(s) skipped — run `scribe adopt` to resolve\n", count)
}

func (f *textFormatter) OnAdoptionComplete(adopted, _, failed int) {
	if adopted == 0 && failed == 0 {
		return
	}
	if failed > 0 {
		fmt.Fprintf(f.out, "adopted %d skill(s), %d failed\n", adopted, failed)
		return
	}
	fmt.Fprintf(f.out, "adopted %d skill(s)\n", adopted)
}

func (f *textFormatter) Flush() error {
	fmt.Fprintf(f.out, "\ndone: %d installed, %d updated, %d current, %d failed\n",
		f.totalSummary.Installed, f.totalSummary.Updated,
		f.totalSummary.Skipped, f.totalSummary.Failed)
	return nil
}
