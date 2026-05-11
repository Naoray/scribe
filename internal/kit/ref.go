package kit

import (
	"fmt"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
)

// Ref is a parsed skill reference from a kit.
type Ref struct {
	Raw      string
	Registry string
	Source   manifest.Source
	Skill    string
	Local    bool
	Glob     bool
}

// ParseSkillRef parses same-registry, cross-registry, pinned github, and local
// skill references used by registry-published kits.
func ParseSkillRef(raw, defaultRegistry string) (Ref, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Ref{}, fmt.Errorf("invalid kit skill ref: empty")
	}

	if strings.HasPrefix(trimmed, "local:") {
		skill := strings.TrimPrefix(trimmed, "local:")
		if skill == "" {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: local skill is empty", raw)
		}
		return Ref{Raw: raw, Skill: skill, Local: true, Glob: hasGlobMeta(skill)}, nil
	}

	if strings.HasPrefix(trimmed, "github:") {
		idx := strings.LastIndex(trimmed, ":")
		if idx <= len("github:") || idx == len(trimmed)-1 {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: expected github:owner/repo@ref:skill", raw)
		}
		source, err := manifest.ParseSource(trimmed[:idx])
		if err != nil {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: %w", raw, err)
		}
		skill := trimmed[idx+1:]
		return Ref{
			Raw:      raw,
			Registry: source.Owner + "/" + source.Repo,
			Source:   source,
			Skill:    skill,
			Glob:     hasGlobMeta(skill),
		}, nil
	}

	if idx := strings.LastIndex(trimmed, ":"); idx >= 0 {
		registryName := trimmed[:idx]
		skill := trimmed[idx+1:]
		if registryName == "" || skill == "" {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: expected owner/repo:skill", raw)
		}
		if hasGlobMeta(registryName) {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: glob registry segment is not supported", raw)
		}
		if _, _, err := manifest.ParseOwnerRepo(registryName); err != nil {
			return Ref{}, fmt.Errorf("invalid kit skill ref %q: %w", raw, err)
		}
		return Ref{Raw: raw, Registry: registryName, Skill: skill, Glob: hasGlobMeta(skill)}, nil
	}

	if strings.Contains(trimmed, "/") {
		return Ref{}, fmt.Errorf("invalid kit skill ref %q: cross-registry refs must use owner/repo:skill", raw)
	}
	return Ref{Raw: raw, Registry: defaultRegistry, Skill: trimmed, Glob: hasGlobMeta(trimmed)}, nil
}
