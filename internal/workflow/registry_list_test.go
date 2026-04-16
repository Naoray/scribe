package workflow_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

func TestCountSkillsPerRegistry(t *testing.T) {
	// Skills record their registry in Sources[].Registry.
	st := &state.State{
		Installed: map[string]state.InstalledSkill{
			"browse": {Sources: []state.SkillSource{{Registry: "ArtistfyHQ/skills"}}},
			"deploy": {Sources: []state.SkillSource{{Registry: "ArtistfyHQ/skills"}}},
			"lint":   {Sources: []state.SkillSource{{Registry: "Naoray/my-skills"}}},
			"orphan": {}, // no sources — not from any registry
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
			map[string]int{"ArtistfyHQ/skills": 2, "Naoray/my-skills": 1},
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

func TestPrintRegistryJSON_Shape(t *testing.T) {
	syncTime := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	st := &state.State{
		LastSync: syncTime,
		Installed: map[string]state.InstalledSkill{
			"browse": {Sources: []state.SkillSource{{Registry: "Foo/bar"}}},
			"deploy": {Sources: []state.SkillSource{{Registry: "Foo/bar"}}},
		},
	}

	var buf bytes.Buffer
	if err := workflow.PrintRegistryJSON(&buf, []string{"Foo/bar"}, st); err != nil {
		t.Fatalf("PrintRegistryJSON error: %v", err)
	}

	var out struct {
		Registries []struct {
			Registry   string `json:"registry"`
			SkillCount int    `json:"skill_count"`
		} `json:"registries"`
		LastSync *string `json:"last_sync"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal error: %v\nraw: %s", err, buf.String())
	}

	if len(out.Registries) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(out.Registries))
	}
	if out.Registries[0].Registry != "Foo/bar" {
		t.Errorf("registry = %q, want %q", out.Registries[0].Registry, "Foo/bar")
	}
	if out.Registries[0].SkillCount != 2 {
		t.Errorf("skill_count = %d, want 2", out.Registries[0].SkillCount)
	}
	if out.LastSync == nil {
		t.Fatal("last_sync is nil, want non-nil")
	}
	if *out.LastSync != "2026-04-03T10:00:00Z" {
		t.Errorf("last_sync = %q, want %q", *out.LastSync, "2026-04-03T10:00:00Z")
	}
}

func TestPrintRegistryJSON_Empty(t *testing.T) {
	st := &state.State{
		Installed: map[string]state.InstalledSkill{},
	}

	var buf bytes.Buffer
	if err := workflow.PrintRegistryJSON(&buf, nil, st); err != nil {
		t.Fatalf("PrintRegistryJSON error: %v", err)
	}

	var out struct {
		Registries []json.RawMessage `json:"registries"`
		LastSync   *string           `json:"last_sync"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	if len(out.Registries) != 0 {
		t.Errorf("expected empty registries array, got %d", len(out.Registries))
	}
	if out.LastSync != nil {
		t.Errorf("expected null last_sync, got %q", *out.LastSync)
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
