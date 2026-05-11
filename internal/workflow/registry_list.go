package workflow

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/config"
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

	registries := b.Config.EnabledRegistries()
	repos := registryRepos(registries)

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
		return PrintRegistryJSON(w, registries, b.State)
	}

	// Populate Bag for cmd/ to render.
	b.RegistryRepos = repos
	b.RegistryConfigs = registries
	b.RegistryCounts = counts
	return nil
}

type RegistryJSON struct {
	Registry   string `json:"registry"`
	Visibility string `json:"visibility"`
	SkillCount int    `json:"skill_count"`
}

type RegistryListJSON struct {
	Registries []RegistryJSON `json:"registries"`
	LastSync   *string        `json:"last_sync"`
}

// PrintRegistryJSON writes registry list as JSON to w.
func PrintRegistryJSON(w io.Writer, registries []config.RegistryConfig, st *state.State) error {
	out := BuildRegistryListJSON(registries, st)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func BuildRegistryListJSON(registries []config.RegistryConfig, st *state.State) RegistryListJSON {
	repos := registryRepos(registries)
	counts := CountSkillsPerRegistry(repos, st)

	entries := make([]RegistryJSON, 0, len(registries))
	for _, registry := range registries {
		registry.Normalize()
		entries = append(entries, RegistryJSON{
			Registry:   registry.Repo,
			Visibility: registry.Visibility,
			SkillCount: counts[registry.Repo],
		})
	}

	var lastSync *string
	if !st.LastSync.IsZero() {
		s := st.LastSync.UTC().Format("2006-01-02T15:04:05Z")
		lastSync = &s
	}

	out := RegistryListJSON{
		Registries: entries,
		LastSync:   lastSync,
	}

	// Ensure empty slice renders as [] not null.
	if out.Registries == nil {
		out.Registries = []RegistryJSON{}
	}

	return out
}

func registryRepos(registries []config.RegistryConfig) []string {
	repos := make([]string, 0, len(registries))
	for _, registry := range registries {
		repos = append(repos, registry.Repo)
	}
	return repos
}
