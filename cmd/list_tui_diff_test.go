package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

type previewStubProvider struct {
	content []byte
	err     error
	calls   int
}

func (p *previewStubProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	return nil, nil
}

func (p *previewStubProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return []tools.SkillFile{{Path: "SKILL.md", Content: p.content}}, nil
}

func withPreviewDeps(t *testing.T) {
	t.Helper()
	original := listEnsureRemoteDepsFn
	listEnsureRemoteDepsFn = func(context.Context, *workflow.Bag) error { return nil }
	t.Cleanup(func() { listEnsureRemoteDepsFn = original })
}

func newPreviewModel(t *testing.T, upstream []byte, fetchErr error) (listModel, *previewStubProvider) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(home, ".scribe", "skills", "recap")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := []byte("# Skill\n\nbase\n")
	local := []byte("# Skill\n\nlocal\n")
	if err := os.WriteFile(filepath.Join(skillDir, ".scribe-base.md"), base, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), local, 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &previewStubProvider{content: upstream, err: fetchErr}
	row := listRow{
		Name:      "recap",
		Group:     "owner/repo",
		Status:    sync.StatusOutdated,
		HasStatus: true,
		Entry:     &manifest.Entry{Name: "recap", Source: "github:owner/repo@main"},
		Local:     &discovery.Skill{Name: "recap", LocalPath: skillDir},
		Managed:   true,
	}
	m := listModel{
		ctx:         context.Background(),
		stage:       stageBrowse,
		selected:    true,
		focus:       focusActions,
		width:       120,
		height:      40,
		rows:        []listRow{row},
		filtered:    []listRow{row},
		groupCounts: map[string]int{"owner/repo": 1},
		bag: &workflow.Bag{
			Config:   &config.Config{},
			State:    &state.State{Installed: map[string]state.InstalledSkill{"recap": {InstalledHash: sync.ComputeFileHash(base)}}},
			Provider: provider,
		},
		viewYours:    newDiffViewport(80, 8),
		viewIncoming: newDiffViewport(80, 8),
	}
	return m, provider
}

func runPreviewFetch(t *testing.T, cmd tea.Cmd) upstreamPreviewMsg {
	t.Helper()
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("preview command returned %T, want tea.BatchMsg", msg)
	}
	for _, c := range batch {
		got := c()
		if preview, ok := got.(upstreamPreviewMsg); ok {
			return preview
		}
	}
	t.Fatal("batch did not contain upstreamPreviewMsg")
	return upstreamPreviewMsg{}
}

func TestListTUIDiffPreviewPopulatesViewports(t *testing.T) {
	withPreviewDeps(t)
	m, _ := newPreviewModel(t, []byte("# Skill\n\nincoming\n"), nil)
	nm, cmd := m.executeAction("update")
	if cmd == nil {
		t.Fatal("expected preview fetch command")
	}
	lm := nm.(listModel)
	if !lm.updatePreview.loading {
		t.Fatal("preview should start loading")
	}
	updated, _ := lm.Update(runPreviewFetch(t, cmd))
	lm = updated.(listModel)
	if lm.updatePreview.loading {
		t.Fatal("preview should stop loading after fetch")
	}
	if !strings.Contains(lm.updatePreview.diffYours, "-base") || !strings.Contains(lm.updatePreview.diffYours, "+local") {
		t.Fatalf("yours diff missing local change:\n%s", lm.updatePreview.diffYours)
	}
	if !strings.Contains(lm.updatePreview.diffIncoming, "+incoming") {
		t.Fatalf("incoming diff missing upstream change:\n%s", lm.updatePreview.diffIncoming)
	}
	if !strings.Contains(lm.viewIncoming.View(), "@@") {
		t.Fatalf("incoming viewport missing unified hunk:\n%s", lm.viewIncoming.View())
	}
}

func TestListTUIDiffKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want func(t *testing.T, m listModel, cmd tea.Cmd)
	}{
		{
			name: "tab toggles viewport",
			key:  "tab",
			want: func(t *testing.T, m listModel, _ tea.Cmd) {
				if m.activeViewport != viewportIncoming {
					t.Fatalf("activeViewport = %v, want incoming", m.activeViewport)
				}
			},
		},
		{
			name: "j scrolls",
			key:  "j",
			want: func(t *testing.T, _ listModel, cmd tea.Cmd) {
				if cmd != nil {
					t.Fatal("scroll should not return async command")
				}
			},
		},
		{
			name: "keep local clears substate",
			key:  "l",
			want: func(t *testing.T, m listModel, cmd tea.Cmd) {
				if cmd != nil {
					t.Fatal("keep local should not return command")
				}
				if m.substate != listSubstateNone {
					t.Fatalf("substate = %v, want none", m.substate)
				}
			},
		},
		{
			name: "merge starts update",
			key:  "m",
			want: func(t *testing.T, m listModel, cmd tea.Cmd) {
				if cmd == nil {
					t.Fatal("merge should return update command")
				}
				if m.substate != listSubstateNone {
					t.Fatalf("substate = %v, want none", m.substate)
				}
			},
		},
		{
			name: "replace starts update",
			key:  "r",
			want: func(t *testing.T, _ listModel, cmd tea.Cmd) {
				if cmd == nil {
					t.Fatal("replace should return update command")
				}
			},
		},
		{
			name: "escape clears substate",
			key:  "esc",
			want: func(t *testing.T, m listModel, cmd tea.Cmd) {
				if cmd != nil {
					t.Fatal("escape should not return command")
				}
				if m.substate != listSubstateNone {
					t.Fatalf("substate = %v, want none", m.substate)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newPreviewModel(t, []byte("# Skill\n\nincoming\n"), nil)
			m.substate = listSubstateUpdateChoice
			m.updateHasMods = true
			m.updatePreview.requestID = 1
			nm, cmd := m.updateUpdateChoice(key(tt.key))
			tt.want(t, nm.(listModel), cmd)
		})
	}
}

func TestListTUIDiffStaleResponseDropped(t *testing.T) {
	m, _ := newPreviewModel(t, []byte("# Skill\n\nincoming\n"), nil)
	m.substate = listSubstateUpdateChoice
	m.updatePreview.requestID = 2
	m.updatePreview.rowName = "recap"
	updated, _ := m.Update(upstreamPreviewMsg{requestID: 1, rowName: "recap", skillMD: []byte("wrong")})
	lm := updated.(listModel)
	if lm.updatePreview.diffIncoming != "" {
		t.Fatalf("stale response populated incoming diff: %q", lm.updatePreview.diffIncoming)
	}
}

func TestListTUIDiffOfflineGatesRegistryChoices(t *testing.T) {
	withPreviewDeps(t)
	m, _ := newPreviewModel(t, nil, errors.New("offline"))
	nm, cmd := m.executeAction("update")
	lm := nm.(listModel)
	updated, _ := lm.Update(runPreviewFetch(t, cmd))
	lm = updated.(listModel)
	if lm.updatePreview.err == nil {
		t.Fatal("expected preview error")
	}
	out := lm.renderDetailPane(lm.filtered[lm.cursor], 80)
	if !strings.Contains(out, "Could not reach registry") {
		t.Fatalf("missing offline render:\n%s", out)
	}
	nm, cmd = lm.updateUpdateChoice(key("m"))
	lm = nm.(listModel)
	if cmd != nil {
		t.Fatal("offline merge should not start update")
	}
	if !strings.Contains(lm.statusMsg, "Registry unavailable") {
		t.Fatalf("statusMsg = %q", lm.statusMsg)
	}
}

func TestListTUIDiffConflictExistsRoutesResolve(t *testing.T) {
	withPreviewDeps(t)
	m, provider := newPreviewModel(t, []byte("# Skill\n\nincoming\n"), nil)
	if err := os.WriteFile(filepath.Join(m.filtered[0].Local.LocalPath, "SKILL.md"), []byte("<<<<<<< HEAD\nlocal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nm, cmd := m.executeAction("update")
	lm := nm.(listModel)
	if cmd != nil {
		t.Fatal("conflict marker path should not fetch upstream")
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
	if lm.substate != listSubstateUpdateConflictExists {
		t.Fatalf("substate = %v, want conflict-exists", lm.substate)
	}
	_, cmd = lm.updateUpdateConflictExists(key("r"))
	if cmd == nil {
		t.Fatal("resolve key should dispatch process command")
	}
}

func TestListTUIDiffSmokeUpdateChoiceRenderAndKeepLocal(t *testing.T) {
	withPreviewDeps(t)
	m, _ := newPreviewModel(t, []byte("# Skill\n\nincoming\n"), nil)
	nm, cmd := m.executeAction("update")
	lm := nm.(listModel)
	updated, _ := lm.Update(runPreviewFetch(t, cmd))
	lm = updated.(listModel)
	out := lm.renderDetailPane(lm.filtered[lm.cursor], 120)
	if !strings.Contains(out, "@@") {
		t.Fatalf("smoke render missing diff hunk:\n%s", out)
	}
	nm, cmd = lm.updateUpdateChoice(key("l"))
	if cmd != nil {
		t.Fatal("keep local should not start command")
	}
	if nm.(listModel).substate != listSubstateNone {
		t.Fatal("keep local should clear update substate")
	}
}
