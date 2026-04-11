package adopt

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
)

// resolvePaths replicates config.(*Config).AdoptionPaths() but operates on a bare
// AdoptionConfig, so FindCandidates does not need a *Config receiver.
func resolvePaths(cfg config.AdoptionConfig) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	resolvedHome := home
	if rh, err := filepath.EvalSymlinks(home); err == nil {
		resolvedHome = rh
	}

	builtins := []string{
		filepath.Join(resolvedHome, ".claude", "skills"),
		filepath.Join(resolvedHome, ".codex", "skills"),
	}

	result := make([]string, 0, len(builtins)+len(cfg.Paths))
	result = append(result, builtins...)

	for _, p := range cfg.Paths {
		var resolved string
		if strings.HasPrefix(p, "~/") {
			resolved = filepath.Join(home, p[2:])
		} else if filepath.IsAbs(p) {
			resolved = p
		} else {
			resolved = filepath.Join(home, p)
		}
		resolved = filepath.Clean(resolved)
		if rp, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = rp
		} else if errors.Is(err, fs.ErrNotExist) {
			if rel, relErr := filepath.Rel(home, resolved); relErr == nil && !strings.HasPrefix(rel, "..") {
				resolved = filepath.Join(resolvedHome, rel)
			}
		}
		rel, err := filepath.Rel(resolvedHome, resolved)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("adoption.paths entry %q is outside home", p)
		}
		result = append(result, resolved)
	}

	return result, nil
}

// builtinToolDirs maps a path suffix (relative to home) to a tool name.
// Used to infer Targets for candidates discovered in well-known locations.
var builtinToolDirs = map[string]string{
	filepath.Join(".claude", "skills"): "claude",
	filepath.Join(".codex", "skills"):  "codex",
}

// findCandidates implements FindCandidates.
func findCandidates(st *state.State, cfg config.AdoptionConfig) ([]Candidate, []Conflict, error) {
	adoptPaths, err := resolvePaths(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("adoption paths: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("home dir: %w", err)
	}
	// Resolve home symlinks so prefix comparisons work on macOS.
	resolvedHome := home
	if rh, err := filepath.EvalSymlinks(home); err == nil {
		resolvedHome = rh
	}

	// Build a set of adoption paths for quick membership test.
	adoptSet := make(map[string]bool, len(adoptPaths))
	for _, p := range adoptPaths {
		adoptSet[p] = true
	}

	// Walk each adoption path and collect skill subdirectories.
	var candidates []Candidate
	var conflicts []Conflict

	for _, root := range adoptPaths {
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, nil, fmt.Errorf("read %s: %w", root, err)
		}

		toolName := toolForPath(root, resolvedHome)

		for _, entry := range entries {
			name := entry.Name()
			if reservedName(name) {
				continue
			}
			if !validSkillName(name) {
				continue
			}

			// Resolve the actual directory (handles symlinks from tool dirs).
			entryPath := filepath.Join(root, name)
			skillDir, err := resolveSkillDir(entryPath)
			if err != nil {
				continue // unresolvable / not a skill dir
			}

			// Must have SKILL.md.
			if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
				continue
			}

			// Skip package sub-skills: skills whose SKILL.md symlinks into a
			// sibling directory are part of a package and not independently adoptable.
			if isPackageSubSkill(entryPath, root) {
				continue
			}

			// skillDir must lie within an adoption path (prevent escaping via symlink).
			if !withinAdoptionPath(skillDir, adoptPaths) {
				continue
			}

			// Compute hash of SKILL.md.
			hash, err := skillFileHash(skillDir)
			if err != nil {
				continue
			}

			// Determine targets from root path.
			var targets []string
			if toolName != "" {
				targets = []string{toolName}
			}

			if installed, ok := st.Installed[name]; ok {
				// Name already in state — check for conflict.
				if installed.InstalledHash == "" || installed.InstalledHash == hash {
					// Same hash (or no hash recorded): no-op, already managed.
					continue
				}
				// Hash differs: conflict.
				conflicts = append(conflicts, Conflict{
					Name:    name,
					Managed: installed,
					Unmanaged: Candidate{
						Name:      name,
						LocalPath: skillDir,
						Targets:   targets,
						Hash:      hash,
					},
				})
				continue
			}

			candidates = append(candidates, Candidate{
				Name:      name,
				LocalPath: skillDir,
				Targets:   targets,
				Hash:      hash,
			})
		}
	}

	return candidates, conflicts, nil
}

// isPackageSubSkill reports whether the skill at entryPath is a sub-skill of a
// package. A sub-skill has its SKILL.md as a symlink pointing into a sibling
// directory within the same scanBase (e.g. browse/SKILL.md → ../gstack/browse/SKILL.md).
// Such skills are not independently adoptable — they move as a unit with their package.
func isPackageSubSkill(entryPath, scanBase string) bool {
	skillMD := filepath.Join(entryPath, "SKILL.md")
	target, err := os.Readlink(skillMD)
	if err != nil {
		return false // not a symlink
	}

	// Resolve to absolute path.
	if !filepath.IsAbs(target) {
		target = filepath.Join(entryPath, target)
	}
	target = filepath.Clean(target)

	// Check if the target is inside a sibling dir in the same scanBase.
	// Pattern: <scanBase>/<package>/<subdir>/SKILL.md
	rel, err := filepath.Rel(scanBase, target)
	if err != nil {
		return false
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) < 2 {
		return false // target is directly in scanBase, not a sub-skill
	}
	pkg := parts[0]
	return pkg != filepath.Base(entryPath) // points into a different sibling dir
}

// resolveSkillDir resolves an entry path to its real skill directory.
// Follows symlinks; if the target is a file, returns its parent directory.
func resolveSkillDir(entryPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(entryPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return resolved, nil
	}
	// It's a file (e.g. ~/.claude/skills/foo → ~/.scribe/skills/foo/SKILL.md).
	// The parent is the skill dir.
	return filepath.Dir(resolved), nil
}

// toolForPath returns the tool name for a given adoption path by comparing
// the path against well-known tool directory suffixes.
func toolForPath(path, resolvedHome string) string {
	for suffix, tool := range builtinToolDirs {
		if path == filepath.Join(resolvedHome, suffix) {
			return tool
		}
	}
	return ""
}

// withinAdoptionPath reports whether dir is inside any of the adoption paths.
func withinAdoptionPath(dir string, adoptPaths []string) bool {
	for _, root := range adoptPaths {
		if dir == root || strings.HasPrefix(dir+string(filepath.Separator), root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// reservedName reports whether a name is reserved and should not be adopted.
func reservedName(name string) bool {
	switch name {
	case "versions", ".git", ".DS_Store":
		return true
	}
	return false
}

// validSkillName reports whether a name is a safe skill name.
func validSkillName(name string) bool {
	if len(name) == 0 || name[0] == '.' {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// skillFileHash returns the git blob SHA of ~/.../SKILL.md in skillDir.
func skillFileHash(skillDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "", err
	}
	return gitBlobSHA(data), nil
}

// gitBlobSHA computes a git blob SHA (sha1) for the given content.
// Duplicated from internal/state to avoid cross-package coupling.
func gitBlobSHA(data []byte) string {
	payload := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf("%x", sum)
}
