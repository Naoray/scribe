package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/state"
)

type fakePushAuth struct {
	authed bool
	user   gh.AuthenticatedUser
}

func (f fakePushAuth) IsAuthenticated() bool { return f.authed }

func (f fakePushAuth) AuthenticatedUser(context.Context) (gh.AuthenticatedUser, error) {
	return f.user, nil
}

type fakeRegistryPusher struct{}

func (fakeRegistryPusher) GetTree(context.Context, string, string, string) ([]registry.TreeEntry, error) {
	return []registry.TreeEntry{{Path: "deploy/SKILL.md", Type: "blob", SHA: "base-sha"}}, nil
}

func (fakeRegistryPusher) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "head-sha", nil
}

func (fakeRegistryPusher) PushFilesAtomic(context.Context, string, string, string, map[string][]byte, string, string) (registry.CommitResult, error) {
	return registry.CommitResult{SHA: "abc123", URL: "https://github.com/acme/skills/commit/abc123"}, nil
}

func TestRunPushWithDepsEnforcesAuthor(t *testing.T) {
	home := setupPushHome(t, "deploy", "---\nname: deploy\ndescription: Deploy things\n---\n")
	t.Setenv("HOME", home)
	st := pushState("deploy", "author@example.com")

	_, err := runPushWithDeps(context.Background(), "deploy", st, fakePushAuth{
		authed: true,
		user:   gh.AuthenticatedUser{Login: "someone", Emails: []string{"other@example.com"}},
	}, fakeRegistryPusher{}, func(context.Context) string { return "other@example.com" })

	if clierrors.ExitCode(err) != clierrors.ExitPerm {
		t.Fatalf("exit = %d, want perm; err=%v", clierrors.ExitCode(err), err)
	}
}

func TestRunPushWithDepsRequiresAuth(t *testing.T) {
	home := setupPushHome(t, "deploy", "---\nname: deploy\ndescription: Deploy things\n---\n")
	t.Setenv("HOME", home)
	st := pushState("deploy", "author@example.com")

	_, err := runPushWithDeps(context.Background(), "deploy", st, fakePushAuth{}, fakeRegistryPusher{}, func(context.Context) string {
		return "author@example.com"
	})

	if clierrors.ExitCode(err) != clierrors.ExitPerm {
		t.Fatalf("exit = %d, want perm; err=%v", clierrors.ExitCode(err), err)
	}
}

func TestRunPushWithDepsValidatesSkillMetadata(t *testing.T) {
	home := setupPushHome(t, "deploy", "---\nname: Deploy\ndescription: Deploy things\n---\n")
	t.Setenv("HOME", home)
	st := pushState("deploy", "author@example.com")

	_, err := runPushWithDeps(context.Background(), "deploy", st, fakePushAuth{
		authed: true,
		user:   gh.AuthenticatedUser{Emails: []string{"author@example.com"}},
	}, fakeRegistryPusher{}, func(context.Context) string { return "" })

	if clierrors.ExitCode(err) != clierrors.ExitValid {
		t.Fatalf("exit = %d, want valid; err=%v", clierrors.ExitCode(err), err)
	}
}

func TestRunPushWithDepsReturnsPushResult(t *testing.T) {
	home := setupPushHome(t, "deploy", "---\nname: deploy\ndescription: Deploy things\n---\n")
	t.Setenv("HOME", home)
	st := pushState("deploy", "author@example.com")

	result, err := runPushWithDeps(context.Background(), "deploy", st, fakePushAuth{
		authed: true,
		user:   gh.AuthenticatedUser{Login: "author", Emails: []string{"author@example.com"}},
	}, fakeRegistryPusher{}, func(context.Context) string { return "" })
	if err != nil {
		t.Fatalf("runPushWithDeps() error = %v", err)
	}
	if result.CommitSHA != "abc123" || result.Registry != "acme/skills" || result.Skill != "deploy" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func setupPushHome(t *testing.T, name, skillMD string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".scribe", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return home
}

func pushState(name, author string) *state.State {
	return &state.State{Installed: map[string]state.InstalledSkill{
		name: {
			Revision:      1,
			InstalledHash: "hash",
			Sources: []state.SkillSource{{
				Registry:   "acme/skills",
				Path:       name,
				Author:     author,
				Ref:        "main",
				LastSHA:    "base-sha",
				LastSynced: time.Now().UTC(),
			}},
		},
	}}
}
