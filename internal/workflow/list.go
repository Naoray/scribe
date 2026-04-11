package workflow

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// ListLoadSteps returns the minimal step list needed before launching the
// list TUI: it loads config/state, resolves active tools for in-place updates,
// then leaves rendering to cmd/.
func ListLoadSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"ResolveTools", StepResolveTools},
	}
}

// ListJSONSteps returns the step list for `scribe list --json` (or any
// non-TTY invocation): it loads, branches local-vs-remote, and writes JSON
// to stdout. Used only for the machine-readable output path.
func ListJSONSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"WriteListJSON", StepWriteListJSON},
	}
}

// StepWriteListJSON emits the JSON form of the list command. It mirrors
// the loader logic the TUI runs (in cmd/) but writes structured output to
// stdout instead of rendering a TUI.
func StepWriteListJSON(ctx context.Context, b *Bag) error {
	w := os.Stdout

	// Local view is the default; --remote switches to registry diff.
	if !b.RemoteFlag {
		skills, err := discovery.OnDisk(b.State)
		if err != nil {
			return err
		}
		return printLocalJSON(w, skills, b.State)
	}

	if err := StepFilterRegistries(ctx, b); err != nil {
		return err
	}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
		Tools:    []tools.Tool{},
	}

	return printMultiListJSON(ctx, w, b.Repos, syncer, b.State)
}

func printLocalJSON(w io.Writer, skills []discovery.Skill, st *state.State) error {
	type localSkillJSON struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Revision    int      `json:"revision,omitempty"`
		ContentHash string   `json:"content_hash,omitempty"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	out := make([]localSkillJSON, 0, len(skills))
	for _, sk := range skills {
		targets := sk.Targets
		if targets == nil {
			targets = []string{}
		}

		_, managed := st.Installed[sk.Name]

		out = append(out, localSkillJSON{
			Name:        sk.Name,
			Description: sk.Description,
			Package:     sk.Package,
			Revision:    sk.Revision,
			ContentHash: sk.ContentHash,
			Targets:     targets,
			Managed:     managed,
			Path:        sk.LocalPath,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func printMultiListJSON(ctx context.Context, w io.Writer, repos []string, syncer *sync.Syncer, st *state.State) error {
	type skillJSON struct {
		Name       string   `json:"name"`
		Status     string   `json:"status"`
		Version    string   `json:"version,omitempty"`
		LoadoutRef string   `json:"loadout_ref,omitempty"`
		Maintainer string   `json:"maintainer,omitempty"`
		Agents     []string `json:"agents,omitempty"`
	}

	type registryJSON struct {
		Registry string      `json:"registry"`
		Skills   []skillJSON `json:"skills"`
	}

	var registries []registryJSON

	for _, teamRepo := range repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			return err
		}

		skills := make([]skillJSON, 0, len(statuses))
		for _, sk := range statuses {
			ver := ""
			var agents []string
			if sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
				agents = sk.Installed.Tools
			}
			skills = append(skills, skillJSON{
				Name:       sk.Name,
				Status:     sk.Status.String(),
				Version:    ver,
				LoadoutRef: sk.LoadoutRef,
				Maintainer: sk.Maintainer,
				Agents:     agents,
			})
		}

		registries = append(registries, registryJSON{
			Registry: teamRepo,
			Skills:   skills,
		})
	}

	return json.NewEncoder(w).Encode(map[string]any{
		"registries": registries,
	})
}

func CountStatuses(statuses []sync.SkillStatus) map[sync.Status]int {
	m := map[sync.Status]int{}
	for _, sk := range statuses {
		m[sk.Status]++
	}
	return m
}
