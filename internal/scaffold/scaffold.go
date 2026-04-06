package scaffold

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ghNameRe matches valid GitHub owner and repo names (alphanumeric, hyphens, dots, underscores).
var ghNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateGitHubName checks that s is a valid GitHub owner/repo/team name.
func ValidateGitHubName(s, label string) error {
	if !ghNameRe.MatchString(s) {
		return fmt.Errorf("%s %q is invalid: use only letters, numbers, hyphens, dots, or underscores", label, s)
	}
	return nil
}

// TitleCase capitalises the first letter of each segment separated by hyphens.
func TitleCase(s string) string {
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

// TeamDescription returns a human-readable description for a team registry.
func TeamDescription(team string) string {
	return fmt.Sprintf("%s dev team skill stack", TitleCase(team))
}

// ScaffoldYAML generates the initial scribe.yaml content for a new registry.
func ScaffoldYAML(team string) string {
	return fmt.Sprintf(`apiVersion: scribe/v1
kind: Registry
team:
  name: %q
  description: %q

# Add skills here. Example:
# catalog:
#   - name: my-skill
#     source: "github:owner/repo@version"
#     author: username

catalog: []
`, team, TeamDescription(team))
}

// ScaffoldREADME generates the initial README.md content for a new registry.
func ScaffoldREADME(team, repo string) string {
	title := TitleCase(team)
	return fmt.Sprintf(`# %s — Skill Registry

Shared skill registry managed by [Scribe](https://github.com/Naoray/scribe).

## Setup

Install scribe, then connect:

    scribe connect %s

## Sync

Pull the latest skills to your machine:

    scribe sync

## Adding skills

Edit `+"`scribe.yaml`"+` to add or update skills, then push to this repo.
Teammates run `+"`scribe sync`"+` to pick up changes.
`, title, repo)
}
