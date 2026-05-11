package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/add"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/tools"
)

type registryAddFakeTool struct {
	name string
}

func (t registryAddFakeTool) Name() string { return t.name }

func (t registryAddFakeTool) Install(string, string, string) ([]string, error) {
	return nil, nil
}

func (t registryAddFakeTool) Uninstall(string) error { return nil }

func (t registryAddFakeTool) Detect() bool { return true }

func (t registryAddFakeTool) SkillPath(string, string) (string, error) {
	return "", nil
}

func (t registryAddFakeTool) CanonicalTarget(string) (string, bool) {
	return "", false
}

func TestCollectInstallCommandsUsesActiveTargets(t *testing.T) {
	targets := []tools.Tool{
		registryAddFakeTool{name: "aider"},
		registryAddFakeTool{name: "gemini"},
	}

	installs, err := collectInstallCommands("acme/superpowers", []string{
		"aider=aider install superpowers",
		"gemini=gemini skills install superpowers",
	}, targets, false)
	if err != nil {
		t.Fatalf("collectInstallCommands: %v", err)
	}

	if got := installs["aider"]; got != "aider install superpowers" {
		t.Fatalf("aider install = %q", got)
	}
	if got := installs["gemini"]; got != "gemini skills install superpowers" {
		t.Fatalf("gemini install = %q", got)
	}

	_, err = collectInstallCommands("acme/superpowers", []string{
		"cursor=/plugin install superpowers",
	}, targets, false)
	if err == nil {
		t.Fatal("expected inactive cursor install to be rejected")
	}
	if !strings.Contains(err.Error(), `unknown tool "cursor"`) {
		t.Fatalf("error = %q, want unknown cursor", err.Error())
	}
	if !strings.Contains(err.Error(), "aider, gemini") {
		t.Fatalf("error = %q, want active target names", err.Error())
	}
}

func TestPushBarePackageRefEmitsVisibleAndJSONResults(t *testing.T) {
	oldFetch := fetchRegistryManifestForPackageRef
	oldPush := pushRegistryFilesForPackageRef
	t.Cleanup(func() {
		fetchRegistryManifestForPackageRef = oldFetch
		pushRegistryFilesForPackageRef = oldPush
	})

	fetchRegistryManifestForPackageRef = func(context.Context, *gh.Client, string, string) (*manifest.Manifest, error) {
		return &manifest.Manifest{
			APIVersion: "scribe/v1",
			Kind:       "Registry",
			Team:       &manifest.Team{Name: "acme"},
		}, nil
	}

	var pushed string
	pushRegistryFilesForPackageRef = func(_ context.Context, _ *gh.Client, owner, repo string, files map[string]string, message string) error {
		if owner != "acme" || repo != "registry" {
			t.Fatalf("push repo = %s/%s, want acme/registry", owner, repo)
		}
		if message != "add package: superpowers" {
			t.Fatalf("message = %q", message)
		}
		pushed = files[manifest.ManifestFilename]
		return nil
	}

	adder := &add.Adder{Client: &gh.Client{}}
	results := wireAddEmit(adder, "acme/registry", false)

	out := captureStdout(t, func() {
		err := pushBarePackageRef(t.Context(), adder, "acme/registry", "obra/superpowers", map[string]string{
			"gemini": "gemini skills install superpowers",
		})
		if err != nil {
			t.Fatalf("pushBarePackageRef: %v", err)
		}
	})

	if !strings.Contains(out, "adding reference superpowers") {
		t.Fatalf("stdout missing adding event: %q", out)
	}
	if !strings.Contains(out, "✓ superpowers added to acme/registry") {
		t.Fatalf("stdout missing success event: %q", out)
	}
	if !strings.Contains(pushed, "gemini: gemini skills install superpowers") {
		t.Fatalf("pushed manifest missing install command:\n%s", pushed)
	}

	adder = &add.Adder{Client: &gh.Client{}}
	results = wireAddEmit(adder, "acme/registry", true)
	if err := pushBarePackageRef(t.Context(), adder, "acme/registry", "obra/superpowers", map[string]string{
		"gemini": "gemini skills install superpowers",
	}); err != nil {
		t.Fatalf("pushBarePackageRef json mode: %v", err)
	}
	if len(*results) != 1 {
		t.Fatalf("results len = %d, want 1", len(*results))
	}
	if (*results)[0].Name != "superpowers" || (*results)[0].Registry != "acme/registry" {
		t.Fatalf("result = %+v", (*results)[0])
	}

	payload, err := json.Marshal(map[string]any{
		"+":      *results,
		"synced": false,
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"name":"superpowers"`)) {
		t.Fatalf("json payload missing package result: %s", payload)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return buf.String()
}
