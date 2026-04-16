package workflow

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/state"
)

// RegistryListSteps returns the step list for the registry list command.
func RegistryListSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"PrintRegistryList", StepPrintRegistryList},
	}
}

// CountSkillsPerRegistry counts installed skills per registry by inspecting
// the Sources field of each installed skill.
func CountSkillsPerRegistry(repos []string, st *state.State) map[string]int {
	counts := make(map[string]int, len(repos))
	for _, repo := range repos {
		counts[repo] = 0
	}
	for _, skill := range st.Installed {
		for _, src := range skill.Sources {
			if _, ok := counts[src.Registry]; ok {
				counts[src.Registry]++
				break
			}
		}
	}
	return counts
}

// StepPrintRegistryList computes connected registry data for rendering.
// For JSON output it writes directly; for styled output it populates Bag
// fields for the cmd/ layer to render.
func StepPrintRegistryList(_ context.Context, b *Bag) error {
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	w := os.Stdout

	repos := b.Config.TeamRepos()

	if len(repos) == 0 {
		if useJSON {
			return PrintRegistryJSON(w, nil, b.State)
		}
		// Populate empty slice so cmd/ knows to render the empty state.
		b.RegistryRepos = []string{}
		return nil
	}

	counts := CountSkillsPerRegistry(repos, b.State)

	if useJSON {
		return PrintRegistryJSON(w, repos, b.State)
	}

	// Populate Bag for cmd/ to render.
	b.RegistryRepos = repos
	b.RegistryCounts = counts
	return nil
}

type regJSON struct {
	Registry   string `json:"registry"`
	SkillCount int    `json:"skill_count"`
}

type regListJSON struct {
	Registries []regJSON `json:"registries"`
	LastSync   *string   `json:"last_sync"`
}

// PrintRegistryJSON writes registry list as JSON to w.
func PrintRegistryJSON(w io.Writer, repos []string, st *state.State) error {
	counts := CountSkillsPerRegistry(repos, st)

	entries := make([]regJSON, 0, len(repos))
	for _, repo := range repos {
		entries = append(entries, regJSON{
			Registry:   repo,
			SkillCount: counts[repo],
		})
	}

	var lastSync *string
	if !st.LastSync.IsZero() {
		s := st.LastSync.UTC().Format("2006-01-02T15:04:05Z")
		lastSync = &s
	}

	out := regListJSON{
		Registries: entries,
		LastSync:   lastSync,
	}

	// Ensure empty slice renders as [] not null.
	if out.Registries == nil {
		out.Registries = []regJSON{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
