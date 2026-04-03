package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

func TestCountSkillsPerRegistry(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"browse": {Registries: []string{"ArtistfyHQ/skills"}},
			"deploy": {Registries: []string{"ArtistfyHQ/skills", "Naoray/my-skills"}},
			"lint":   {Registries: []string{"Naoray/my-skills"}},
			"orphan": {Registries: nil},
			"empty":  {Registries: []string{}},
		},
	}

	cases := []struct {
		name    string
		repos   []string
		wantMap map[string]int
	}{
		{
			"multi-registry counts",
			[]string{"ArtistfyHQ/skills", "Naoray/my-skills"},
			map[string]int{"ArtistfyHQ/skills": 2, "Naoray/my-skills": 2},
		},
		{
			"case-insensitive match",
			[]string{"artistfyhq/skills"},
			map[string]int{"artistfyhq/skills": 2},
		},
		{
			"no matching skills",
			[]string{"unknown/repo"},
			map[string]int{"unknown/repo": 0},
		},
		{
			"empty repos",
			[]string{},
			map[string]int{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := workflow.CountSkillsPerRegistry(c.repos, st)
			for repo, want := range c.wantMap {
				if got[repo] != want {
					t.Errorf("repo %q: got %d, want %d", repo, got[repo], want)
				}
			}
		})
	}
}

func TestPrintRegistryList_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"browse": {Registries: []string{"Foo/bar"}},
			"deploy": {Registries: []string{"Foo/bar"}},
		},
	}

	repos := []string{"Foo/bar"}

	counts := workflow.CountSkillsPerRegistry(repos, st)

	// Verify counts are correct.
	if counts["Foo/bar"] != 2 {
		t.Fatalf("expected 2 skills for Foo/bar, got %d", counts["Foo/bar"])
	}
}

func TestPrintRegistryList_EmptyRepos(t *testing.T) {
	repos := []string{}
	st := &state.State{
		Installed: map[string]state.InstalledSkill{},
	}
	counts := workflow.CountSkillsPerRegistry(repos, st)
	if len(counts) != 0 {
		t.Fatalf("expected empty map, got %v", counts)
	}
}

func TestRegistryListSteps_Composition(t *testing.T) {
	steps := workflow.RegistryListSteps()
	if len(steps) == 0 {
		t.Fatal("RegistryListSteps() returned empty list")
	}

	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "PrintRegistryList" {
		t.Errorf("expected last step PrintRegistryList, got %s", steps[len(steps)-1].Name)
	}
}
