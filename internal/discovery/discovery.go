package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/Naoray/scribe/internal/state"
)

// validSkillName matches safe skill names that work as TOML keys and filesystem paths.
var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Skill represents a skill found on disk, optionally enriched with state info.
type Skill struct {
	Name      string
	LocalPath string // absolute path on disk
	Source    string // from state if tracked, else empty
	Version   string // from state if tracked, else empty
	Targets   []string // from state if tracked, else inferred from location
	Managed   bool   // true if tracked in state.json
}

// OnDisk scans ~/.claude/skills/ and ~/.scribe/skills/ for skill directories.
// Cross-references state.json for version/source info on tracked skills.
// Deduplicates by name (first seen wins).
func OnDisk(st *state.State) ([]Skill, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	type scanDir struct {
		path   string
		target string // inferred target for skills found here
	}

	dirs := []scanDir{
		{filepath.Join(home, ".claude", "skills"), "claude"},
		{filepath.Join(home, ".scribe", "skills"), ""},
	}

	seen := map[string]bool{}
	var skills []Skill

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir.path, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] || !validSkillName.MatchString(name) {
				continue
			}

			skillDir := filepath.Join(dir.path, name)
			empty, err := isDirEmpty(skillDir)
			if err != nil || empty {
				continue
			}

			seen[name] = true

			sk := Skill{
				Name:      name,
				LocalPath: skillDir,
			}

			if installed, ok := st.Installed[name]; ok {
				sk.Source = installed.Source
				sk.Version = installed.DisplayVersion()
				sk.Targets = installed.Targets
				sk.Managed = true
			} else if dir.target != "" {
				sk.Targets = []string{dir.target}
			}

			skills = append(skills, sk)
		}
	}

	// Also include state-tracked skills not found on disk (e.g. removed files).
	for name, installed := range st.Installed {
		if seen[name] {
			continue
		}
		skills = append(skills, Skill{
			Name:    name,
			Source:  installed.Source,
			Version: installed.DisplayVersion(),
			Targets: installed.Targets,
			Managed: true,
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
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
