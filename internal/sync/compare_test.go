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

func installedWithSource(registry, ref, sha string) *state.InstalledSkill {
	return &state.InstalledSkill{
		Revision: 1,
		Sources: []state.SkillSource{{
			Registry: registry,
			Ref:      ref,
			LastSHA:  sha,
		}},
	}
}

func TestCompareEntryMissing(t *testing.T) {
	got := compareEntry(entry("github:a/b@v1.0.0"), nil, "", "a/b")
	if got != StatusMissing {
		t.Errorf("got %s, want %s", got, StatusMissing)
	}
}

func TestCompareEntryCurrentBranch(t *testing.T) {
	inst := installedWithSource("a/b", "main", "abc123")
	got := compareEntry(entry("github:a/b@main"), inst, "abc123", "a/b")
	if got != StatusCurrent {
		t.Errorf("got %s, want %s", got, StatusCurrent)
	}
}

func TestCompareEntryOutdatedBranch(t *testing.T) {
	inst := installedWithSource("a/b", "main", "abc123")
	got := compareEntry(entry("github:a/b@main"), inst, "def456", "a/b")
	if got != StatusOutdated {
		t.Errorf("got %s, want %s", got, StatusOutdated)
	}
}

func TestCompareEntryCurrentTag(t *testing.T) {
	inst := installedWithSource("a/b", "v1.0.0", "")
	got := compareEntry(entry("github:a/b@v1.0.0"), inst, "", "a/b")
	if got != StatusCurrent {
		t.Errorf("got %s, want %s", got, StatusCurrent)
	}
}

func TestCompareEntrySourceLookup(t *testing.T) {
	// Installed from registry "x/y", but looking up "a/b" — should be missing.
	inst := &state.InstalledSkill{
		Revision: 1,
		Sources: []state.SkillSource{{
			Registry: "x/y",
			Ref:      "v1.0.0",
		}},
	}
	got := compareEntry(entry("github:a/b@v1.0.0"), inst, "", "a/b")
	if got != StatusMissing {
		t.Errorf("got %s, want %s", got, StatusMissing)
	}
}

func TestCompareEntry(t *testing.T) {
	cases := []struct {
		name      string
		entry     manifest.Entry
		installed *state.InstalledSkill
		latestSHA string
		registry  string
		want      Status
	}{
		// Not installed
		{"missing", entry("github:a/b@v1.0.0"), nil, "", "a/b", StatusMissing},

		// Semver tags: local ahead → current
		{"semver ahead", entry("github:a/b@v1.0.0"), installedWithSource("a/b", "v1.1.0", ""), "", "a/b", StatusCurrent},
		{"semver same", entry("github:a/b@v1.0.0"), installedWithSource("a/b", "v1.0.0", ""), "", "a/b", StatusCurrent},
		{"semver behind", entry("github:a/b@v1.1.0"), installedWithSource("a/b", "v1.0.0", ""), "", "a/b", StatusOutdated},

		// Non-semver tags: exact match only
		{"non-semver match", entry("github:a/b@v0.12.9.0"), installedWithSource("a/b", "v0.12.9.0", ""), "", "a/b", StatusCurrent},
		{"non-semver mismatch", entry("github:a/b@v0.12.9.0"), installedWithSource("a/b", "v0.12.9.1", ""), "", "a/b", StatusOutdated},

		// Branch refs: SHA comparison
		{"branch same sha", entry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "abc123", "a/b", StatusCurrent},
		{"branch diff sha", entry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "def456", "a/b", StatusOutdated},

		// Branch with empty SHA (API unreachable) → assume current
		{"branch empty sha", entry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "", "a/b", StatusCurrent},

		// Wrong registry → missing
		{"wrong registry", entry("github:a/b@v1.0.0"), installedWithSource("x/y", "v1.0.0", ""), "", "a/b", StatusMissing},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareEntry(c.entry, c.installed, c.latestSHA, c.registry)
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
		registry  string
		want      Status
	}{
		{"package missing", packageEntry("github:a/b@main"), nil, "", "a/b", StatusMissing},
		{"package current", packageEntry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "abc123", "a/b", StatusCurrent},
		{"package outdated", packageEntry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "def456", "a/b", StatusOutdated},
		{"package empty sha assumes current", packageEntry("github:a/b@main"), installedWithSource("a/b", "main", "abc123"), "", "a/b", StatusCurrent},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareEntry(c.entry, c.installed, c.latestSHA, c.registry)
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}
