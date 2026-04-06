package workflow

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestStepFilterRegistries_OnlyEnabled(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: true},
			{Repo: "acme/disabled-repo", Enabled: false},
			{Repo: "acme/default-repo", Enabled: true},
		},
	}

	b := &Bag{Config: cfg}

	if err := StepFilterRegistries(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(b.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(b.Repos), b.Repos)
	}

	if b.Repos[0] != "acme/team-skills" {
		t.Errorf("repos[0]: got %q, want %q", b.Repos[0], "acme/team-skills")
	}
	if b.Repos[1] != "acme/default-repo" {
		t.Errorf("repos[1]: got %q, want %q", b.Repos[1], "acme/default-repo")
	}
}

func TestStepFilterRegistries_WithFilterFunc(t *testing.T) {
	cfg := &config.Config{
		Registries: []config.RegistryConfig{
			{Repo: "acme/team-skills", Enabled: true},
			{Repo: "acme/other-repo", Enabled: true},
		},
	}

	b := &Bag{
		Config:   cfg,
		RepoFlag: "acme/team-skills",
		FilterRegistries: func(flag string, repos []string) ([]string, error) {
			for _, r := range repos {
				if r == flag {
					return []string{r}, nil
				}
			}
			return repos, nil
		},
	}

	if err := StepFilterRegistries(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(b.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(b.Repos), b.Repos)
	}
	if b.Repos[0] != "acme/team-skills" {
		t.Errorf("repos[0]: got %q, want %q", b.Repos[0], "acme/team-skills")
	}
}
