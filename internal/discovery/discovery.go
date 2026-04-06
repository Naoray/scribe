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
	"gopkg.in/yaml.v3"
)

// validSkillName matches safe skill names that work as catalog entry names and filesystem paths.
var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// SkillMeta holds metadata parsed from SKILL.md frontmatter.
type SkillMeta struct {
	Name        string
	Description string
	Version     string
	Author      string
}

// rawFrontmatter maps the YAML frontmatter structure in SKILL.md files.
type rawFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Version     string         `yaml:"version"`
	Author      string         `yaml:"author"`
	Metadata    map[string]any `yaml:"metadata"`
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

			skillDir := filepath.Join(dir.path, name)

			// Check if this is a skill directory (contains files like SKILL.md)
			// or a registry-slug directory (contains sub-skill directories).
			if isSkillDir(skillDir) {
				// Direct skill: ~/.scribe/skills/<name>/
				if seen[name] || !validSkillName.MatchString(name) {
					continue
				}

				empty, err := isDirEmpty(skillDir)
				if err != nil || empty {
					continue
				}

				seen[name] = true
				sk := buildSkill(name, skillDir, dir.path, dir.target, st)
				skills = append(skills, sk)
			} else {
				// Registry-slug directory: scan sub-entries as skills.
				subEntries, err := os.ReadDir(skillDir)
				if err != nil {
					continue
				}
				for _, subEntry := range subEntries {
					if !subEntry.IsDir() {
						continue
					}
					subName := subEntry.Name()
					qualifiedName := name + "/" + subName
					if seen[qualifiedName] || !validSkillName.MatchString(subName) {
						continue
					}

					subDir := filepath.Join(skillDir, subName)
					empty, err := isDirEmpty(subDir)
					if err != nil || empty {
						continue
					}

					seen[qualifiedName] = true
					sk := buildSkill(qualifiedName, subDir, dir.path, dir.target, st)
					skills = append(skills, sk)
				}
			}
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
			Targets: installed.Tools,
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

// isSkillDir checks if a directory is a skill directory (has a SKILL.md file)
// as opposed to a registry-slug directory containing sub-skill directories.
func isSkillDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil
}

// buildSkill creates a Skill from a directory, enriching with state info.
func buildSkill(name, skillDir, scanBase, target string, st *state.State) Skill {
	meta := readSkillMetadata(skillDir)
	hash, _ := contentHash(skillDir)

	sk := Skill{
		Name:        name,
		Description: meta.Description,
		LocalPath:   skillDir,
		Package:     detectPackage(skillDir, scanBase),
		ContentHash: hash,
	}

	if installed, ok := st.Installed[name]; ok {
		sk.Source = installed.Source
		sk.Version = installed.DisplayVersion()
		sk.Targets = installed.Tools
	} else if target != "" {
		sk.Targets = []string{target}
	}

	// Version resolution: frontmatter → state → content hash.
	if meta.Version != "" {
		sk.Version = meta.Version
	}
	if sk.Version == "" && hash != "" {
		sk.Version = "#" + hash
	}

	return sk
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
	target = filepath.Clean(target)

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
// Parses YAML frontmatter and falls back to first paragraph for description.
func readSkillMetadata(skillDir string) SkillMeta {
	path := filepath.Join(skillDir, "SKILL.md")

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return SkillMeta{}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return SkillMeta{}
	}

	fm := extractFrontmatter(data)
	if fm == "" {
		return SkillMeta{Description: extractFirstParagraph(data)}
	}

	var raw rawFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &raw); err != nil {
		return SkillMeta{}
	}

	meta := SkillMeta{
		Name:        raw.Name,
		Description: truncateDescription(raw.Description),
		Version:     raw.Version,
		Author:      raw.Author,
	}

	// metadata.* overrides top-level (agentskills spec).
	if v, ok := raw.Metadata["version"]; ok {
		meta.Version = fmt.Sprint(v)
	}
	if v, ok := raw.Metadata["author"]; ok {
		meta.Author = fmt.Sprint(v)
	}

	if meta.Description == "" {
		meta.Description = extractFirstParagraph(data)
	}

	return meta
}

// extractFrontmatter returns the YAML content between --- delimiters.
// Uses line-by-line scanning so --- inside YAML values doesn't terminate early.
func extractFrontmatter(data []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return ""
	}
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			return strings.Join(lines, "\n")
		}
		lines = append(lines, line)
	}
	return ""
}

// extractFirstParagraph returns the first non-empty line after a # heading.
func extractFirstParagraph(data []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	pastTitle := false
	for scanner.Scan() {
		line := scanner.Text()
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

// stripQuotes removes surrounding single or double quotes from a YAML value.
func stripQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
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
