package cmd

import (
	"fmt"
	"strings"
	"unicode"
)

// scaffoldTOML generates the initial scribe.toml content for a new registry.
func scaffoldTOML(team string) string {
	return fmt.Sprintf(`[team]
name = %q
description = "%s dev team skill stack"

# Add skills here. Format:
# "skill-name" = { source = "github:owner/repo@version" }
# "my-skill"   = { source = "github:Owner/repo@main", path = "username/my-skill" }

[skills]
`, team, titleCase(team))
}

// scaffoldREADME generates the initial README.md content for a new registry.
func scaffoldREADME(team, repo string) string {
	title := titleCase(team)
	return fmt.Sprintf(`# %s — Skill Registry

Shared skill registry managed by [Scribe](https://github.com/Naoray/scribe).

## Setup

Install scribe, then connect:

    scribe connect %s

## Sync

Pull the latest skills to your machine:

    scribe sync

## Adding skills

Edit `+"`scribe.toml`"+` to add or update skills, then push to this repo.
Teammates run `+"`scribe sync`"+` to pick up changes.
`, title, repo)
}

// titleCase capitalises the first letter of each segment separated by hyphens.
func titleCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, "-")
}
