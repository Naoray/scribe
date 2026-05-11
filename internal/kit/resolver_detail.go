package kit

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/projectfile"
)

type ResolvedOrigin string

const (
	OriginSameRegistry  ResolvedOrigin = "same_registry"
	OriginCrossRegistry ResolvedOrigin = "cross_registry"
	OriginLocal         ResolvedOrigin = "local"
)

type ResolverInput struct {
	Project         *projectfile.ProjectFile
	Kits            map[string]*Kit
	InstalledSkills []string
	Registries      []config.RegistryConfig
	NowRegistry     func(context.Context, string) (*manifest.Manifest, error)
}

type ResolvedSkill struct {
	Name     string          `json:"name"`
	Origin   ResolvedOrigin  `json:"origin"`
	Registry string          `json:"registry,omitempty"`
	Source   manifest.Source `json:"source,omitempty"`
	Aliased  bool            `json:"aliased,omitempty"`
	AliasFor string          `json:"alias_for,omitempty"`
}

type MissingRef struct {
	Ref      string `json:"ref"`
	Registry string `json:"registry,omitempty"`
	Reason   string `json:"reason"`
}

type Conflict struct {
	Name    string `json:"name"`
	Current string `json:"current"`
	Next    string `json:"next"`
}

type Resolution struct {
	Skills    []ResolvedSkill `json:"skills"`
	Missing   []MissingRef    `json:"missing,omitempty"`
	Conflicts []Conflict      `json:"conflicts,omitempty"`
}

func ResolveWithDetail(ctx context.Context, in ResolverInput) (Resolution, error) {
	pf := in.Project
	if pf == nil {
		pf = &projectfile.ProjectFile{}
	}

	var refs []kitRefWithRegistry
	for _, kitName := range pf.Kits {
		k, ok := in.Kits[kitName]
		if !ok {
			return Resolution{}, fmt.Errorf("kit %q not found", kitName)
		}
		defaultRegistry := ""
		if k.Source != nil {
			defaultRegistry = k.Source.Registry
		}
		for _, raw := range k.Skills {
			refs = append(refs, kitRefWithRegistry{Raw: raw, DefaultRegistry: defaultRegistry, Published: k.Source != nil})
		}
	}
	for _, raw := range pf.Add {
		refs = append(refs, kitRefWithRegistry{Raw: raw})
	}

	connected := connectedRegistrySet(in.Registries)
	installed := installedSkillSet(in.InstalledSkills)
	result := Resolution{}
	seen := map[string]ResolvedSkill{}
	for _, rawRef := range refs {
		ref, err := ParseSkillRef(rawRef.Raw, rawRef.DefaultRegistry)
		if err != nil {
			return Resolution{}, err
		}
		if ref.Local {
			if rawRef.Published {
				result.Missing = append(result.Missing, MissingRef{Ref: ref.Raw, Reason: "local_ref_forbidden"})
				continue
			}
			addResolvedSkill(&result, seen, ResolvedSkill{Name: ref.Skill, Origin: OriginLocal})
			continue
		}

		origin := OriginCrossRegistry
		if ref.Registry == rawRef.DefaultRegistry || ref.Source.Host != "" {
			if ref.Registry == rawRef.DefaultRegistry {
				origin = OriginSameRegistry
			}
		}
		if ref.Registry != "" && rawRef.DefaultRegistry == ref.Registry {
			origin = OriginSameRegistry
		}
		if ref.Registry != "" && !connected[ref.Registry] && ref.Registry != rawRef.DefaultRegistry {
			result.Missing = append(result.Missing, MissingRef{Ref: ref.Raw, Registry: ref.Registry, Reason: "registry_not_connected"})
			continue
		}

		matches, missing, err := resolveRefMatches(ctx, ref, rawRef.DefaultRegistry, in.NowRegistry, installed)
		if err != nil {
			return Resolution{}, err
		}
		if missing != nil {
			result.Missing = append(result.Missing, *missing)
			continue
		}
		for _, name := range matches {
			addResolvedSkill(&result, seen, ResolvedSkill{
				Name:     name,
				Origin:   origin,
				Registry: ref.Registry,
				Source:   ref.Source,
			})
		}
	}

	for _, skill := range pf.Remove {
		filtered := result.Skills[:0]
		for _, resolved := range result.Skills {
			if resolved.Name != skill {
				filtered = append(filtered, resolved)
			}
		}
		result.Skills = filtered
	}
	sort.Slice(result.Skills, func(i, j int) bool { return result.Skills[i].Name < result.Skills[j].Name })
	return result, nil
}

type kitRefWithRegistry struct {
	Raw             string
	DefaultRegistry string
	Published       bool
}

func connectedRegistrySet(registries []config.RegistryConfig) map[string]bool {
	set := make(map[string]bool, len(registries))
	for _, rc := range registries {
		if rc.Enabled {
			set[rc.Repo] = true
		}
	}
	return set
}

func installedSkillSet(skills []string) map[string]bool {
	set := make(map[string]bool, len(skills))
	for _, skill := range skills {
		set[skill] = true
	}
	return set
}

func resolveRefMatches(ctx context.Context, ref Ref, defaultRegistry string, nowRegistry func(context.Context, string) (*manifest.Manifest, error), installed map[string]bool) ([]string, *MissingRef, error) {
	if !ref.Glob {
		return []string{ref.Skill}, nil, nil
	}

	var candidates []string
	if ref.Registry == "" || ref.Registry == defaultRegistry {
		for name := range installed {
			if match, err := filepath.Match(ref.Skill, name); err != nil {
				return nil, nil, fmt.Errorf("match skill pattern %q: %w", ref.Skill, err)
			} else if match {
				candidates = append(candidates, name)
			}
		}
	} else if nowRegistry != nil {
		m, err := nowRegistry(ctx, ref.Registry)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range m.Catalog {
			if match, err := filepath.Match(ref.Skill, entry.Name); err != nil {
				return nil, nil, fmt.Errorf("match skill pattern %q: %w", ref.Skill, err)
			} else if match {
				candidates = append(candidates, entry.Name)
			}
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return nil, &MissingRef{Ref: ref.Raw, Registry: ref.Registry, Reason: "glob_no_matches"}, nil
	}
	return candidates, nil, nil
}

func addResolvedSkill(result *Resolution, seen map[string]ResolvedSkill, skill ResolvedSkill) {
	if existing, ok := seen[skill.Name]; ok {
		if existing.Registry != skill.Registry {
			result.Conflicts = append(result.Conflicts, Conflict{Name: skill.Name, Current: existing.Registry, Next: skill.Registry})
		}
		return
	}
	seen[skill.Name] = skill
	result.Skills = append(result.Skills, skill)
}
