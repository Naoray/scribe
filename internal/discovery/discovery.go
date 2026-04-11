package discovery

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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

// reservedNames lists directory names that should be skipped during scanning.
var reservedNames = map[string]bool{
	"versions": true,
	".git":     true,
	".DS_Store": true,
}

// Skill represents a skill found on disk, optionally enriched with state info.
type Skill struct {
	Name        string
	Description string   // short description from SKILL.md frontmatter or first paragraph
	Package     string   // parent package name if skill is a symlink sub-skill (e.g. "gstack")
	LocalPath   string   // absolute path on disk
	ContentHash string   // deterministic content fingerprint
	Targets     []string // from state if tracked, else inferred from location
	Modified    bool     // SKILL.md hash differs from installed_hash in state
	Conflicted  bool     // SKILL.md contains unresolved merge conflict markers
	Revision    int      // from state
}

// OnDisk scans ~/.scribe/skills/ (flat directories) and ~/.claude/skills/ (file symlinks)
// for installed skills. Cross-references state.json for tracked skill metadata.
// Deduplicates by resolved path (a skill found via both locations is one skill).
func OnDisk(st *state.State) ([]Skill, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	scribeSkills := filepath.Join(home, ".scribe", "skills")
	claudeSkills := filepath.Join(home, ".claude", "skills")

	// seen tracks skill names we've already processed (dedup by name).
	seen := map[string]bool{}
	var skills []Skill

	// 1. Scan ~/.scribe/skills/ — every directory with SKILL.md is a skill.
	entries, err := os.ReadDir(scribeSkills)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", scribeSkills, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if reservedNames[name] || !validSkillName.MatchString(name) {
			continue
		}

		skillDir := filepath.Join(scribeSkills, name)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue // no SKILL.md, skip
		}

		seen[name] = true
		sk := buildSkill(name, skillDir, scribeSkills, "", st)
		skills = append(skills, sk)
	}

	// 2. Scan ~/.claude/skills/ — file symlinks to ~/.scribe/skills/<name>/SKILL.md.
	claudeEntries, err := os.ReadDir(claudeSkills)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", claudeSkills, err)
	}
	for _, entry := range claudeEntries {
		name := entry.Name()
		if reservedNames[name] || !validSkillName.MatchString(name) {
			continue
		}

		entryPath := filepath.Join(claudeSkills, name)

		// Resolve the symlink to find the actual skill directory.
		resolved, err := filepath.EvalSymlinks(entryPath)
		if err != nil {
			continue // broken symlink, skip
		}

		// For file symlinks (pointing to SKILL.md), the skill dir is the parent.
		// For directory symlinks (legacy), the skill dir is the resolved path itself.
		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}

		var skillDir string
		if info.IsDir() {
			skillDir = resolved
		} else {
			skillDir = filepath.Dir(resolved)
		}

		// Derive the skill name from the scribe store directory name.
		skillName := filepath.Base(skillDir)
		if seen[skillName] {
			continue // already found via scribe store
		}

		// Verify SKILL.md exists in the resolved directory.
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue
		}

		seen[skillName] = true
		sk := buildSkill(skillName, skillDir, filepath.Dir(skillDir), "claude", st)
		skills = append(skills, sk)
	}

	// 3. Include state-tracked skills not found on disk (orphans).
	for name, installed := range st.Installed {
		if seen[name] {
			continue
		}
		skills = append(skills, Skill{
			Name:     name,
			Targets:  installed.Tools,
			Revision: installed.Revision,
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		// Group by package (standalone first, then by package name), alpha within.
		pkgI, pkgJ := skills[i].Package, skills[j].Package
		if pkgI != pkgJ {
			if pkgI == "" {
				return true // standalone before packages
			}
			if pkgJ == "" {
				return false
			}
			return pkgI < pkgJ
		}
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

// buildSkill creates a Skill from a directory, enriching with state info.
func buildSkill(name, skillDir, scanBase, target string, st *state.State) Skill {
	meta := readSkillMetadata(skillDir)
	hash, _ := contentHash(skillDir)
	fileHash, _ := skillFileHash(skillDir)

	// Read SKILL.md content for conflict detection.
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	conflicted := false
	if content, err := os.ReadFile(skillMDPath); err == nil {
		conflicted = hasConflictMarkers(content)
	}

	sk := Skill{
		Name:        name,
		Description: meta.Description,
		LocalPath:   skillDir,
		Package:     detectPackage(skillDir, scanBase),
		ContentHash: hash,
		Conflicted:  conflicted,
	}

	if installed, ok := st.Installed[name]; ok {
		sk.Targets = installed.Tools
		sk.Revision = installed.Revision
		// Detect local modification.
		if installed.InstalledHash != "" && fileHash != "" && fileHash != installed.InstalledHash {
			sk.Modified = true
		}
	} else if target != "" {
		sk.Targets = []string{target}
	}

	return sk
}

// hasConflictMarkers checks if content contains unresolved Git merge conflict markers.
func hasConflictMarkers(content []byte) bool {
	return bytes.Contains(content, []byte("<<<<<<< "))
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

