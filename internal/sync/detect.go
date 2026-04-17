package sync

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Naoray/scribe/internal/tools"
)

// Kind classifies a fetched repo payload.
//
// KindSkill: single-authorship skill; dir-symlinked into tool skill dirs.
// KindPackage: multi-skill bundle or self-installing toolkit; stored under
// ~/.scribe/packages/<name>/ and never projected into agent skill dirs.
type Kind string

const (
	KindSkill   Kind = "skill"
	KindPackage Kind = "package"
)

// installScriptNames names the bootstrap entry points a package may ship at
// its repo root. Presence of any (executable or not) marks a payload as a
// self-installing package when the root SKILL.md is absent.
var installScriptNames = map[string]bool{
	"setup":       true,
	"install.sh":  true,
	"install":     true,
	"bootstrap":   true,
	"Makefile":    true,
	"package.json": true,
}

// DetectKind classifies a fetched file tree as either a skill or a package.
//
// Rules (first match wins):
//  1. Any SKILL.md at a non-root path (e.g. "browse/SKILL.md") → package.
//  2. No SKILL.md at the root AND the tree has a recognised install script
//     (setup, install.sh, bootstrap, Makefile, package.json) → package.
//  3. Otherwise → skill.
//
// This mirrors the spec at docs/superpowers/specs/2026-04-17-packages-store-design.md.
// The detection runs against in-memory SkillFile slices before the tree is
// written to disk, so nothing is persisted until routing is decided.
func DetectKind(files []tools.SkillFile) Kind {
	hasRootSkill := false
	hasNestedSkill := false
	hasInstallScript := false

	for _, f := range files {
		path := filepath.ToSlash(filepath.Clean(f.Path))
		base := filepath.Base(path)

		if base == "SKILL.md" {
			if !strings.Contains(path, "/") {
				hasRootSkill = true
			} else {
				hasNestedSkill = true
			}
			continue
		}
		if !strings.Contains(path, "/") && installScriptNames[base] {
			hasInstallScript = true
		}
	}

	if hasNestedSkill {
		return KindPackage
	}
	if !hasRootSkill && hasInstallScript {
		return KindPackage
	}
	return KindSkill
}

// DetectKindFromDir classifies an existing on-disk directory. Used by the
// migration pass: first sync after upgrade walks ~/.scribe/skills/<name>/
// for each installed skill and reclassifies any that look like packages.
func DetectKindFromDir(dir string) (Kind, error) {
	var files []tools.SkillFile
	// We don't actually need contents — DetectKind only looks at paths.
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip Scribe's version snapshot dir and vendor-ish noise.
			name := d.Name()
			if path != dir && (name == "versions" || name == ".git" || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		// Ignore Scribe-managed merge-base files so they don't skew detection.
		if rel == ".scribe-base.md" {
			return nil
		}
		files = append(files, tools.SkillFile{Path: rel})
		return nil
	})
	if err != nil {
		return KindSkill, err
	}
	return DetectKind(files), nil
}
