package cmd

import (
	"context"
	"testing"

	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/registry"
)

type fakeLockfilePusher struct {
	expectedHead string
	files        map[string][]byte
}

func (f *fakeLockfilePusher) LatestCommitSHA(context.Context, string, string, string) (string, error) {
	return "head-sha", nil
}

func (f *fakeLockfilePusher) PushFilesAtomic(_ context.Context, _, _, _ string, files map[string][]byte, _ string, expectedHead string) (registry.CommitResult, error) {
	f.expectedHead = expectedHead
	f.files = files
	return registry.CommitResult{SHA: "commit-sha", URL: "https://github.com/acme/registry/commit/commit-sha"}, nil
}

func TestPushLockfileWritesScribeLockAtomically(t *testing.T) {
	pusher := &fakeLockfilePusher{}
	result, err := pushLockfile(context.Background(), pusher, "acme/registry", &lockfile.Lockfile{
		FormatVersion: lockfile.SchemaVersion,
		Registry:      "acme/registry",
		Entries: []lockfile.Entry{{
			Name:           "deploy",
			SourceRegistry: "acme/source",
			CommitSHA:      "sha",
			ContentHash:    "hash",
		}},
	})
	if err != nil {
		t.Fatalf("pushLockfile() error = %v", err)
	}
	if result.SHA != "commit-sha" || pusher.expectedHead != "head-sha" {
		t.Fatalf("unexpected result=%+v expectedHead=%q", result, pusher.expectedHead)
	}
	if string(pusher.files[lockfile.Filename]) == "" {
		t.Fatalf("scribe.lock was not pushed: %#v", pusher.files)
	}
}
