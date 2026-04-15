package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	gogithub "github.com/google/go-github/v69/github"
)

type fakeUpgradeAgentClient struct {
	authenticated bool
	tag           string
	content       []byte
}

func (f fakeUpgradeAgentClient) LatestRelease(context.Context, string, string) (*gogithub.RepositoryRelease, error) {
	return &gogithub.RepositoryRelease{TagName: gogithub.Ptr(f.tag)}, nil
}

func (f fakeUpgradeAgentClient) FetchFile(context.Context, string, string, string, string) ([]byte, error) {
	return f.content, nil
}

func (f fakeUpgradeAgentClient) IsAuthenticated() bool {
	return f.authenticated
}

func TestUpgradeAgentCommandRefreshesFromNetwork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	if err := runUpgradeAgentWithDeps(context.Background(), cfg, st, fakeUpgradeAgentClient{
		authenticated: true,
		tag:           "v1.2.3",
		content:       []byte("---\nname: scribe-agent\ndescription: test\n---\nbody\n"),
	}); err != nil {
		t.Fatalf("runUpgradeAgentWithDeps() error = %v", err)
	}

	storePath := filepath.Join(os.Getenv("HOME"), ".scribe", "skills", "scribe-agent", "SKILL.md")
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("expected store file at %s: %v", storePath, err)
	}
	if got := st.Installed["scribe-agent"].Sources[0].Ref; got != "v1.2.3" {
		t.Fatalf("source ref = %q, want v1.2.3", got)
	}
}

func TestUpgradeAgentCommandRejectsInvalidSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{}
	st := &state.State{Installed: map[string]state.InstalledSkill{}}

	err := runUpgradeAgentWithDeps(context.Background(), cfg, st, fakeUpgradeAgentClient{
		authenticated: true,
		tag:           "v1.2.3",
		content:       []byte("---\nname: wrong\n---\n"),
	})
	if err == nil {
		t.Fatal("runUpgradeAgentWithDeps() error = nil, want validation error")
	}
}

func TestUpgradeAgentCommandRequiresAuth(t *testing.T) {
	err := runUpgradeAgentWithDeps(context.Background(), &config.Config{}, &state.State{Installed: map[string]state.InstalledSkill{}}, fakeUpgradeAgentClient{
		authenticated: false,
		tag:           "v1.2.3",
		content:       []byte("---\nname: scribe-agent\n---\n"),
	})
	if !errors.Is(err, errAuthRequired) {
		t.Fatalf("err = %v, want errAuthRequired", err)
	}
}

func TestEnsureScribeAgentNotCalledByVersionCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := newRootCmd()
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--version execute: %v", err)
	}

	storePath := filepath.Join(home, ".scribe", "skills", "scribe-agent", "SKILL.md")
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("version command should not bootstrap scribe-agent; stat err = %v", err)
	}
}
