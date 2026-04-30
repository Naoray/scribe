package workflow

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// ListLoadStepsLocal returns the minimal local-only step list needed before
// launching the list TUI.
func ListLoadStepsLocal() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"EnsureScribeAgent", StepEnsureScribeAgent},
	}
}

// ListLoadStepsRemote returns the remote list setup path, including tool
// resolution for in-place actions on remote rows.
func ListLoadStepsRemote() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"ResolveTools", StepResolveTools},
		{"EnsureScribeAgent", StepEnsureScribeAgent},
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
	out, stateDirty, err := BuildListJSONData(ctx, b)
	if stateDirty {
		b.MarkStateDirty()
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ListOutput is the legacy payload shape for `scribe list --json`.
type ListOutput map[string]any

type ListLocalSkillJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Package     string   `json:"package,omitempty"`
	Revision    int      `json:"revision,omitempty"`
	ContentHash string   `json:"content_hash,omitempty"`
	Targets     []string `json:"targets"`
	Managed     bool     `json:"managed"`
	Origin      string   `json:"origin,omitempty"`
	Path        string   `json:"path,omitempty"`
}

type ListLocalPackageJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Revision    int      `json:"revision,omitempty"`
	Path        string   `json:"path,omitempty"`
	InstallCmd  string   `json:"install_cmd,omitempty"`
	Sources     []string `json:"sources,omitempty"`
}

type ListRemoteSkillJSON struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Version    string   `json:"version,omitempty"`
	LoadoutRef string   `json:"loadout_ref,omitempty"`
	Maintainer string   `json:"maintainer,omitempty"`
	Agents     []string `json:"agents,omitempty"`
}

type ListRegistryJSON struct {
	Registry string                `json:"registry"`
	Skills   []ListRemoteSkillJSON `json:"skills"`
}

func BuildListJSONData(ctx context.Context, b *Bag) (ListOutput, bool, error) {
	// Local view is the default; --remote switches to registry diff.
	if !b.RemoteFlag {
		skills, err := discovery.OnDisk(b.State)
		if err != nil {
			return ListOutput{}, false, err
		}
		return buildLocalListJSON(skills, b.State), false, nil
	}

	if err := StepFilterRegistries(ctx, b); err != nil {
		return ListOutput{}, false, err
	}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
		Tools:    []tools.Tool{},
	}

	return buildMultiListJSON(ctx, b.Repos, syncer, b.State)
}

func printLocalJSON(w io.Writer, skills []discovery.Skill, st *state.State) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildLocalListJSON(skills, st))
}

func buildLocalListJSON(skills []discovery.Skill, st *state.State) ListOutput {
	skillsOut := make([]ListLocalSkillJSON, 0, len(skills))
	// Emit one entry per discovered skill that is NOT tracked as a package.
	// Package entries come from the state map because discovery walks
	// ~/.scribe/skills/ and tool dirs, neither of which now hold packages.
	for _, sk := range skills {
		// A sub-skill whose Package name matches an installed package entry
		// should not surface as a standalone skill — the package owns it.
		if sk.Package != "" {
			if inst, ok := st.Installed[sk.Package]; ok && inst.IsPackage() {
				continue
			}
		}
		if inst, ok := st.Installed[sk.Name]; ok && inst.IsPackage() {
			continue
		}
		targets := sk.Targets
		if targets == nil {
			targets = []string{}
		}

		var origin string
		if installed, ok := st.Installed[sk.Name]; ok && installed.Origin == state.OriginLocal {
			origin = "local"
		}

		skillsOut = append(skillsOut, ListLocalSkillJSON{
			Name:        sk.Name,
			Description: sk.Description,
			Package:     sk.Package,
			Revision:    sk.Revision,
			ContentHash: sk.ContentHash,
			Targets:     targets,
			Managed:     sk.Managed,
			Origin:      origin,
			Path:        sk.LocalPath,
		})
	}

	packagesOut := make([]ListLocalPackageJSON, 0)
	for name, inst := range st.Installed {
		if !inst.IsPackage() {
			continue
		}
		pkgsDir, _ := stateInstalledPackageDir(name)
		srcRegistries := make([]string, 0, len(inst.Sources))
		for _, s := range inst.Sources {
			if s.Registry != "" {
				srcRegistries = append(srcRegistries, s.Registry)
			}
		}
		packagesOut = append(packagesOut, ListLocalPackageJSON{
			Name:       name,
			Revision:   inst.Revision,
			Path:       pkgsDir,
			InstallCmd: inst.InstallCmd,
			Sources:    srcRegistries,
		})
	}

	return ListOutput{
		"skills":   skillsOut,
		"packages": packagesOut,
	}
}

// stateInstalledPackageDir resolves the canonical package directory for a
// given package name. Returns the empty string if the packages root cannot
// be resolved (we still emit the entry).
func stateInstalledPackageDir(name string) (string, error) {
	pkgs, err := paths.PackagesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(pkgs, name), nil
}

func printMultiListJSON(ctx context.Context, w io.Writer, repos []string, syncer *sync.Syncer, st *state.State) (bool, error) {
	out, stateDirty, err := buildMultiListJSON(ctx, repos, syncer, st)
	if err != nil {
		return stateDirty, err
	}
	return stateDirty, json.NewEncoder(w).Encode(out)
}

func buildMultiListJSON(ctx context.Context, repos []string, syncer *sync.Syncer, st *state.State) (ListOutput, bool, error) {
	var registries []ListRegistryJSON
	var warnings []string
	stateDirty := false

	for _, teamRepo := range repos {
		if st.RegistryFailure(teamRepo).Muted {
			continue
		}
		statuses, _, err := syncer.Diff(ctx, teamRepo, st)
		if err != nil {
			failure, changed := st.RecordRegistryFailure(teamRepo, err, registryMuteAfter)
			stateDirty = stateDirty || changed
			if !failure.Muted {
				warnings = append(warnings, teamRepo+": "+err.Error())
			}
			continue
		}
		stateDirty = stateDirty || st.ClearRegistryFailure(teamRepo)

		skills := make([]ListRemoteSkillJSON, 0, len(statuses))
		for _, sk := range statuses {
			ver := ""
			var agents []string
			if sk.Installed != nil {
				ver = sk.Installed.DisplayVersion()
				agents = sk.Installed.Tools
			}
			skills = append(skills, ListRemoteSkillJSON{
				Name:       sk.Name,
				Status:     sk.Status.String(),
				Version:    ver,
				LoadoutRef: sk.LoadoutRef,
				Maintainer: sk.Maintainer,
				Agents:     agents,
			})
		}

		registries = append(registries, ListRegistryJSON{
			Registry: teamRepo,
			Skills:   skills,
		})
	}

	out := ListOutput{"registries": registries}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return out, stateDirty, nil
}

func CountStatuses(statuses []sync.SkillStatus) map[sync.Status]int {
	m := map[sync.Status]int{}
	for _, sk := range statuses {
		m[sk.Status]++
	}
	return m
}
