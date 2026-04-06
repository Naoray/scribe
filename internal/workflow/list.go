package workflow

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// ListSteps returns the step list for the list command.
func ListSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"BranchLocalOrRemote", StepBranchLocalOrRemote},
	}
}

// StepBranchLocalOrRemote handles both the local-only view and the
// remote diff view, keeping the workflow linear.
func StepBranchLocalOrRemote(ctx context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	// Local view: explicit --local flag or no registries connected.
	if b.LocalFlag || len(b.Config.TeamRepos()) == 0 {
		return listLocal(w, b, useJSON)
	}

	// Reuse shared steps for filtering.
	if err := StepFilterRegistries(ctx, b); err != nil {
		return err
	}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
		Tools:    []tools.Tool{},
	}

	if useJSON {
		return printMultiListJSON(ctx, w, b.Repos, syncer, b.State)
	}

	// Populate results on Bag for cmd/ to render.
	b.MultiRegistry = len(b.Repos) > 1
	b.RegistryDiffs = make(map[string][]sync.SkillStatus, len(b.Repos))
	for _, teamRepo := range b.Repos {
		statuses, _, err := syncer.Diff(ctx, teamRepo, b.State)
		if err != nil {
			return err
		}
		b.RegistryDiffs[teamRepo] = statuses
	}
	return nil
}

func listLocal(w io.Writer, b *Bag, useJSON bool) error {
	skills, err := discovery.OnDisk(b.State)
	if err != nil {
		return err
	}

	if useJSON {
		return printLocalJSON(w, skills, b.State)
	}
	if b.ListTUI != nil {
		return b.ListTUI(skills)
	}
	// Populate results on Bag for cmd/ to render.
	b.LocalSkills = skills
	return nil
}

func printLocalJSON(w io.Writer, skills []discovery.Skill, st *state.State) error {
	type localSkillJSON struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Version     string   `json:"version"`
		ContentHash string   `json:"content_hash,omitempty"`
		Source      string   `json:"source"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Path        string   `json:"path,omitempty"`
	}

	out := make([]localSkillJSON, 0, len(skills))
	for _, sk := range skills {
		tgts := sk.Targets
		if tgts == nil {
			tgts = []string{}
		}

		_, managed := st.Installed[sk.Name]

		out = append(out, localSkillJSON{
			Name:        sk.Name,
			Description: sk.Description,
			Package:     sk.Package,
			Version:     sk.Version,
			ContentHash: sk.ContentHash,
			Source:      sk.Source,
			Targets:     tgts,
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
