package manifest_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
)

const teamLoadout = `
[team]
name = "artistfy"
description = "Artistfy dev team skill stack"

[skills]
"gstack"       = { source = "github:garrytan/gstack@v0.12.9.0" }
"laravel-init" = { source = "github:Naoray/scribe-skills@v1.0.0", path = "skills/laravel-init" }
"deploy"       = { source = "github:ArtistfyHQ/team-skills@main", path = "krishan/deploy" }
"frontend-prs" = { source = "github:ArtistfyHQ/team-skills@main", path = "markus/frontend-prs", private = true }

[targets]
default = ["claude", "cursor"]
`

const packageManifest = `
[package]
name = "scribe-skills"
version = "1.0.0"
description = "Shared skills for the Artistfy team"
license = "MIT"
authors = ["Krishan <krishan@artistfy.com>"]

[skills]
laravel-init = "skills/laravel-init/SKILL.md"
code-review  = "skills/code-review/SKILL.md"
`

func TestParseTeamLoadout(t *testing.T) {
	m, err := manifest.Parse([]byte(teamLoadout))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !m.IsLoadout() {
		t.Error("expected IsLoadout() = true")
	}
	if m.Package != nil {
		t.Error("expected Package == nil for team loadout")
	}
	if m.Team.Name != "artistfy" {
		t.Errorf("team name: got %q, want %q", m.Team.Name, "artistfy")
	}
	if len(m.Skills) != 4 {
		t.Errorf("skills count: got %d, want 4", len(m.Skills))
	}

	gstack := m.Skills["gstack"]
	if gstack.Source != "github:garrytan/gstack@v0.12.9.0" {
		t.Errorf("gstack source: got %q", gstack.Source)
	}
	if gstack.Maintainer() != "garrytan" {
		t.Errorf("gstack maintainer: got %q, want garrytan", gstack.Maintainer())
	}

	deploy := m.Skills["deploy"]
	if deploy.Path != "krishan/deploy" {
		t.Errorf("deploy path: got %q", deploy.Path)
	}
	if deploy.Maintainer() != "krishan" {
		t.Errorf("deploy maintainer: got %q, want krishan", deploy.Maintainer())
	}

	frontend := m.Skills["frontend-prs"]
	if !frontend.Private {
		t.Error("frontend-prs: expected private = true")
	}
	if frontend.Maintainer() != "markus" {
		t.Errorf("frontend-prs maintainer: got %q, want markus", frontend.Maintainer())
	}

	if len(m.Targets.Default) != 2 {
		t.Errorf("targets: got %v, want [claude cursor]", m.Targets.Default)
	}
}

func TestParsePackageManifest(t *testing.T) {
	m, err := manifest.Parse([]byte(packageManifest))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if m.Package == nil {
		t.Error("expected Package != nil for package manifest")
	}
	if m.Package.Name != "scribe-skills" {
		t.Errorf("package name: got %q", m.Package.Name)
	}

	skill := m.Skills["laravel-init"]
	if skill.Path != "skills/laravel-init/SKILL.md" {
		t.Errorf("laravel-init path: got %q", skill.Path)
	}
}

func TestParseSource(t *testing.T) {
	cases := []struct {
		raw      string
		owner    string
		repo     string
		ref      string
		isBranch bool
	}{
		{"github:garrytan/gstack@v0.12.9.0", "garrytan", "gstack", "v0.12.9.0", false},
		{"github:Naoray/scribe-skills@v1.0.0", "Naoray", "scribe-skills", "v1.0.0", false},
		{"github:ArtistfyHQ/team-skills@main", "ArtistfyHQ", "team-skills", "main", true},
	}

	for _, c := range cases {
		src, err := manifest.ParseSource(c.raw)
		if err != nil {
			t.Errorf("ParseSource(%q): %v", c.raw, err)
			continue
		}
		if src.Owner != c.owner {
			t.Errorf("ParseSource(%q).Owner = %q, want %q", c.raw, src.Owner, c.owner)
		}
		if src.Repo != c.repo {
			t.Errorf("ParseSource(%q).Repo = %q, want %q", c.raw, src.Repo, c.repo)
		}
		if src.Ref != c.ref {
			t.Errorf("ParseSource(%q).Ref = %q, want %q", c.raw, src.Ref, c.ref)
		}
		if src.IsBranch() != c.isBranch {
			t.Errorf("ParseSource(%q).IsBranch() = %v, want %v", c.raw, src.IsBranch(), c.isBranch)
		}
		if src.String() != c.raw {
			t.Errorf("ParseSource(%q).String() = %q", c.raw, src.String())
		}
	}
}

func TestParseSourceErrors(t *testing.T) {
	cases := []string{
		"garrytan/gstack@v1.0.0",  // missing host
		"github:garrytan/gstack",  // missing @ref
		"github:gstack@v1.0.0",    // missing slash in repo
		"npm:garrytan/gstack@v1",  // unsupported host
	}
	for _, raw := range cases {
		if _, err := manifest.ParseSource(raw); err == nil {
			t.Errorf("ParseSource(%q): expected error, got nil", raw)
		}
	}
}

const invalidBothSections = `
[team]
name = "artistfy"

[package]
name = "scribe-skills"
version = "1.0.0"
`

func TestValidateMutualExclusivity(t *testing.T) {
	_, err := manifest.Parse([]byte(invalidBothSections))
	if err == nil {
		t.Fatal("expected error when both [team] and [package] are present")
	}
	want := "manifest cannot have both [team] and [package] sections"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseInitializesSkillsMap(t *testing.T) {
	m, err := manifest.Parse([]byte(`[team]
name = "empty"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Skills == nil {
		t.Error("expected Skills map to be initialized, got nil")
	}
}
