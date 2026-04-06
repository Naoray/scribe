package manifest

import (
	"fmt"
	"strings"
)

// hostGitHub is the source host identifier for GitHub repositories.
const hostGitHub = "github"

// Source is a parsed skill source reference.
// Format: "github:owner/repo@ref"
// Ref can be a semver tag (v1.2.3), a non-semver tag (v0.12.9.0), or a branch (main).
type Source struct {
	Host  string // always "github" for now
	Owner string
	Repo  string
	Ref   string // tag or branch name
}

// ParseSource parses a source string like "github:owner/repo@ref".
func ParseSource(raw string) (Source, error) {
	host, rest, ok := strings.Cut(raw, ":")
	if !ok {
		return Source{}, fmt.Errorf("invalid source %q: missing host prefix (expected github:owner/repo@ref)", raw)
	}
	if host != hostGitHub {
		return Source{}, fmt.Errorf("unsupported source host %q (only github is supported)", host)
	}

	repoRef, ref, ok := strings.Cut(rest, "@")
	if !ok {
		return Source{}, fmt.Errorf("invalid source %q: missing @ref (expected github:owner/repo@ref)", raw)
	}

	owner, repo, ok := strings.Cut(repoRef, "/")
	if !ok {
		return Source{}, fmt.Errorf("invalid source %q: missing repo (expected github:owner/repo@ref)", raw)
	}

	return Source{Host: host, Owner: owner, Repo: repo, Ref: ref}, nil
}

// String reassembles the source into its canonical string form.
func (s Source) String() string {
	return fmt.Sprintf("%s:%s/%s@%s", s.Host, s.Owner, s.Repo, s.Ref)
}

// IsBranch reports whether the ref looks like a branch name rather than a version tag.
// Tags follow semver-ish conventions (start with "v" and contain dots).
func (s Source) IsBranch() bool {
	return !strings.HasPrefix(s.Ref, "v") || !strings.Contains(s.Ref, ".")
}
