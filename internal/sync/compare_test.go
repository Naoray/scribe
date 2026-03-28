package sync

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

func skill(source string) manifest.Skill {
	return manifest.Skill{Source: source}
}

func installed(version, sha string) *state.InstalledSkill {
	return &state.InstalledSkill{Version: version, CommitSHA: sha}
}

func TestCompareSkill(t *testing.T) {
	cases := []struct {
		name      string
		skill     manifest.Skill
		installed *state.InstalledSkill
		latestSHA string
		want      Status
	}{
		// Not installed
		{"missing", skill("github:a/b@v1.0.0"), nil, "", StatusMissing},

		// Semver tags: local ahead → current
		{"semver ahead", skill("github:a/b@v1.0.0"), installed("v1.1.0", ""), "", StatusCurrent},
		{"semver same", skill("github:a/b@v1.0.0"), installed("v1.0.0", ""), "", StatusCurrent},
		{"semver behind", skill("github:a/b@v1.1.0"), installed("v1.0.0", ""), "", StatusOutdated},

		// Non-semver tags: exact match only
		{"non-semver match", skill("github:a/b@v0.12.9.0"), installed("v0.12.9.0", ""), "", StatusCurrent},
		{"non-semver mismatch", skill("github:a/b@v0.12.9.0"), installed("v0.12.9.1", ""), "", StatusOutdated},

		// Branch refs: SHA comparison
		{"branch same sha", skill("github:a/b@main"), installed("main", "abc123"), "abc123", StatusCurrent},
		{"branch diff sha", skill("github:a/b@main"), installed("main", "abc123"), "def456", StatusOutdated},
		{"branch no sha yet", skill("github:a/b@main"), installed("main", ""), "", StatusOutdated},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareSkill(c.skill, c.installed, c.latestSHA)
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}
