package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func TestRunBrowseWithDeps_JSONQueryFiltersResults(t *testing.T) {
	old := discoverEntriesFn
	defer func() { discoverEntriesFn = old }()
	discoverEntriesFn = func(context.Context, []string, *gh.Client, []tools.Tool, *state.State) ([]browseEntry, []error) {
		return []browseEntry{
			{Registry: "acme/skills", Status: sync.SkillStatus{Name: "cleanup", Status: sync.StatusMissing}},
			{Registry: "acme/skills", Status: sync.SkillStatus{Name: "deploy", Status: sync.StatusMissing}},
		}, nil
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = runBrowseWithDeps(context.Background(), []string{"acme/skills"}, "clean", "", nil, &state.State{Installed: map[string]state.InstalledSkill{}}, nil, nil, true, true)
	w.Close()
	if err != nil {
		t.Fatalf("runBrowseWithDeps() error = %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	var out struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal browse json: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Name != "cleanup" {
		t.Fatalf("results = %+v, want only cleanup", out.Results)
	}
}

func TestBrowseInstallRejectsAmbiguousName(t *testing.T) {
	err := browseInstall(context.Background(), "cleanup", []browseEntry{
		{Registry: "acme/skills", Status: sync.SkillStatus{Name: "cleanup"}},
		{Registry: "other/skills", Status: sync.SkillStatus{Name: "cleanup"}},
	}, nil, nil, nil, nil, true, true)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("browseInstall() error = %v, want ambiguous error", err)
	}
}

func TestBrowseInstallRejectsMissingName(t *testing.T) {
	err := browseInstall(context.Background(), "cleanup", nil, nil, nil, nil, nil, true, true)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("browseInstall() error = %v, want not found error", err)
	}
}
