package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type connectFakeGitHubClient struct {
	files map[string][]byte
}

func (c connectFakeGitHubClient) FetchFile(_ context.Context, owner, repo, path, _ string) ([]byte, error) {
	key := owner + "/" + repo + "/" + path
	if body, ok := c.files[key]; ok {
		return body, nil
	}
	return nil, errors.New("not found")
}

func (connectFakeGitHubClient) FetchDirectory(context.Context, string, string, string, string) ([]tools.SkillFile, error) {
	return nil, errors.New("not implemented")
}

func (connectFakeGitHubClient) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "abc123", nil
}

func (connectFakeGitHubClient) GetTree(context.Context, string, string, string) ([]provider.TreeEntry, error) {
	return nil, errors.New("not implemented")
}

func (connectFakeGitHubClient) HasPushAccess(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestConnectMaterialisesRegistryKits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	fakeClient := connectFakeGitHubClient{files: map[string][]byte{
		"Naoray/skills/scribe.yaml": []byte(`apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: plan-my-day
    source: github:Naoray/skills@HEAD
    path: skills/plan-my-day
kits:
  - name: daily-workflow
    path: kits/daily-workflow.yaml
`),
		"Naoray/skills/kits/daily-workflow.yaml": []byte(`apiVersion: scribe/v1
kind: Kit
name: daily-workflow
skills: [plan-my-day]
`),
	}}
	restore := installConnectFakeFactory(t, fakeClient)
	defer restore()

	cmd := newConnectCommand()
	cmd.SetArgs([]string{"Naoray/skills"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	kitPath := filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")
	k, err := kit.Load(kitPath)
	if err != nil {
		t.Fatalf("load kit: %v", err)
	}
	if k.Source == nil || k.Source.Registry != "Naoray/skills" {
		t.Fatalf("kit source = %+v", k.Source)
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	got, ok := st.Kits["daily-workflow"]
	if !ok {
		t.Fatalf("state.Kits missing daily-workflow: %+v", st.Kits)
	}
	if got.Source != "Naoray/skills" {
		t.Fatalf("state.Kits[daily-workflow].Source = %q, want Naoray/skills", got.Source)
	}
	if got.Version != "HEAD" {
		t.Fatalf("state.Kits[daily-workflow].Version = %q, want HEAD", got.Version)
	}
}

func installConnectFakeFactory(t *testing.T, client connectFakeGitHubClient) func() {
	t.Helper()
	old := commandFactory
	commandFactory = func() *app.Factory {
		f := app.NewFactory()
		f.Client = func() (*gh.Client, error) { return nil, nil }
		f.Provider = func() (provider.Provider, error) {
			return provider.NewGitHubProvider(client), nil
		}
		return f
	}
	return func() { commandFactory = old }
}
