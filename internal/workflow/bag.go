package workflow

import (
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/targets"
)

// Bag carries all intermediate state across workflow steps.
// Each step reads/writes only its relevant fields.
type Bag struct {
	// Inputs (set by cmd/ before Run)
	Args     []string
	JSONFlag bool
	RepoFlag string // --registry filter

	// Create-specific inputs
	Team, Owner, Repo string
	Private           bool
	IsTTY             bool

	// Populated by steps
	Config    *config.Config
	State     *state.State
	Client    *gh.Client
	Targets   []targets.Target
	Repos     []string // filtered registries to process
	Formatter Formatter

	// Connect-specific
	RepoArg string // resolved owner/repo from args or prompt

	// FilterRegistries is injected by cmd/ to bridge flag resolution.
	// If nil, defaults to returning all repos.
	FilterRegistries func(flag string, repos []string) ([]string, error)
}
