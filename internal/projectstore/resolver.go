package projectstore

import (
	"fmt"
	"os"

	"github.com/Naoray/scribe/internal/kit"
)

type Resolver struct {
	Project Store
	Global  Store
}

func NewResolver(projectRoot, globalRoot string) Resolver {
	return Resolver{
		Project: Project(projectRoot),
		Global:  Global(globalRoot),
	}
}

func (r Resolver) LoadKits() (map[string]*kit.Kit, error) {
	global, err := r.Global.LoadKits()
	if err != nil {
		return nil, fmt.Errorf("load global kits: %w", err)
	}
	project, err := r.Project.LoadKits()
	if err != nil {
		return nil, fmt.Errorf("load project kits: %w", err)
	}
	merged := make(map[string]*kit.Kit, len(global)+len(project))
	for name, entry := range global {
		merged[name] = entry
	}
	for name, entry := range project {
		merged[name] = entry
	}
	return merged, nil
}

func (r Resolver) SkillDir(name string) (string, bool) {
	projectDir := r.Project.SkillDir(name)
	if dirExists(projectDir) {
		return projectDir, true
	}
	globalDir := r.Global.SkillDir(name)
	if dirExists(globalDir) {
		return globalDir, true
	}
	return "", false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
