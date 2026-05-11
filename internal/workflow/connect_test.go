package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

type connectTestProvider struct {
	isTeam   bool
	manifest *manifest.Manifest
}

func (p connectTestProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	if p.manifest != nil {
		return &provider.DiscoverResult{
			IsTeam:   p.isTeam,
			Entries:  p.manifest.Catalog,
			Manifest: p.manifest,
		}, nil
	}
	return &provider.DiscoverResult{
		IsTeam: p.isTeam,
		Entries: []manifest.Entry{
			{Name: "deploy", Source: "github:acme/skills@main", Path: "skills/deploy"},
		},
	}, nil
}

func (p connectTestProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	return nil, errors.New("unused")
}

type connectVisibilityClient struct {
	private bool
	err     error
}

func (c connectVisibilityClient) RepositoryIsPrivate(context.Context, string, string) (bool, error) {
	return c.private, c.err
}

type connectIndexClient struct{}

func (connectIndexClient) RepositoryDefaultBranch(context.Context, string, string) (string, error) {
	return "main", nil
}

func (connectIndexClient) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "abc123", nil
}

func TestConnectSteps_EndsWithShowAvailable(t *testing.T) {
	steps := workflow.ConnectSteps()
	last := steps[len(steps)-1]
	if last.Name != "ShowAvailable" {
		t.Errorf("expected last step ShowAvailable, got %s (connect must not auto-install)", last.Name)
	}
}

func TestStepIndexPublicRegistryWritesLocalIndex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bag := &workflow.Bag{
		Config: &config.Config{},
		Provider: connectTestProvider{
			isTeam: true,
			manifest: &manifest.Manifest{
				APIVersion: "scribe/v1",
				Kind:       "Registry",
				Team:       &manifest.Team{Name: "Acme"},
				Catalog:    []manifest.Entry{{Name: "deploy"}, {Name: "review"}},
				Kits:       []manifest.KitEntry{{Name: "ops"}},
			},
		},
		Visibility:    connectVisibilityClient{private: false},
		RegistryIndex: connectIndexClient{},
		RepoArg:       "acme/skills",
	}

	for _, step := range []func(context.Context, *workflow.Bag) error{
		workflow.StepFetchManifest,
		workflow.StepValidateManifest,
		workflow.StepInferRegistryType,
		workflow.StepIndexPublicRegistry,
	} {
		if err := step(context.Background(), bag); err != nil {
			t.Fatalf("step failed: %v", err)
		}
	}

	data, err := os.ReadFile(filepath.Join(home, ".scribe", "index", "registries.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx struct {
		Registries []struct {
			Repo       string `json:"repo"`
			HeadSHA    string `json:"head_sha"`
			SkillCount int    `json:"skill_count"`
			KitCount   int    `json:"kit_count"`
		} `json:"registries"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(idx.Registries) != 1 || idx.Registries[0].Repo != "acme/skills" {
		t.Fatalf("registries = %#v", idx.Registries)
	}
	if idx.Registries[0].HeadSHA != "abc123" || idx.Registries[0].SkillCount != 2 || idx.Registries[0].KitCount != 1 {
		t.Fatalf("registry = %#v", idx.Registries[0])
	}
}

func TestStepIndexPublicRegistrySkipsPrivateAndUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, visibility := range []string{config.RegistryVisibilityPrivate, config.RegistryVisibilityUnknown} {
		t.Run(visibility, func(t *testing.T) {
			bag := &workflow.Bag{
				Config: &config.Config{Registries: []config.RegistryConfig{{
					Repo:       "acme/skills",
					Enabled:    true,
					Visibility: visibility,
				}}},
				RepoArg: "acme/skills",
			}
			if err := workflow.StepIndexPublicRegistry(context.Background(), bag); err != nil {
				t.Fatalf("StepIndexPublicRegistry: %v", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(home, ".scribe", "index", "registries.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index stat error = %v, want not exist", err)
	}
}

func TestConnectSteps_StartsWithLoadConfig(t *testing.T) {
	steps := workflow.ConnectSteps()
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
}

func TestConnectSteps_ContainsDedupCheck(t *testing.T) {
	steps := workflow.ConnectSteps()
	for _, s := range steps {
		if s.Name == "DedupCheck" {
			return
		}
	}
	t.Error("ConnectSteps missing DedupCheck step")
}

func TestConnectSteps_DoesNotContainSyncSkills(t *testing.T) {
	for _, s := range workflow.ConnectSteps() {
		if s.Name == "SyncSkills" {
			t.Error("ConnectSteps must not contain SyncSkills — connect is opt-in, not auto-install")
		}
	}
}

func TestConnectInstallAllSteps_ContainsSyncSkills(t *testing.T) {
	steps := workflow.ConnectInstallAllSteps()
	last := steps[len(steps)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected ConnectInstallAllSteps last step SyncSkills, got %s", last.Name)
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected ConnectInstallAllSteps to start with LoadConfig, got %s", steps[0].Name)
	}
}

func TestConnectInstallAllTail_SkipsLoadConfig(t *testing.T) {
	tail := workflow.ConnectInstallAllTail()
	if tail[0].Name == "LoadConfig" {
		t.Error("ConnectInstallAllTail should not start with LoadConfig")
	}
	if tail[0].Name != "ResolveFormatter" {
		t.Errorf("expected ConnectInstallAllTail to start with ResolveFormatter, got %s", tail[0].Name)
	}
}

func TestConnectInstallAllTail_EndsWithSyncSkills(t *testing.T) {
	tail := workflow.ConnectInstallAllTail()
	last := tail[len(tail)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected ConnectInstallAllTail last step SyncSkills, got %s", last.Name)
	}
}

func TestStepInferRegistryTypeSetsVisibilityFromGitHubMetadata(t *testing.T) {
	cases := []struct {
		name       string
		private    bool
		err        error
		want       string
		wantPublic bool
	}{
		{"public repo", false, nil, config.RegistryVisibilityPublic, true},
		{"private repo", true, nil, config.RegistryVisibilityPrivate, false},
		{"metadata error fail closed", false, errors.New("api unavailable"), config.RegistryVisibilityUnknown, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bag := &workflow.Bag{
				Config:     &config.Config{},
				Provider:   connectTestProvider{},
				Visibility: connectVisibilityClient{private: c.private, err: c.err},
				RepoArg:    "acme/skills",
			}

			if err := workflow.StepFetchManifest(context.Background(), bag); err != nil {
				t.Fatalf("StepFetchManifest: %v", err)
			}
			if err := workflow.StepValidateManifest(context.Background(), bag); err != nil {
				t.Fatalf("StepValidateManifest: %v", err)
			}
			if err := workflow.StepInferRegistryType(context.Background(), bag); err != nil {
				t.Fatalf("StepInferRegistryType: %v", err)
			}

			got := bag.Config.Registries[0]
			if got.Visibility != c.want {
				t.Errorf("Visibility = %q, want %q", got.Visibility, c.want)
			}
			if got.IsPublic() != c.wantPublic {
				t.Errorf("IsPublic = %v, want %v", got.IsPublic(), c.wantPublic)
			}
		})
	}
}
