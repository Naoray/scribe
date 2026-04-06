package sync

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
)

func entry(source string) manifest.Entry {
	return manifest.Entry{Source: source}
}

func packageEntry(source string) manifest.Entry {
	return manifest.Entry{Source: source, Type: manifest.EntryTypePackage}
}

func installed(version, sha string) *state.InstalledSkill {
	return &state.InstalledSkill{Version: version, CommitSHA: sha}
}

func TestCompareEntry(t *testing.T) {
	cases := []struct {
		name      string
		entry     manifest.Entry
		installed *state.InstalledSkill
		latestSHA string
		want      Status
	}{
		// Not installed
		{"missing", entry("github:a/b@v1.0.0"), nil, "", StatusMissing},

		// Semver tags: local ahead → current
		{"semver ahead", entry("github:a/b@v1.0.0"), installed("v1.1.0", ""), "", StatusCurrent},
		{"semver same", entry("github:a/b@v1.0.0"), installed("v1.0.0", ""), "", StatusCurrent},
		{"semver behind", entry("github:a/b@v1.1.0"), installed("v1.0.0", ""), "", StatusOutdated},

		// Non-semver tags: exact match only
		{"non-semver match", entry("github:a/b@v0.12.9.0"), installed("v0.12.9.0", ""), "", StatusCurrent},
		{"non-semver mismatch", entry("github:a/b@v0.12.9.0"), installed("v0.12.9.1", ""), "", StatusOutdated},

		// Branch refs: SHA comparison
		{"branch same sha", entry("github:a/b@main"), installed("main", "abc123"), "abc123", StatusCurrent},
		{"branch diff sha", entry("github:a/b@main"), installed("main", "abc123"), "def456", StatusOutdated},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareEntry(c.entry, c.installed, c.latestSHA)
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestComparePackage(t *testing.T) {
	cases := []struct {
		name      string
		entry     manifest.Entry
		installed *state.InstalledSkill
		latestSHA string
		want      Status
	}{
		{"package missing", packageEntry("github:a/b@main"), nil, "", StatusMissing},
		{"package current", packageEntry("github:a/b@main"), installed("main", "abc123"), "abc123", StatusCurrent},
		{"package outdated", packageEntry("github:a/b@main"), installed("main", "abc123"), "def456", StatusOutdated},
		{"package empty sha assumes current", packageEntry("github:a/b@main"), installed("main", "abc123"), "", StatusCurrent},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareEntry(c.entry, c.installed, c.latestSHA)
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}
