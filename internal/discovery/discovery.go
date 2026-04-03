package discovery

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/state"
)

// validSkillName matches safe skill names that work as TOML keys and filesystem paths.
var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Skill represents a skill found on disk, optionally enriched with state info.
type Skill struct {
	Name        string
	Description string   // short description from SKILL.md frontmatter or first paragraph
	Package     string   // parent package name if skill is a symlink sub-skill (e.g. "gstack")
	LocalPath   string   // absolute path on disk
	Source      string   // from state if tracked, else empty
	Version     string   // from state if tracked, else empty
	Targets     []string // from state if tracked, else inferred from location
	Managed     bool     // true if tracked in state.json
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
				Name:        name,
				Description: readSkillDescription(skillDir),
				LocalPath:   skillDir,
				Package:     detectPackage(skillDir, dir.path),
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
		// Group by package (standalone first, then by package name), alpha within.
		pi, pj := skills[i].Package, skills[j].Package
		if pi != pj {
			if pi == "" {
				return true // standalone before packages
			}
			if pj == "" {
				return false
			}
			return pi < pj
		}
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

// detectPackage determines if a skill directory is a sub-skill of a parent
// package by checking if SKILL.md is a symlink pointing into another skill dir.
// For example, browse/SKILL.md -> gstack/browse/SKILL.md means package "gstack".
// Also detects whole-directory symlinks (e.g., find-skills -> ~/.agents/skills/find-skills).
func detectPackage(skillDir, scanBase string) string {
	// Check if the directory itself is a symlink.
	if target, err := os.Readlink(skillDir); err == nil {
		// Resolve relative to the scan base for relative symlinks.
		if !filepath.IsAbs(target) {
			target = filepath.Join(scanBase, target)
		}
		// Not a sibling in the same skills dir — external package.
		return ""
	}

	// Check if SKILL.md is a symlink into a sibling package.
	skillMD := filepath.Join(skillDir, "SKILL.md")
	target, err := os.Readlink(skillMD)
	if err != nil {
		return "" // not a symlink
	}

	// Resolve to absolute path.
	if !filepath.IsAbs(target) {
		target = filepath.Join(skillDir, target)
	}
	target, err = filepath.Clean(target), nil
	if err != nil {
		return ""
	}

	// Check if the target is inside a sibling dir in the same scan base.
	// Pattern: <scanBase>/<package>/<subdir>/SKILL.md
	rel, err := filepath.Rel(scanBase, target)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) < 2 {
		return "" // target is directly in scanBase, not a sub-skill
	}
	pkg := parts[0]
	if pkg == filepath.Base(skillDir) {
		return "" // points to itself
	}
	return pkg
}

// readSkillDescription extracts a short description from SKILL.md.
// Tries frontmatter `description:` first, falls back to first paragraph after `# Title`.
func readSkillDescription(skillDir string) string {
	path := filepath.Join(skillDir, "SKILL.md")

	// Resolve symlinks so we read the actual file.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}

	f, err := os.Open(resolved)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	pastTitle := false

	for scanner.Scan() {
		line := scanner.Text()

		// Frontmatter handling.
		if line == "---" {
			if !inFrontmatter && !frontmatterDone {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
		}

		if inFrontmatter {
			// Match `description: <text>` or `description: |` (multiline).
			if strings.HasPrefix(line, "description:") {
				val := strings.TrimPrefix(line, "description:")
				val = strings.TrimSpace(val)
				if val == "|" || val == ">" {
					// Read the next non-empty line as the description.
					for scanner.Scan() {
						next := strings.TrimSpace(scanner.Text())
						if next != "" {
							return truncateDescription(next)
						}
					}
					return ""
				}
				return truncateDescription(val)
			}
			continue
		}

		// Past frontmatter: look for first paragraph after # Title.
		if strings.HasPrefix(line, "# ") {
			pastTitle = true
			continue
		}
		if pastTitle && strings.TrimSpace(line) != "" {
			return truncateDescription(strings.TrimSpace(line))
		}
	}
	return ""
}

// truncateDescription shortens a description to a scannable length.
func truncateDescription(s string) string {
	// Take first sentence or max 80 chars.
	if idx := strings.IndexAny(s, ".!"); idx > 0 && idx < 80 {
		return s[:idx+1]
	}
	if len(s) > 80 {
		// Break at word boundary.
		cut := strings.LastIndex(s[:80], " ")
		if cut > 40 {
			return s[:cut] + "..."
		}
		return s[:80] + "..."
	}
	return s
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
