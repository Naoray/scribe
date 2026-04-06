package workflow

import (
	"encoding/json"
	"io"

	"github.com/Naoray/scribe/internal/sync"
)

// jsonFormatter buffers all events and writes a single JSON object on Flush.
type jsonFormatter struct {
	out        io.Writer
	registries []registryResult
	current    *registryResult
	summary    sync.SyncCompleteMsg
}

type skillResult struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Status  string `json:"status,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type registryResult struct {
	Registry string        `json:"registry"`
	Skills   []skillResult `json:"skills"`
}

func newJSONFormatter(out io.Writer) *jsonFormatter {
	return &jsonFormatter{out: out, registries: []registryResult{}}
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

func (f *jsonFormatter) OnSkillInstalled(name string, version string, updated bool) {
	if f.current == nil {
		return
	}
	action := "installed"
	if updated {
		action = "updated"
	}
	f.current.Skills = append(f.current.Skills, skillResult{
		Name:    name,
		Action:  action,
		Version: version,
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

func (f *jsonFormatter) Flush() error {
	return json.NewEncoder(f.out).Encode(map[string]any{
		"registries": f.registries,
		"summary": map[string]int{
			"installed": f.summary.Installed,
			"updated":   f.summary.Updated,
			"skipped":   f.summary.Skipped,
			"failed":    f.summary.Failed,
		},
	})
}
