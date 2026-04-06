package workflow

import (
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// Bag carries all intermediate state across workflow steps.
// Each step reads/writes only its relevant fields.
type Bag struct {
	// Inputs (set by cmd/ before Run)
	Args      []string
	JSONFlag  bool
	LocalFlag bool   // --local: show local inventory only
	RepoFlag  string // --registry filter

	// Populated by steps
	Config    *config.Config
	State     *state.State
	Client    *gh.Client
	Tools     []tools.Tool
	Repos     []string // filtered registries to process
	Formatter Formatter

	// Provider is the skill discovery/fetch backend. Set by StepLoadConfig.
	Provider provider.Provider

	// Connect-specific
	RepoArg string // resolved owner/repo from args or prompt

	// FilterRegistries is injected by cmd/ to bridge flag resolution.
	// If nil, defaults to returning all repos.
	FilterRegistries func(flag string, repos []string) ([]string, error)

	// ListTUI is injected by cmd/ to launch an interactive TUI for local skill list.
	// If nil, falls back to static table output.
	ListTUI func(skills []discovery.Skill) error

	// Results populated by steps for cmd/ to render.
	// List command results:
	LocalSkills    []discovery.Skill               // populated when listing local skills
	RegistryDiffs  map[string][]sync.SkillStatus   // repo → skill statuses (remote list)
	MultiRegistry  bool                            // whether multiple registries are shown

	// Registry list command results:
	RegistryRepos  []string       // connected registries
	RegistryCounts map[string]int // skills per registry

	// Internal fields populated by steps
	manifest *manifest.Manifest
}
