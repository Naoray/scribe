package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/state"
)

type fakePusher struct {
	tree       []TreeEntry
	head       string
	err        error
	files      map[string][]byte
	expected   string
	pushCalled bool
}

func (f *fakePusher) GetTree(context.Context, string, string, string) ([]TreeEntry, error) {
	return f.tree, f.err
}

func (f *fakePusher) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return f.head, f.err
}

func (f *fakePusher) PushFilesAtomic(_ context.Context, _, _, _ string, files map[string][]byte, _ string, expectedHead string) (CommitResult, error) {
	f.pushCalled = true
	f.files = files
	f.expected = expectedHead
	return CommitResult{SHA: "abc123", URL: "https://github.com/acme/skills/commit/abc123"}, f.err
}

func TestPushSkillPushesDirectoryInSingleCommit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: deploy\ndescription: Deploy things\n---\n")
	writeFile(t, filepath.Join(dir, "scripts", "deploy.sh"), "echo deploy\n")
	writeFile(t, filepath.Join(dir, "versions", "rev-1.md"), "old\n")

	client := &fakePusher{
		tree: []TreeEntry{{Path: "skills/deploy/SKILL.md", Type: "blob", SHA: "base-sha"}},
		head: "head-sha",
	}
	result, err := PushSkill(context.Background(), client, "deploy", dir, state.SkillSource{
		SourceRepo: "acme/skills",
		Path:       "skills/deploy",
		Ref:        "main",
		LastSHA:    "base-sha",
	})
	if err != nil {
		t.Fatalf("PushSkill() error = %v", err)
	}
	if result.CommitSHA != "abc123" || result.Registry != "acme/skills" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if client.expected != "head-sha" {
		t.Fatalf("expected head = %q", client.expected)
	}
	if _, ok := client.files["skills/deploy/SKILL.md"]; !ok {
		t.Fatalf("SKILL.md not pushed: %#v", client.files)
	}
	if _, ok := client.files["skills/deploy/scripts/deploy.sh"]; !ok {
		t.Fatalf("adjacent file not pushed: %#v", client.files)
	}
	if _, ok := client.files["skills/deploy/versions/rev-1.md"]; ok {
		t.Fatalf("versions snapshot should not be pushed: %#v", client.files)
	}
}

func TestPushSkillRefusesRemoteDivergence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: deploy\ndescription: Deploy things\n---\n")
	client := &fakePusher{
		tree: []TreeEntry{{Path: "deploy/SKILL.md", Type: "blob", SHA: "remote-sha"}},
		head: "head-sha",
	}

	_, err := PushSkill(context.Background(), client, "deploy", dir, state.SkillSource{
		Registry: "acme/skills",
		Path:     "deploy",
		Ref:      "main",
		LastSHA:  "base-sha",
	})
	if clierrors.ExitCode(err) != clierrors.ExitConflict {
		t.Fatalf("exit = %d, want conflict; err=%v", clierrors.ExitCode(err), err)
	}
	if client.pushCalled {
		t.Fatal("push should not be called on conflict")
	}
}

func TestPushSkillRefusesPartialRemoteDivergence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: deploy\ndescription: Deploy things\n---\n")
	writeFile(t, filepath.Join(dir, "scripts", "foo.sh"), "echo local\n")

	client := &fakePusher{
		tree: []TreeEntry{
			{Path: "deploy/SKILL.md", Type: "blob", SHA: "base-sha"},
			{Path: "deploy/scripts/foo.sh", Type: "blob", SHA: "remote-script-sha"},
		},
		head: "head-sha",
	}

	_, err := PushSkill(context.Background(), client, "deploy", dir, state.SkillSource{
		Registry: "acme/skills",
		Path:     "deploy",
		Ref:      "main",
		LastSHA:  "base-sha",
		BlobSHAs: map[string]string{
			"deploy/SKILL.md":       "base-sha",
			"deploy/scripts/foo.sh": "base-script-sha",
		},
	})
	if clierrors.ExitCode(err) != clierrors.ExitConflict {
		t.Fatalf("exit = %d, want conflict; err=%v", clierrors.ExitCode(err), err)
	}
	if client.pushCalled {
		t.Fatal("push should not be called on partial divergence")
	}
}

func TestPushSkillPropagatesNetworkFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: deploy\ndescription: Deploy things\n---\n")
	networkErr := clierrors.Wrap(errors.New("offline"), "GH_NETWORK_FAILED", clierrors.ExitNetwork)
	client := &fakePusher{err: networkErr}

	_, err := PushSkill(context.Background(), client, "deploy", dir, state.SkillSource{
		Registry: "acme/skills",
		Path:     "deploy",
		Ref:      "main",
		LastSHA:  "base-sha",
	})
	if clierrors.ExitCode(err) != clierrors.ExitNetwork {
		t.Fatalf("exit = %d, want network; err=%v", clierrors.ExitCode(err), err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
