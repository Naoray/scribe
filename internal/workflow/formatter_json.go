package workflow

import (
	"io"

	clienv "github.com/Naoray/scribe/internal/cli/env"
	"github.com/Naoray/scribe/internal/cli/envelope"
	"github.com/Naoray/scribe/internal/cli/output"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

// jsonFormatter buffers all events and writes a single JSON object on Flush.
type jsonFormatter struct {
	registries []registryResult
	current    *registryResult
	summary    sync.SyncCompleteMsg
	denied     []denyListSkip
	adoption   adoptionResult
	reconcile  *reconcileResult
	meta       func() envelope.Meta
	renderer   output.Renderer
}

type reconcileResult struct {
	Installed int                        `json:"installed"`
	Relinked  int                        `json:"relinked"`
	Removed   int                        `json:"removed"`
	Conflicts []state.ProjectionConflict `json:"conflicts,omitempty"`
}

type adoptedSkill struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools,omitempty"`
	Error string   `json:"error,omitempty"`
}

type adoptionResult struct {
	Skipped   string         `json:"skipped,omitempty"`
	Skills    []adoptedSkill `json:"skills,omitempty"`
	Conflicts int            `json:"conflicts_deferred,omitempty"`
	Adopted   int            `json:"adopted,omitempty"`
	Failed    int            `json:"failed,omitempty"`
	active    bool
}

type skillResult struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Status  string `json:"status,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type denyListSkip struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
}

type registryResult struct {
	Registry string        `json:"registry"`
	Skills   []skillResult `json:"skills"`
}

func newJSONFormatter(out io.Writer, meta func() envelope.Meta) *jsonFormatter {
	if meta == nil {
		meta = func() envelope.Meta { return envelope.Meta{} }
	}
	renderer := output.New(clienv.Mode{Format: clienv.FormatJSON}, out, io.Discard)
	return &jsonFormatter{registries: []registryResult{}, meta: meta, renderer: renderer}
}

func (f *jsonFormatter) OnRegistryStart(repo string) {
	f.current = &registryResult{Registry: repo}
}

func (f *jsonFormatter) OnSkillResolved(_ string, _ sync.SkillStatus) {
	// JSON mode doesn't need the resolved event.
}

func (f *jsonFormatter) OnSkillDownloading(_ string) {
	// JSON mode doesn't emit progress.
}

func (f *jsonFormatter) OnSkillInstalled(name string, updated bool) {
	if f.current == nil {
		return
	}
	action := "installed"
	if updated {
		action = "updated"
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: action,
	})
}

func (f *jsonFormatter) OnSkillSkipped(name string, status sync.SkillStatus) {
	if f.current == nil {
		return
	}
	ver := ""
	if status.Installed != nil {
		ver = status.Installed.DisplayVersion()
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:    name,
		Action:  "skipped",
		Status:  status.Status.String(),
		Version: ver,
	})
}

func (f *jsonFormatter) OnSkillSkippedByDenyList(name, registry string) {
	f.denied = append(f.denied, denyListSkip{Name: name, Registry: registry})
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "skipped",
		Status: "removed_by_user",
	})
}

func (f *jsonFormatter) OnSkillError(name string, err error) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "error",
		Error:  err.Error(),
	})
}

func (f *jsonFormatter) OnBudgetWarning(_, _ string) {}

func (f *jsonFormatter) OnSyncComplete(summary sync.SyncCompleteMsg) {
	f.summary.Installed += summary.Installed
	f.summary.Updated += summary.Updated
	f.summary.Skipped += summary.Skipped
	f.summary.Failed += summary.Failed

	if f.current != nil {
		f.registries = append(f.registries, *f.current)
		f.current = nil
	}
}

func (f *jsonFormatter) OnReconcileConflict(_ string, conflict state.ProjectionConflict) {
	if f.reconcile == nil {
		f.reconcile = &reconcileResult{}
	}
	f.reconcile.Conflicts = append(f.reconcile.Conflicts, conflict)
}

func (f *jsonFormatter) OnReconcileComplete(msg sync.ReconcileCompleteMsg) {
	if f.reconcile == nil {
		f.reconcile = &reconcileResult{}
	}
	f.reconcile.Installed += msg.Summary.Installed
	f.reconcile.Relinked += msg.Summary.Relinked
	f.reconcile.Removed += msg.Summary.Removed
}

func (f *jsonFormatter) OnPackageInstallPrompt(name, command, source string) {}

func (f *jsonFormatter) OnPackageApproved(name string) {}

func (f *jsonFormatter) OnPackageDenied(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "denied",
	})
}

func (f *jsonFormatter) OnPackageSkipped(name, reason string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "skipped",
		Status: reason,
	})
}

func (f *jsonFormatter) OnPackageInstalling(name string) {}

func (f *jsonFormatter) OnPackageInstalled(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "package_installed",
	})
}

func (f *jsonFormatter) OnPackageUpdating(name string) {}

func (f *jsonFormatter) OnPackageUpdated(name string) {
	if f.current == nil {
		return
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "package_updated",
	})
}

func (f *jsonFormatter) OnPackageError(name string, err error, stderr string) {
	if f.current == nil {
		return
	}
	errMsg := err.Error()
	if stderr != "" {
		errMsg += ": " + stderr
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:   name,
		Action: "error",
		Error:  errMsg,
	})
}

func (f *jsonFormatter) OnPackageHashMismatch(name, oldCmd, newCmd, source string) {}

func (f *jsonFormatter) OnAdoptionSkipped(reason string) {
	f.adoption.Skipped = reason
}

func (f *jsonFormatter) OnAdoptionStarted(_ int) {
	f.adoption.active = true
}

func (f *jsonFormatter) OnAdopted(name string, targetTools []string) {
	f.adoption.Skills = append(f.adoption.Skills, adoptedSkill{Name: name, Tools: targetTools})
}

func (f *jsonFormatter) OnAdoptionError(name string, err error) {
	f.adoption.Skills = append(f.adoption.Skills, adoptedSkill{Name: name, Error: err.Error()})
}

func (f *jsonFormatter) OnAdoptionConflictsDeferred(count int) {
	f.adoption.Conflicts = count
}

func (f *jsonFormatter) OnAdoptionComplete(adopted, _, failed int) {
	f.adoption.Adopted = adopted
	f.adoption.Failed = failed
}

func (f *jsonFormatter) OnConnectDuplicate(_ string) {
	// JSON mode: handled by caller via exit code / structured output.
}

func (f *jsonFormatter) OnConnectSaved(_ string) {
	// JSON mode: connection status is implicit in the response.
}

func (f *jsonFormatter) OnConnectSyncing() {
	// JSON mode: no progress output.
}

func (f *jsonFormatter) OnConnectSyncWarning(_ string, _ error) {
	// JSON mode: sync warnings are not emitted as JSON yet.
}

func (f *jsonFormatter) OnConnectAvailable(_ string, _ int) {
	// JSON mode: available skill count is not emitted.
}

func (f *jsonFormatter) OnLegacyFormat(_ string) {}

func (f *jsonFormatter) Flush() error {
	out := map[string]any{
		"registries": f.registries,
		"summary": map[string]int{
			"installed": f.summary.Installed,
			"updated":   f.summary.Updated,
			"skipped":   f.summary.Skipped,
			"failed":    f.summary.Failed,
		},
	}
	if f.adoption.active || f.adoption.Skipped != "" || f.adoption.Conflicts > 0 {
		out["adoption"] = f.adoption
	}
	if f.reconcile != nil {
		out["reconcile"] = f.reconcile
	}
	if len(f.denied) > 0 {
		out["skipped_by_deny_list"] = f.denied
	}
	status := envelope.StatusOK
	if f.summary.Failed > 0 || f.adoption.Failed > 0 {
		status = envelope.StatusPartialSuccess
	}
	meta := f.meta()
	f.renderer.SetMeta("duration_ms", meta.DurationMS)
	f.renderer.SetMeta("bootstrap_ms", meta.BootstrapMS)
	f.renderer.SetMeta("command", meta.Command)
	f.renderer.SetMeta("scribe_version", meta.ScribeVersion)
	f.renderer.SetStatus(status)
	if err := f.renderer.Result(out); err != nil {
		return err
	}
	return f.renderer.Flush()
}
