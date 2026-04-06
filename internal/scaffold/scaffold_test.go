package scaffold

import (
	"strings"
	"testing"
)

func TestScaffoldYAML(t *testing.T) {
	cases := []struct {
		name     string
		team     string
		wantTeam string
		wantDesc string
	}{
		{
			name:     "simple name",
			team:     "artistfy",
			wantTeam: `name: "artistfy"`,
			wantDesc: `description: "Artistfy dev team skill stack"`,
		},
		{
			name:     "hyphenated name",
			team:     "my-team",
			wantTeam: `name: "my-team"`,
			wantDesc: `description: "My-Team dev team skill stack"`,
		},
		{
			name:     "already capitalized",
			team:     "DevOps",
			wantTeam: `name: "DevOps"`,
			wantDesc: `description: "DevOps dev team skill stack"`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScaffoldYAML(c.team)
			if !strings.Contains(got, c.wantTeam) {
				t.Errorf("missing team name line %q in:\n%s", c.wantTeam, got)
			}
			if !strings.Contains(got, c.wantDesc) {
				t.Errorf("missing description line %q in:\n%s", c.wantDesc, got)
			}
			if !strings.Contains(got, "apiVersion: scribe/v1") {
				t.Error("missing apiVersion")
			}
			if !strings.Contains(got, "kind: Registry") {
				t.Error("missing kind")
			}
			if !strings.Contains(got, "catalog:") {
				t.Error("missing catalog section")
			}
		})
	}
}

func TestScaffoldREADME(t *testing.T) {
	got := ScaffoldREADME("artistfy", "ArtistfyHQ/team-skills")
	if !strings.Contains(got, "# Artistfy — Skill Registry") {
		t.Errorf("missing title in:\n%s", got)
	}
	if !strings.Contains(got, "scribe connect ArtistfyHQ/team-skills") {
		t.Errorf("missing connect command in:\n%s", got)
	}
}

func TestValidateGitHubName(t *testing.T) {
	valid := []string{"artistfy", "my-team", "DevOps", "user.name", "a123", "my_org"}
	for _, name := range valid {
		if err := ValidateGitHubName(name, "test"); err != nil {
			t.Errorf("ValidateGitHubName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "-start", ".start", "has space", "foo/bar", "new\nline", "tab\there"}
	for _, name := range invalid {
		if err := ValidateGitHubName(name, "test"); err == nil {
			t.Errorf("ValidateGitHubName(%q) = nil, want error", name)
		}
	}
}

func TestTitleCase(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"artistfy", "Artistfy"},
		{"my-team", "My-Team"},
		{"DevOps", "DevOps"},
		{"a", "A"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := TitleCase(c.input)
			if got != c.want {
				t.Errorf("TitleCase(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}
