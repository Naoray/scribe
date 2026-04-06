package workflow

import (
	"fmt"
	"io"

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

func (f *textFormatter) OnSkillInstalled(name string, version string, updated bool) {
	verb := "installed"
	if updated {
		verb = "updated to"
	}
	fmt.Fprintf(f.out, "  %-20s %s %s\n", name, verb, version)
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

func (f *textFormatter) Flush() error {
	fmt.Fprintf(f.out, "\ndone: %d installed, %d updated, %d current, %d failed\n",
		f.totalSummary.Installed, f.totalSummary.Updated,
		f.totalSummary.Skipped, f.totalSummary.Failed)
	return nil
}
