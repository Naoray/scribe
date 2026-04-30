package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v69/github"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

func TestPushFilesAtomicClassifiesUpdateRefNonFastForwardAsConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/skills/git/ref/heads/main", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("GetRef method = %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ref":"refs/heads/main","object":{"sha":"parent-sha","type":"commit"}}`))
	})
	mux.HandleFunc("/repos/acme/skills/git/commits/parent-sha", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("GetCommit method = %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sha":"parent-sha","tree":{"sha":"base-tree"}}`))
	})
	mux.HandleFunc("/repos/acme/skills/git/blobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("CreateBlob method = %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sha":"blob-sha"}`))
	})
	mux.HandleFunc("/repos/acme/skills/git/trees", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("CreateTree method = %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sha":"tree-sha"}`))
	})
	mux.HandleFunc("/repos/acme/skills/git/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("CreateCommit method = %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sha":"commit-sha","html_url":"https://github.com/acme/skills/commit/commit-sha"}`))
	})
	mux.HandleFunc("/repos/acme/skills/git/refs/heads/main", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("UpdateRef method = %s", r.Method)
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Update is not a fast forward"}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	ghClient := github.NewClient(server.Client())
	ghClient.BaseURL = baseURL
	client := &Client{gh: ghClient, authenticated: true}

	_, err = client.PushFilesAtomic(context.Background(), "acme", "skills", "main", map[string][]byte{
		"deploy/SKILL.md": []byte("# deploy\n"),
	}, "Update deploy skill", "parent-sha")
	if clierrors.ExitCode(err) != clierrors.ExitConflict {
		t.Fatalf("exit = %d, want conflict; err=%v", clierrors.ExitCode(err), err)
	}
}
