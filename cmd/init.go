package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
)

func discoverPackageSkills(root string) ([]manifest.Skill, error) {
	var skills []manifest.Skill

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && shouldSkipInitDiscoveryDir(d.Name()) {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}

		skillFile := filepath.Join(path, "SKILL.md")
		info, err := os.Stat(skillFile)
		if err != nil || info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := filepath.Base(path)
		if meta, err := discovery.ReadSkillMetadata(path); err == nil && strings.TrimSpace(meta.Name) != "" {
			name = strings.TrimSpace(meta.Name)
		}
		skills = append(skills, manifest.Skill{Name: name, Path: rel})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name != skills[j].Name {
			return skills[i].Name < skills[j].Name
		}
		return skills[i].Path < skills[j].Path
	})
	return skills, nil
}

func shouldSkipInitDiscoveryDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == ".scribe" || name == "versions"
}
