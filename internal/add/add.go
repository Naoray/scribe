package add

import (
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/targets"
)

// Candidate represents a skill that can be added to a registry.
type Candidate struct {
	Name      string // skill name (directory basename)
	Origin    string // "local" or "registry:owner/repo"
	Source    string // "github:owner/repo@ref" or empty for local-only
	LocalPath string // absolute path on disk, empty for remote-only
}

// NeedsUpload reports whether this candidate requires uploading files to the
// registry (as opposed to just adding a source reference to scribe.toml).
func (c Candidate) NeedsUpload() bool {
	return c.Source == "" && c.LocalPath != ""
}

// Adder wires discovery and GitHub push together.
// Emits events via the Emit callback — the caller decides output format.
type Adder struct {
	Client  *gh.Client
	Targets []targets.Target
	Emit    func(any)
}

func (a *Adder) emit(msg any) {
	if a.Emit != nil {
		a.Emit(msg)
	}
}
