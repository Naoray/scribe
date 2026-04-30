package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type listTestFetcher struct{}

func (listTestFetcher) FetchFile(ctx context.Context, o, r, p, ref string) ([]byte, error) {
	return nil, nil
}
func (listTestFetcher) FetchDirectory(ctx context.Context, o, r, p, ref string) ([]tools.SkillFile, error) {
	return nil, nil
}
func (listTestFetcher) LatestCommitSHA(ctx context.Context, o, r, b string) (string, error) {
	return "", nil
}
func (listTestFetcher) GetTree(ctx context.Context, o, r, ref string) ([]provider.TreeEntry, error) {
	return nil, nil
}

type panicListProvider struct{}

func (panicListProvider) Discover(context.Context, string) (*provider.DiscoverResult, error) {
	panic("Discover should not be called")
}

func (panicListProvider) Fetch(context.Context, manifest.Entry) ([]tools.SkillFile, error) {
	panic("Fetch should not be called")
}

// partialFailProvider returns skills for one repo and errors for another —
// exercises the per-registry warning path in printMultiListJSON.
type partialFailProvider struct {
	failRepo string
}

func (p *partialFailProvider) Discover(ctx context.Context, repo string) (*provider.DiscoverResult, error) {
	if repo == p.failRepo {
		return nil, fmt.Errorf("%s: no skills found", repo)
	}
	return &provider.DiscoverResult{
		Entries: []manifest.Entry{{
			Name:   "xray",
			Path:   "SKILL.md",
			Source: "github:acme/ok@main",
		}},
		IsTeam: false,
	}, nil
}

func (p *partialFailProvider) Fetch(ctx context.Context, e manifest.Entry) ([]tools.SkillFile, error) {
	return nil, nil
}

func TestListLoadSteps_Composition(t *testing.T) {
	steps := ListLoadStepsLocal()
	if len(steps) != 3 {
		t.Fatalf("ListLoadSteps() = %d steps, want 3", len(steps))
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("step[0] = %s, want LoadConfig", steps[0].Name)
	}
	if steps[1].Name != "LoadState" {
		t.Errorf("step[1] = %s, want LoadState", steps[1].Name)
	}
	if steps[2].Name != "EnsureScribeAgent" {
		t.Errorf("step[2] = %s, want EnsureScribeAgent", steps[2].Name)
	}
}

func TestListLoadStepsRemote_Composition(t *testing.T) {
	steps := ListLoadStepsRemote()
	if len(steps) != 4 {
		t.Fatalf("ListLoadStepsRemote() = %d steps, want 4", len(steps))
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("step[0] = %s, want LoadConfig", steps[0].Name)
	}
	if steps[1].Name != "LoadState" {
		t.Errorf("step[1] = %s, want LoadState", steps[1].Name)
	}
	if steps[2].Name != "ResolveTools" {
		t.Errorf("step[2] = %s, want ResolveTools", steps[2].Name)
	}
	if steps[3].Name != "EnsureScribeAgent" {
		t.Errorf("step[3] = %s, want EnsureScribeAgent", steps[3].Name)
	}
}

func TestListJSONSteps_Composition(t *testing.T) {
	steps := ListJSONSteps()
	if len(steps) == 0 {
		t.Fatal("ListJSONSteps() returned empty list")
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("first step = %s, want LoadConfig", steps[0].Name)
	}
	if steps[len(steps)-1].Name != "WriteListJSON" {
		t.Errorf("last step = %s, want WriteListJSON", steps[len(steps)-1].Name)
	}
}

func TestListLocalPathSkipsGitHubClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	clientCalled := false
	bag := &Bag{
		Factory: &app.Factory{
			Config: func() (*config.Config, error) {
				return &config.Config{}, nil
			},
			Client: func() (*gh.Client, error) {
				clientCalled = true
				return nil, nil
			},
		},
		LazyGitHub: true,
	}

	if err := StepLoadConfig(context.Background(), bag); err != nil {
		t.Fatalf("StepLoadConfig() error = %v", err)
	}

	if clientCalled {
		t.Fatal("StepLoadConfig() called Factory.Client in lazy mode")
	}
	if bag.Client != nil {
		t.Fatalf("bag.Client = %#v, want nil", bag.Client)
	}
}

func TestPrintLocalJSON(t *testing.T) {
	type outputSkill struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Package     string   `json:"package,omitempty"`
		Revision    int      `json:"revision,omitempty"`
		ContentHash string   `json:"content_hash,omitempty"`
		Targets     []string `json:"targets"`
		Managed     bool     `json:"managed"`
		Origin      string   `json:"origin,omitempty"`
		Path        string   `json:"path,omitempty"`
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	storeDir := home + "/.scribe/skills"

	t.Run("unmanaged tool-facing skill", func(t *testing.T) {
		// Path outside ~/.scribe/skills/ → Managed=false in discovery.Skill.
		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills := []discovery.Skill{
			{
				Name:        "tool-skill",
				Description: "A tool-facing skill",
				ContentHash: "abc123",
				Targets:     []string{"claude"},
				LocalPath:   home + "/.claude/skills/tool-skill",
				Managed:     false,
			},
		}

		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON error: %v", err)
		}
		var got []outputSkill
		var wrapper struct {
			Skills   []outputSkill `json:"skills"`
			Packages []any         `json:"packages"`
		}
		if err := json.Unmarshal(buf.Bytes(), &wrapper); err != nil {
			t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
		}
		got = wrapper.Skills
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if got[0].Managed {
			t.Error("tool-skill: expected managed=false")
		}
		if got[0].Origin != "" {
			t.Errorf("tool-skill: expected no origin, got %q", got[0].Origin)
		}
	})

	t.Run("adopted local-origin skill", func(t *testing.T) {
		// State has Origin=OriginLocal + path inside store → managed=true, origin="local".
		st := &state.State{
			Installed: map[string]state.InstalledSkill{
				"adopted-skill": {Revision: 1, Origin: state.OriginLocal},
			},
		}
		skills := []discovery.Skill{
			{
				Name:        "adopted-skill",
				Description: "An adopted skill",
				ContentHash: "def456",
				Targets:     []string{"claude"},
				LocalPath:   storeDir + "/adopted-skill",
				Managed:     true,
			},
		}

		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON error: %v", err)
		}
		var got []outputSkill
		var wrapper struct {
			Skills   []outputSkill `json:"skills"`
			Packages []any         `json:"packages"`
		}
		if err := json.Unmarshal(buf.Bytes(), &wrapper); err != nil {
			t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
		}
		got = wrapper.Skills
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if !got[0].Managed {
			t.Error("adopted-skill: expected managed=true")
		}
		if got[0].Origin != "local" {
			t.Errorf("adopted-skill: expected origin=%q, got %q", "local", got[0].Origin)
		}
	})

	t.Run("registry-sourced skill", func(t *testing.T) {
		// State has empty Origin (OriginRegistry) → managed=true, no origin in JSON.
		st := &state.State{
			Installed: map[string]state.InstalledSkill{
				"registry-skill": {Revision: 3, Origin: state.OriginRegistry},
			},
		}
		skills := []discovery.Skill{
			{
				Name:        "registry-skill",
				Description: "A registry skill",
				Revision:    3,
				ContentHash: "ghi789",
				Targets:     []string{"claude"},
				LocalPath:   storeDir + "/registry-skill",
				Managed:     true,
			},
		}

		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON error: %v", err)
		}
		var got []outputSkill
		var wrapper struct {
			Skills   []outputSkill `json:"skills"`
			Packages []any         `json:"packages"`
		}
		if err := json.Unmarshal(buf.Bytes(), &wrapper); err != nil {
			t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
		}
		got = wrapper.Skills
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if !got[0].Managed {
			t.Error("registry-skill: expected managed=true")
		}
		if got[0].Origin != "" {
			t.Errorf("registry-skill: expected no origin (omitempty), got %q", got[0].Origin)
		}
		if got[0].ContentHash != "ghi789" {
			t.Errorf("registry-skill: content_hash = %q, want %q", got[0].ContentHash, "ghi789")
		}
		if got[0].Revision != 3 {
			t.Errorf("registry-skill: revision = %d, want 3", got[0].Revision)
		}
	})

	t.Run("failing registry is reported as warning, others still listed", func(t *testing.T) {
		syncer := &sync.Syncer{
			Client:   listTestFetcher{},
			Provider: &partialFailProvider{failRepo: "acme/broken"},
			Tools:    []tools.Tool{},
		}
		var buf bytes.Buffer
		_, err := printMultiListJSON(context.Background(), &buf,
			[]string{"acme/broken", "acme/ok"}, syncer, &state.State{
				Installed: map[string]state.InstalledSkill{},
			})
		if err != nil {
			t.Fatalf("printMultiListJSON returned error, want nil: %v", err)
		}
		var decoded struct {
			Registries []struct {
				Registry string `json:"registry"`
			} `json:"registries"`
			Warnings []string `json:"warnings"`
		}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
		}
		if len(decoded.Registries) != 1 || decoded.Registries[0].Registry != "acme/ok" {
			t.Errorf("registries = %+v, want one entry for acme/ok", decoded.Registries)
		}
		if len(decoded.Warnings) != 1 {
			t.Fatalf("warnings = %v, want 1 entry", decoded.Warnings)
		}
		if want := "acme/broken"; !bytes.Contains([]byte(decoded.Warnings[0]), []byte(want)) {
			t.Errorf("warning missing repo name: %q", decoded.Warnings[0])
		}
	})

	t.Run("nil targets become empty array", func(t *testing.T) {
		st := &state.State{Installed: map[string]state.InstalledSkill{}}
		skills := []discovery.Skill{
			{Name: "bare", ContentHash: "x", Targets: nil, Managed: false},
		}
		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON error: %v", err)
		}
		var got []outputSkill
		var wrapper struct {
			Skills   []outputSkill `json:"skills"`
			Packages []any         `json:"packages"`
		}
		if err := json.Unmarshal(buf.Bytes(), &wrapper); err != nil {
			t.Fatalf("json.Unmarshal error: %v\nraw: %s", err, buf.String())
		}
		got = wrapper.Skills
		if got[0].Targets == nil {
			t.Error("targets should be [] not null")
		}
		if len(got[0].Targets) != 0 {
			t.Errorf("targets = %v, want empty array", got[0].Targets)
		}
	})

	t.Run("packages surface in dedicated section", func(t *testing.T) {
		st := &state.State{Installed: map[string]state.InstalledSkill{
			"gstack": {
				Revision:   2,
				Kind:       state.KindPackage,
				InstallCmd: "./setup",
				Sources:    []state.SkillSource{{Registry: "Naoray/gstack"}},
			},
			"plain": {Revision: 1},
		}}
		skills := []discovery.Skill{
			{Name: "plain", ContentHash: "h", Targets: []string{"claude"}, Managed: true},
			// gstack intentionally absent from discovery — packages live
			// under ~/.scribe/packages/ which plain list_test doesn't stage.
		}
		var buf bytes.Buffer
		if err := printLocalJSON(&buf, skills, st); err != nil {
			t.Fatalf("printLocalJSON: %v", err)
		}
		var out struct {
			Skills []struct {
				Name string `json:"name"`
			} `json:"skills"`
			Packages []struct {
				Name       string `json:"name"`
				Revision   int    `json:"revision"`
				InstallCmd string `json:"install_cmd"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
		}
		if len(out.Skills) != 1 || out.Skills[0].Name != "plain" {
			t.Errorf("skills = %+v, want just plain", out.Skills)
		}
		if len(out.Packages) != 1 || out.Packages[0].Name != "gstack" {
			t.Fatalf("packages = %+v, want gstack", out.Packages)
		}
		if out.Packages[0].InstallCmd != "./setup" {
			t.Errorf("install_cmd = %q", out.Packages[0].InstallCmd)
		}
	})
}

func TestBuildLocalRows_HidesBootstrapOrigin(t *testing.T) {
	// Regression for #487: scribe-agent (Origin=Bootstrap) is auto-managed
	// by the CLI and shouldn't appear in `scribe list`. It can't be removed
	// — the next invocation re-installs it — so listing it just clutters.
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"my-skill":     {Origin: state.OriginRegistry},
		"scribe-agent": {Origin: state.OriginBootstrap},
	}}
	skills := []discovery.Skill{
		{Name: "my-skill", Managed: true},
		{Name: "scribe-agent", Managed: true},
	}

	rows := BuildLocalRows(skills, st)
	if len(rows) != 1 {
		t.Fatalf("BuildLocalRows returned %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].Name != "my-skill" {
		t.Fatalf("row name = %q, want my-skill", rows[0].Name)
	}
}

func TestBuildLocalRows_OnlyBootstrapReturnsEmpty(t *testing.T) {
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"scribe-agent": {Origin: state.OriginBootstrap},
	}}
	skills := []discovery.Skill{
		{Name: "scribe-agent", Managed: true},
	}

	rows := BuildLocalRows(skills, st)
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want empty", rows)
	}
}

func TestBuildLocalRowsExcluding_FiltersBootstrapAfterMatching(t *testing.T) {
	// BuildLocalRowsExcluding delegates to BuildLocalRows after dropping
	// matched entries. Verify the bootstrap filter survives the exclusion
	// path (regression for #487 second-order coverage).
	st := &state.State{Installed: map[string]state.InstalledSkill{
		"matched":      {Origin: state.OriginRegistry},
		"my-skill":     {Origin: state.OriginRegistry},
		"scribe-agent": {Origin: state.OriginBootstrap},
	}}
	skills := []discovery.Skill{
		{Name: "matched", Managed: true},
		{Name: "my-skill", Managed: true},
		{Name: "scribe-agent", Managed: true},
	}
	matched := map[string]bool{"matched": true}

	rows := BuildLocalRowsExcluding(skills, matched, st)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want only my-skill", rows)
	}
	if rows[0].Name != "my-skill" {
		t.Fatalf("row[0].Name = %q, want my-skill", rows[0].Name)
	}
}
