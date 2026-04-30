package manifest

import (
	"fmt"
	"regexp"
	"strings"
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// PackageMeta contains the package-level fields collected by scribe init.
type PackageMeta struct {
	Name        string
	Description string
	Author      string
}

// Skill describes a local skill directory to include in a package manifest.
type Skill struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ScaffoldPackageManifest builds a parseable package manifest for a new skill package.
func ScaffoldPackageManifest(meta PackageMeta, skills []Skill) ([]byte, error) {
	meta.Name = strings.TrimSpace(meta.Name)
	if err := ValidatePackageName(meta.Name); err != nil {
		return nil, err
	}

	pkg := &Package{
		Name:        meta.Name,
		Version:     "0.1.0",
		Description: strings.TrimSpace(meta.Description),
	}
	if author := strings.TrimSpace(meta.Author); author != "" {
		pkg.Authors = []string{author}
	}

	catalog := make([]Entry, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		path := strings.TrimSpace(skill.Path)
		if name == "" {
			return nil, fmt.Errorf("skill has empty name for path %q", path)
		}
		if path == "" {
			return nil, fmt.Errorf("skill %q has empty path", name)
		}
		catalog = append(catalog, Entry{Name: name, Path: path})
	}

	m := &Manifest{
		APIVersion: "scribe/v1",
		Kind:       "Package",
		Package:    pkg,
		Catalog:    catalog,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m.Encode()
}

// ValidatePackageName checks whether name is suitable for a package manifest.
func ValidatePackageName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("package name is required")
	}
	if !packageNamePattern.MatchString(name) {
		return fmt.Errorf("invalid package name %q: use letters, numbers, dots, underscores, or dashes", name)
	}
	return nil
}
