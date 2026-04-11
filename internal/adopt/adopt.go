// Package adopt detects unmanaged skills in tool-facing directories and imports
// them into the canonical Scribe store (~/.scribe/skills/).
package adopt

import (
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

// Candidate is an unmanaged skill that adoption can import.
type Candidate struct {
	Name       string   // skill name (dir base)
	LocalPath  string   // source dir on disk
	Targets    []string // tool names where this skill was discovered (e.g. ["claude"])
	Hash       string   // content hash (blob SHA of SKILL.md)
	reLinkOnly bool     // internal: skip WriteToStore; just re-link the existing canonical store
}

// Conflict describes a name collision between an unmanaged skill and a managed one.
type Conflict struct {
	Name      string
	Managed   state.InstalledSkill
	Unmanaged Candidate
}

// Plan describes what Apply will do when invoked. Pure data — no I/O.
type Plan struct {
	Adopt     []Candidate // ready to adopt
	Conflicts []Conflict  // unresolved; must pass through Resolve to adopt
}

// Decision is the user's choice for a single conflict.
type Decision int

const (
	// DecisionSkip drops the conflict — nothing is adopted.
	DecisionSkip Decision = iota
	// DecisionOverwriteManaged imports the unmanaged skill into the store,
	// bumping the managed revision.
	DecisionOverwriteManaged
	// DecisionReplaceUnmanaged re-links the unmanaged path to the managed
	// store entry (refreshes symlink, does not overwrite canonical content).
	DecisionReplaceUnmanaged
)

// Result summarizes what Apply did.
type Result struct {
	Adopted []string
	Skipped []string
	Failed  map[string]error
}

// Adopter performs adoption.
type Adopter struct {
	State *state.State
	Tools []tools.Tool // tools to consider (typically tools.DefaultTools())
	Emit  func(any)   // optional; nil is safe
}

// FindCandidates walks configured adoption paths (via cfg.AdoptionPaths())
// and returns candidates and conflicts. Never mutates state.
func FindCandidates(st *state.State, cfg config.AdoptionConfig) ([]Candidate, []Conflict, error) {
	return findCandidates(st, cfg)
}

// Resolve folds user decisions into the plan, producing the final set
// of candidates to adopt. Unresolved conflicts are filtered out.
func Resolve(p Plan, decisions map[string]Decision) []Candidate {
	out := make([]Candidate, 0, len(p.Adopt)+len(p.Conflicts))

	// Pass through clean candidates unchanged.
	out = append(out, p.Adopt...)

	// Apply decisions to conflicts.
	for _, c := range p.Conflicts {
		d, ok := decisions[c.Name]
		if !ok {
			d = DecisionSkip
		}
		switch d {
		case DecisionOverwriteManaged:
			out = append(out, c.Unmanaged)
		case DecisionReplaceUnmanaged:
			cand := c.Unmanaged
			cand.reLinkOnly = true
			out = append(out, cand)
		default: // DecisionSkip
			// drop
		}
	}

	return out
}

// emit safely calls a.Emit if non-nil.
func (a *Adopter) emit(msg any) {
	if a.Emit != nil {
		a.Emit(msg)
	}
}
