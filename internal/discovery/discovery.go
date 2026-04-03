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

// SkillMeta holds metadata parsed from SKILL.md frontmatter.
type SkillMeta struct {
	Name        string
	Description string
	Version     string
}

// Skill represents a skill found on disk, optionally enriched with state info.
type Skill struct {
	Name        string
	Description string   // short description from SKILL.md frontmatter or first paragraph
	Package     string   // parent package name if skill is a symlink sub-skill (e.g. "gstack")
	LocalPath   string   // absolute path on disk
	Source      string   // from state if tracked, else empty
	Version     string   // from state if tracked, else empty
	ContentHash string   // deterministic content fingerprint
	Targets     []string // from state if tracked, else inferred from location
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

			meta := readSkillMetadata(skillDir)
			hash, _ := contentHash(skillDir)

			sk := Skill{
				Name:        name,
				Description: meta.Description,
				LocalPath:   skillDir,
				Package:     detectPackage(skillDir, dir.path),
				ContentHash: hash,
			}

			if installed, ok := st.Installed[name]; ok {
				sk.Source = installed.Source
				sk.Version = installed.DisplayVersion()
				sk.Targets = installed.Targets
			} else if dir.target != "" {
				sk.Targets = []string{dir.target}
			}

			// Version resolution: frontmatter → state → content hash.
			if meta.Version != "" {
				sk.Version = meta.Version
			}
			if sk.Version == "" && hash != "" {
				sk.Version = "#" + hash
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

// readSkillMetadata extracts metadata from SKILL.md frontmatter.
// Parses name:, version:, and description: fields using line-by-line scanning
// (not a YAML library) to avoid type coercion issues.
func readSkillMetadata(skillDir string) SkillMeta {
	path := filepath.Join(skillDir, "SKILL.md")

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return SkillMeta{}
	}

	f, err := os.Open(resolved)
	if err != nil {
		return SkillMeta{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	pastTitle := false

	var meta SkillMeta

	for scanner.Scan() {
		line := scanner.Text()

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
			if strings.HasPrefix(line, "description:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				if val == "|" || val == ">" {
					for scanner.Scan() {
						next := strings.TrimSpace(scanner.Text())
						if next != "" {
							meta.Description = truncateDescription(next)
							break
						}
					}
				} else {
					meta.Description = truncateDescription(val)
				}
				continue
			}
			if strings.HasPrefix(line, "version:") {
				meta.Version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
				continue
			}
			if strings.HasPrefix(line, "name:") {
				meta.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				continue
			}
			continue
		}

		// Past frontmatter: look for first paragraph after # Title.
		if strings.HasPrefix(line, "# ") {
			pastTitle = true
			continue
		}
		if pastTitle && meta.Description == "" && strings.TrimSpace(line) != "" {
			meta.Description = truncateDescription(strings.TrimSpace(line))
		}
	}
	return meta
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
