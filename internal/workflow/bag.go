package workflow

import (
	"context"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/registryindex"
	"github.com/Naoray/scribe/internal/source"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type RepositoryVisibilityClient interface {
	RepositoryIsPrivate(ctx context.Context, owner, repo string) (bool, error)
}

// Bag carries all intermediate state across workflow steps.
// Each step reads/writes only its relevant fields.
type Bag struct {
	// Inputs (set by cmd/ before Run)
	Args               []string
	JSONFlag           bool
	RepoFlag           string // --registry filter
	RemoteFlag         bool   // --remote: show available skills from registries
	BrowseFlag         bool   // browse mode: remote catalog UI with install-first actions
	KitBrowseFlag      bool   // browse mode shows registry kits instead of skills
	InitialQuery       string // initial search/filter text for TUI surfaces
	TrustAllFlag       bool   // --trust-all: approve all package commands without prompting
	InstallAllFlag     bool   // --all: install all available skills without prompting
	ForceBudget        bool   // deprecated --force no-op for budget guardrails
	ForceKits          bool   // --force-kits: overwrite existing kit files from connect/resync
	RefreshKits        bool   // --refresh-kits: opt into kit refresh during registry resync
	AliasName          string // --alias: install incoming skill under another name on projection conflict
	SkillAliases       map[string]string
	PinnedSkillSources map[string]string
	LazyGitHub         bool // skip eager GitHub client/provider setup for local-only flows
	Factory            *app.Factory

	// SkillFilter is populated by StepSelectSkills with the names the user chose.
	// If non-empty, StepSyncSkills passes it to the Syncer so only those skills
	// are installed. Nil means no filter (all eligible skills processed).
	SkillFilter []string

	// KitFilter is populated by StepResolveKitFilter from the project's
	// .scribe.yaml. When non-nil, StepSyncSkills passes it to the Syncer so
	// only kit-resolved skills are projected into the project's tool dirs.
	KitFilter        []string
	KitFilterEnabled bool

	// ProjectMCPServers is populated by StepResolveMCPServers from kit-declared
	// MCP server names. It is read-only workflow state for future projection;
	// no agent settings are written from this value yet.
	ProjectMCPServers        []string
	ProjectMCPServersEnabled bool

	ProjectSnippets []string

	// Populated by steps
	Config        *config.Config
	State         *state.State
	Client        *gh.Client
	Visibility    RepositoryVisibilityClient
	RegistryIndex registryindex.MetadataClient
	Tools         []tools.Tool
	ProjectRoot   string
	TeamShareMode bool
	Repos         []string // filtered registries to process
	Formatter     Formatter
	StateDirty    bool

	// Provider is the skill discovery/fetch backend. Set by StepLoadConfig.
	Provider provider.Provider

	// Connect-specific
	RepoArg   string // resolved owner/repo from args or prompt
	SourceArg source.SourceSpec
	SourceKey string
	SourceID  string

	// FilterRegistries is injected by cmd/ to bridge flag resolution.
	// If nil, defaults to returning all repos.
	FilterRegistries func(flag string, repos []string) ([]string, error)

	// Results populated by steps for cmd/ to render.
	// List command results (JSON path only — TUI loads its own data):
	LocalSkills   []discovery.Skill             // populated when listing local skills
	RegistryDiffs map[string][]sync.SkillStatus // repo → skill statuses (remote list)
	MultiRegistry bool                          // whether multiple registries are shown
	Partial       bool                          // true when a mutating workflow completed with failures

	// Registry list command results:
	RegistryRepos   []string                // connected registries
	RegistryConfigs []config.RegistryConfig // connected registry config rows
	RegistryCounts  map[string]int          // skills per registry

	// Registry-published kits populated by connect/resync.
	Kits          []provider.KitFile
	KitsInstalled []string

	// Internal fields populated by steps
	manifest *manifest.Manifest
}

func (b *Bag) MarkStateDirty() {
	if b != nil {
		b.StateDirty = true
	}
}
