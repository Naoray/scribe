package workflow

import (
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

// Bag carries all intermediate state across workflow steps.
// Each step reads/writes only its relevant fields.
type Bag struct {
	// Inputs (set by cmd/ before Run)
	Args         []string
	JSONFlag     bool
	RepoFlag     string // --registry filter
	RemoteFlag   bool   // --remote: show available skills from registries
	BrowseFlag   bool   // browse mode: remote catalog UI with install-first actions
	InitialQuery string // initial search/filter text for TUI surfaces
	TrustAllFlag bool   // --trust-all: approve all package commands without prompting
	LazyGitHub   bool   // skip eager GitHub client/provider setup for local-only flows
	Factory      *app.Factory

	// Populated by steps
	Config     *config.Config
	State      *state.State
	Client     *gh.Client
	Tools      []tools.Tool
	Repos      []string // filtered registries to process
	Formatter  Formatter
	StateDirty bool

	// Provider is the skill discovery/fetch backend. Set by StepLoadConfig.
	Provider provider.Provider

	// Connect-specific
	RepoArg string // resolved owner/repo from args or prompt

	// FilterRegistries is injected by cmd/ to bridge flag resolution.
	// If nil, defaults to returning all repos.
	FilterRegistries func(flag string, repos []string) ([]string, error)

	// Results populated by steps for cmd/ to render.
	// List command results (JSON path only — TUI loads its own data):
	LocalSkills   []discovery.Skill             // populated when listing local skills
	RegistryDiffs map[string][]sync.SkillStatus // repo → skill statuses (remote list)
	MultiRegistry bool                          // whether multiple registries are shown

	// Registry list command results:
	RegistryRepos  []string       // connected registries
	RegistryCounts map[string]int // skills per registry

	// Internal fields populated by steps
	manifest *manifest.Manifest
}

func (b *Bag) MarkStateDirty() {
	if b != nil {
		b.StateDirty = true
	}
}
