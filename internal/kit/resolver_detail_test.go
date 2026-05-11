package kit

import (
	"context"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/projectfile"
)

func TestResolveWithDetailClassifiesRefs(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		kit        *Kit
		registries []config.RegistryConfig
		installed  []string
		wantSkills []string
		wantMiss   []MissingRef
	}{
		{
			name: "same registry plain and glob",
			kit: &Kit{
				Name:   "baseline",
				Skills: []string{"tdd", "init-*"},
				Source: &Source{Registry: "acme/skills"},
			},
			registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}},
			installed:  []string{"init-api", "init-web"},
			wantSkills: []string{"init-api:same_registry:acme/skills", "init-web:same_registry:acme/skills", "tdd:same_registry:acme/skills"},
		},
		{
			name: "cross registry missing",
			kit: &Kit{
				Name:   "baseline",
				Skills: []string{"other/skills:debugging"},
				Source: &Source{Registry: "acme/skills"},
			},
			registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}},
			wantMiss:   []MissingRef{{Ref: "other/skills:debugging", Registry: "other/skills", Reason: "registry_not_connected"}},
		},
		{
			name: "published local ref forbidden",
			kit: &Kit{
				Name:   "baseline",
				Skills: []string{"local:private"},
				Source: &Source{Registry: "acme/skills"},
			},
			registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}},
			wantMiss:   []MissingRef{{Ref: "local:private", Reason: "local_ref_forbidden"}},
		},
		{
			name: "connected cross registry",
			kit: &Kit{
				Name:   "baseline",
				Skills: []string{"other/skills:debugging"},
				Source: &Source{Registry: "acme/skills"},
			},
			registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}, {Repo: "other/skills", Enabled: true}},
			wantSkills: []string{"debugging:cross_registry:other/skills"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveWithDetail(ctx, ResolverInput{
				Project:         &projectfile.ProjectFile{Kits: []string{"baseline"}},
				Kits:            map[string]*Kit{"baseline": tt.kit},
				InstalledSkills: tt.installed,
				Registries:      tt.registries,
			})
			if err != nil {
				t.Fatalf("ResolveWithDetail: %v", err)
			}
			if joinedSkills(got.Skills) != strings.Join(tt.wantSkills, ",") {
				t.Fatalf("skills = %s, want %s", joinedSkills(got.Skills), strings.Join(tt.wantSkills, ","))
			}
			if len(got.Missing) != len(tt.wantMiss) {
				t.Fatalf("missing = %+v, want %+v", got.Missing, tt.wantMiss)
			}
			for i := range tt.wantMiss {
				if got.Missing[i] != tt.wantMiss[i] {
					t.Fatalf("missing[%d] = %+v, want %+v", i, got.Missing[i], tt.wantMiss[i])
				}
			}
		})
	}
}

func TestResolveWithDetailCrossRegistryGlobUsesCatalog(t *testing.T) {
	got, err := ResolveWithDetail(context.Background(), ResolverInput{
		Project: &projectfile.ProjectFile{Kits: []string{"baseline"}},
		Kits: map[string]*Kit{"baseline": {
			Name:   "baseline",
			Skills: []string{"other/skills:init-*"},
			Source: &Source{Registry: "acme/skills"},
		}},
		Registries: []config.RegistryConfig{{Repo: "acme/skills", Enabled: true}, {Repo: "other/skills", Enabled: true}},
		NowRegistry: func(_ context.Context, registry string) (*manifest.Manifest, error) {
			return &manifest.Manifest{Catalog: []manifest.Entry{{Name: "init-api"}, {Name: "other"}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("ResolveWithDetail: %v", err)
	}
	if joinedSkills(got.Skills) != "init-api:cross_registry:other/skills" {
		t.Fatalf("skills = %s", joinedSkills(got.Skills))
	}
}

func joinedSkills(skills []ResolvedSkill) string {
	parts := make([]string, 0, len(skills))
	for _, skill := range skills {
		parts = append(parts, skill.Name+":"+string(skill.Origin)+":"+skill.Registry)
	}
	return strings.Join(parts, ",")
}
