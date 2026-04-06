package migrate_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/migrate"
)

const teamTOML = `
[team]
name = "artistfy"
description = "Artistfy dev team skill stack"

[skills]
"gstack"       = { source = "github:garrytan/gstack@v0.12.9.0" }
"laravel-init" = { source = "github:Naoray/scribe-skills@v1.0.0", path = "skills/laravel-init" }
"deploy"       = { source = "github:ArtistfyHQ/team-skills@main", path = "krishan/deploy" }

[targets]
default = ["claude", "cursor"]
`

func TestConvertTeamLoadout(t *testing.T) {
	result, err := migrate.Convert([]byte(teamTOML))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if result.APIVersion != "scribe/v1" {
		t.Errorf("apiVersion: got %q", result.APIVersion)
	}
	if result.Kind != "Registry" {
		t.Errorf("kind: got %q", result.Kind)
	}
	if result.Team == nil || result.Team.Name != "artistfy" {
		t.Error("team not preserved")
	}
	if len(result.Catalog) != 3 {
		t.Fatalf("catalog count: got %d, want 3", len(result.Catalog))
	}

	// Catalog should be sorted alphabetically.
	if result.Catalog[0].Name != "deploy" {
		t.Errorf("first entry: got %q, want deploy", result.Catalog[0].Name)
	}
	if result.Catalog[1].Name != "gstack" {
		t.Errorf("second entry: got %q, want gstack", result.Catalog[1].Name)
	}
	if result.Catalog[2].Name != "laravel-init" {
		t.Errorf("third entry: got %q, want laravel-init", result.Catalog[2].Name)
	}

	// Check author inference.
	deploy := result.FindByName("deploy")
	if deploy == nil {
		t.Fatal("deploy not found")
	}
	if deploy.Author != "krishan" {
		t.Errorf("deploy author: got %q, want krishan", deploy.Author)
	}
	gstack := result.FindByName("gstack")
	if gstack == nil {
		t.Fatal("gstack not found")
	}
	if gstack.Author != "garrytan" {
		t.Errorf("gstack author: got %q, want garrytan", gstack.Author)
	}
}

const packageTOML = `
[package]
name = "scribe-skills"
version = "1.0.0"

[skills]
laravel-init = "skills/laravel-init/SKILL.md"
code-review  = "skills/code-review/SKILL.md"
`

func TestConvertPackageManifest(t *testing.T) {
	result, err := migrate.Convert([]byte(packageTOML))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if result.Kind != "Package" {
		t.Errorf("kind: got %q", result.Kind)
	}
	if result.Package == nil || result.Package.Name != "scribe-skills" {
		t.Error("package not preserved")
	}
	if len(result.Catalog) != 2 {
		t.Fatalf("catalog count: got %d, want 2", len(result.Catalog))
	}

	li := result.FindByName("laravel-init")
	if li == nil {
		t.Fatal("laravel-init not found")
	}
	if li.Path != "skills/laravel-init/SKILL.md" {
		t.Errorf("path: got %q", li.Path)
	}
}
