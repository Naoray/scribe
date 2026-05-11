package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/source"
)

func TestNewCommandsExposeTypedSourceFlags(t *testing.T) {
	for _, cmd := range []*cobraCommandLike{
		{name: "add", cmd: newAddCommand(), wantsRegistry: true},
		{name: "browse", cmd: newBrowseCommand(), wantsRegistry: true},
		{name: "connect", cmd: newConnectCommand(), wantsRegistry: false},
	} {
		t.Run(cmd.name, func(t *testing.T) {
			for _, flag := range []string{"source", "repo", "url", "ref", "path", "id"} {
				if cmd.cmd.Flags().Lookup(flag) == nil {
					t.Fatalf("missing --%s", flag)
				}
			}
			if got := cmd.cmd.Flags().Lookup("registry") != nil; got != cmd.wantsRegistry {
				t.Fatalf("--registry present = %v, want %v", got, cmd.wantsRegistry)
			}
		})
	}
}

type cobraCommandLike struct {
	name          string
	cmd           *cobra.Command
	wantsRegistry bool
}

func TestSourceSpecFromFlagsBuildsTypedGitHubSource(t *testing.T) {
	spec, ident, display, err := sourceSpecFromFlags(sourceFlagValues{
		repo: "owner/repo",
		ref:  "v1.2.3",
		path: "skills",
		id:   "team-skills",
	})
	if err != nil {
		t.Fatalf("sourceSpecFromFlags: %v", err)
	}
	if spec.Type != source.SourceGitHub || spec.Repo != "owner/repo" || spec.Ref != "v1.2.3" || spec.Path != "skills" || spec.ID != "team-skills" {
		t.Fatalf("spec = %#v", spec)
	}
	if ident.Key != "github:owner/repo:skills" {
		t.Fatalf("key = %q", ident.Key)
	}
	if display != "team-skills" {
		t.Fatalf("display = %q", display)
	}
}

func TestSourceSpecFromFlagsBuildsProviderTypedGitHubSource(t *testing.T) {
	spec, ident, display, err := sourceSpecFromFlags(sourceFlagValues{
		source: "github",
		repo:   "vercel-labs/agent-skills",
		ref:    "main",
		path:   "skills",
		id:     "vercel",
	})
	if err != nil {
		t.Fatalf("sourceSpecFromFlags: %v", err)
	}
	if spec.Type != source.SourceGitHub || spec.Repo != "vercel-labs/agent-skills" || spec.Ref != "main" || spec.Path != "skills" || spec.ID != "vercel" {
		t.Fatalf("spec = %#v", spec)
	}
	if ident.Key != "github:vercel-labs/agent-skills:skills" {
		t.Fatalf("key = %q", ident.Key)
	}
	if display != "vercel" {
		t.Fatalf("display = %q", display)
	}
}

func TestAddSourceFlagConnectedIDIsFilterNotRawLocator(t *testing.T) {
	cfg := &config.Config{Registries: []config.RegistryConfig{{
		ID:      "vercel",
		Enabled: true,
		Source: &source.SourceSpec{
			Type: source.SourceGitHub,
			Repo: "vercel-labs/agent-skills",
			Ref:  "main",
			Path: "skills",
		},
	}}}
	flags := sourceFlagValues{source: "vercel"}

	if !sourceFlagsMatchConnectedSource(flags, cfg) {
		t.Fatal("--source vercel should resolve as a connected source filter")
	}
	sources, err := browseSources(flags.source, cfg)
	if err != nil {
		t.Fatalf("browseSources: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != "vercel" || sources[0].Source.Repo != "vercel-labs/agent-skills" || sources[0].Source.Path != "skills" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestParseInstallRefForCommandKeepsLegacyAndBracketSyntax(t *testing.T) {
	spec, ident, display, skill, err := parseInstallRefForCommand("owner/repo:deploy")
	if err != nil {
		t.Fatalf("legacy parse: %v", err)
	}
	if spec.Repo != "owner/repo" || ident.Key != "owner/repo" || display != "owner/repo" || skill != "deploy" {
		t.Fatalf("legacy parse = spec:%#v ident:%#v display:%q skill:%q", spec, ident, display, skill)
	}

	spec, ident, display, skill, err = parseInstallRefForCommand("[https://github.com/owner/repo/tree/main/skills]:deploy")
	if err != nil {
		t.Fatalf("bracket parse: %v", err)
	}
	if spec.Repo != "owner/repo" || spec.Ref != "main" || spec.Path != "skills" || ident.Key != "github:owner/repo:skills" || display != "github:owner/repo:skills" || skill != "deploy" {
		t.Fatalf("bracket parse = spec:%#v ident:%#v display:%q skill:%q", spec, ident, display, skill)
	}
}

func TestParseInstallRefForCommandRejectsAmbiguousURLRef(t *testing.T) {
	_, _, _, _, err := parseInstallRefForCommand("https://github.com/owner/repo:deploy")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "[source]:skill or --source") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestBrowseSourcesAcceptsTypedSourceURL(t *testing.T) {
	sources, err := browseSources("https://github.com/owner/repo/tree/main/skills", &config.Config{})
	if err != nil {
		t.Fatalf("browseSources: %v", err)
	}
	if len(sources) != 1 || sources[0].Source.Repo != "owner/repo" || sources[0].Source.Ref != "main" || sources[0].Source.Path != "skills" {
		t.Fatalf("sources = %#v", sources)
	}
}
