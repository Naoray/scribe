package add

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/targets"
)

// Candidate represents a skill that can be added to a registry.
type Candidate struct {
	Name      string // skill name (directory basename)
	Origin    string // "local" or "registry:owner/repo"
	Source    string // "github:owner/repo@ref" or empty for local-only
	LocalPath string // absolute path on disk, empty for remote-only
}

// NeedsUpload reports whether this candidate requires uploading files to the
// registry (as opposed to just adding a source reference to scribe.toml).
func (c Candidate) NeedsUpload() bool {
	return c.Source == "" && c.LocalPath != ""
}

// Adder wires discovery and GitHub push together.
// Emits events via the Emit callback — the caller decides output format.
type Adder struct {
	Client  *gh.Client
	Targets []targets.Target
	Emit    func(any)
}

func (a *Adder) emit(msg any) {
	if a.Emit != nil {
		a.Emit(msg)
	}
}

// DiscoverLocal scans ~/.claude/skills/ and ~/.scribe/skills/ for skills on disk.
// Cross-references state for source info. Deduplicates by name (first seen wins).
func (a *Adder) DiscoverLocal(st *state.State) ([]Candidate, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	dirs := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".scribe", "skills"),
	}

	seen := map[string]bool{}
	var candidates []Candidate

	for _, base := range dirs {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", base, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}

			skillDir := filepath.Join(base, name)
			empty, err := isDirEmpty(skillDir)
			if err != nil || empty {
				continue
			}

			seen[name] = true

			c := Candidate{
				Name:      name,
				Origin:    "local",
				LocalPath: skillDir,
			}
			if installed, ok := st.Installed[name]; ok && installed.Source != "" {
				c.Source = installed.Source
			}

			candidates = append(candidates, c)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates, nil
}

// DiscoverRemote finds skills in other registries that are not in the target registry.
// Takes pre-fetched manifests to keep GitHub calls in the cmd layer.
func (a *Adder) DiscoverRemote(targetManifest *manifest.Manifest, otherManifests map[string]*manifest.Manifest) []Candidate {
	var candidates []Candidate

	for registry, m := range otherManifests {
		for name, skill := range m.Skills {
			// Skip if already in target registry.
			if _, exists := targetManifest.Skills[name]; exists {
				continue
			}

			candidates = append(candidates, Candidate{
				Name:   name,
				Origin: "registry:" + registry,
				Source: skill.Source,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates
}

// isDirEmpty reports whether a directory has no files (ignoring subdirectories).
func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			return false, nil
		}
	}
	// Check subdirectories recursively.
	for _, e := range entries {
		if e.IsDir() {
			empty, err := isDirEmpty(filepath.Join(dir, e.Name()))
			if err != nil {
				return false, err
			}
			if !empty {
				return false, nil
			}
		}
	}
	return true, nil
}
