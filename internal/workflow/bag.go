package workflow

import (
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
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

	// Connect-specific
	RepoArg string // resolved owner/repo from args or prompt

	// FilterRegistries is injected by cmd/ to bridge flag resolution.
	// If nil, defaults to returning all repos.
	FilterRegistries func(flag string, repos []string) ([]string, error)

	// ListTUI is injected by cmd/ to launch an interactive TUI for local skill list.
	// If nil, falls back to static table output.
	ListTUI func(skills []discovery.Skill) error

	// Internal fields populated by steps
	manifest *manifest.Manifest
}
