package manifest_test

import (
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
)

const teamRegistry = `
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
  description: Artistfy dev team skill stack
catalog:
  - name: gstack
    source: "github:garrytan/gstack@v0.12.9.0"
    author: garrytan
  - name: laravel-init
    source: "github:Naoray/scribe-skills@v1.0.0"
    path: skills/laravel-init
    author: krishan
  - name: deploy
    source: "github:ArtistfyHQ/team-skills@main"
    path: krishan/deploy
    author: krishan
  - name: superpowers
    source: "github:obra/superpowers@main"
    type: package
    install: /plugin install superpowers@claude-plugins-official
    author: obra
targets:
  default:
    - claude
    - cursor
`

const packageManifest = `
apiVersion: scribe/v1
kind: Package
package:
  name: scribe-skills
  version: "1.0.0"
  description: Shared skills for the Artistfy team
  license: MIT
  authors:
    - Krishan
  repository: github.com/Naoray/scribe-skills
catalog:
  - name: laravel-init
    path: skills/laravel-init
  - name: code-review
    path: skills/code-review
`

func TestParseTeamRegistry(t *testing.T) {
	m, err := manifest.Parse([]byte(teamRegistry))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !m.IsRegistry() {
		t.Error("expected IsRegistry() = true")
	}
	if !m.IsLoadout() {
		t.Error("expected IsLoadout() = true (backward compat)")
	}
	if m.Package != nil {
		t.Error("expected Package == nil for team registry")
	}
	if m.Team.Name != "artistfy" {
		t.Errorf("team name: got %q, want %q", m.Team.Name, "artistfy")
	}
	if len(m.Catalog) != 4 {
		t.Fatalf("catalog count: got %d, want 4", len(m.Catalog))
	}

	gstack := m.FindByName("gstack")
	if gstack == nil {
		t.Fatal("FindByName(gstack) returned nil")
	}
	if gstack.Source != "github:garrytan/gstack@v0.12.9.0" {
		t.Errorf("gstack source: got %q", gstack.Source)
	}
	if gstack.Author != "garrytan" {
		t.Errorf("gstack author: got %q, want garrytan", gstack.Author)
	}

	laravelInit := m.FindByName("laravel-init")
	if laravelInit == nil {
		t.Fatal("FindByName(laravel-init) returned nil")
	}
	if laravelInit.Source != "github:Naoray/scribe-skills@v1.0.0" {
		t.Errorf("laravel-init source: got %q", laravelInit.Source)
	}
	if laravelInit.Path != "skills/laravel-init" {
		t.Errorf("laravel-init path: got %q", laravelInit.Path)
	}
	if laravelInit.Author != "krishan" {
		t.Errorf("laravel-init author: got %q, want krishan", laravelInit.Author)
	}

	deploy := m.FindByName("deploy")
	if deploy == nil {
		t.Fatal("FindByName(deploy) returned nil")
	}
	if deploy.Path != "krishan/deploy" {
		t.Errorf("deploy path: got %q", deploy.Path)
	}
	if deploy.Author != "krishan" {
		t.Errorf("deploy author: got %q, want krishan", deploy.Author)
	}

	superpowers := m.FindByName("superpowers")
	if superpowers == nil {
		t.Fatal("FindByName(superpowers) returned nil")
	}
	if !superpowers.IsPackage() {
		t.Error("superpowers: expected IsPackage() = true")
	}
	if superpowers.Install != "/plugin install superpowers@claude-plugins-official" {
		t.Errorf("superpowers install: got %q", superpowers.Install)
	}
	if superpowers.Author != "obra" {
		t.Errorf("superpowers author: got %q, want obra", superpowers.Author)
	}

	if len(m.Targets.Default) != 2 {
		t.Errorf("targets: got %v, want [claude cursor]", m.Targets.Default)
	}
	if m.Targets.Default[0] != "claude" || m.Targets.Default[1] != "cursor" {
		t.Errorf("targets: got %v, want [claude cursor]", m.Targets.Default)
	}
}

func TestParsePackageManifest(t *testing.T) {
	m, err := manifest.Parse([]byte(packageManifest))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !m.IsPackage() {
		t.Error("expected IsPackage() = true")
	}
	if m.Package.Name != "scribe-skills" {
		t.Errorf("package name: got %q", m.Package.Name)
	}
	if m.Package.Version != "1.0.0" {
		t.Errorf("package version: got %q", m.Package.Version)
	}
	if len(m.Catalog) != 2 {
		t.Fatalf("catalog count: got %d, want 2", len(m.Catalog))
	}

	laravelInit := m.FindByName("laravel-init")
	if laravelInit == nil {
		t.Fatal("FindByName(laravel-init) returned nil")
	}
	if laravelInit.Path != "skills/laravel-init" {
		t.Errorf("laravel-init path: got %q", laravelInit.Path)
	}

	codeReview := m.FindByName("code-review")
	if codeReview == nil {
		t.Fatal("FindByName(code-review) returned nil")
	}
	if codeReview.Path != "skills/code-review" {
		t.Errorf("code-review path: got %q", codeReview.Path)
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

func TestManifestEncode(t *testing.T) {
	m, err := manifest.Parse([]byte(teamRegistry))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Round-trip: re-parse the encoded output.
	m2, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}

	if m2.Team.Name != m.Team.Name {
		t.Errorf("team name: got %q, want %q", m2.Team.Name, m.Team.Name)
	}
	if len(m2.Catalog) != len(m.Catalog) {
		t.Fatalf("catalog count: got %d, want %d", len(m2.Catalog), len(m.Catalog))
	}

	// Verify catalog order is preserved.
	for i, entry := range m.Catalog {
		got := m2.Catalog[i]
		if got.Name != entry.Name {
			t.Errorf("catalog[%d] name: got %q, want %q", i, got.Name, entry.Name)
		}
		if got.Source != entry.Source {
			t.Errorf("catalog[%d] source: got %q, want %q", i, got.Source, entry.Source)
		}
		if got.Path != entry.Path {
			t.Errorf("catalog[%d] path: got %q, want %q", i, got.Path, entry.Path)
		}
		if got.Author != entry.Author {
			t.Errorf("catalog[%d] author: got %q, want %q", i, got.Author, entry.Author)
		}
		if got.Type != entry.Type {
			t.Errorf("catalog[%d] type: got %q, want %q", i, got.Type, entry.Type)
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

func TestValidateMutualExclusivity(t *testing.T) {
	input := `
apiVersion: scribe/v1
kind: Registry
team:
  name: artistfy
package:
  name: scribe-skills
  version: "1.0.0"
`
	_, err := manifest.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error when both team and package are present")
	}
	want := "manifest cannot have both team and package sections"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestValidateAPIVersion(t *testing.T) {
	input := `
apiVersion: scribe/v2
kind: Registry
team:
  name: test
`
	_, err := manifest.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unsupported apiVersion")
	}
	if want := `unsupported apiVersion "scribe/v2" (expected scribe/v1)`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestValidateKind(t *testing.T) {
	input := `
apiVersion: scribe/v1
kind: Unknown
`
	_, err := manifest.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if want := `unknown kind "Unknown" (expected Registry or Package)`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	input := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: foo
    path: a
  - name: foo
    path: b
`
	_, err := manifest.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for duplicate catalog entry names")
	}
	if want := `duplicate catalog entry name "foo"`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestValidateUnknownType(t *testing.T) {
	input := `
apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: foo
    type: plugin
`
	_, err := manifest.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown entry type")
	}
	if want := `unknown entry type "plugin" for "foo" (expected "" or "package")`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestFindByName(t *testing.T) {
	m, err := manifest.Parse([]byte(teamRegistry))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Found.
	entry := m.FindByName("deploy")
	if entry == nil {
		t.Fatal("FindByName(deploy) returned nil")
	}
	if entry.Name != "deploy" {
		t.Errorf("entry name: got %q, want deploy", entry.Name)
	}

	// Not found.
	if m.FindByName("nonexistent") != nil {
		t.Error("FindByName(nonexistent) should return nil")
	}
}

func TestParseInitializesCatalog(t *testing.T) {
	input := `
apiVersion: scribe/v1
kind: Registry
team:
  name: empty
`
	m, err := manifest.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Catalog == nil {
		t.Error("expected Catalog to be initialized, got nil")
	}
	if len(m.Catalog) != 0 {
		t.Errorf("expected empty Catalog, got %d entries", len(m.Catalog))
	}
}

func TestEntryGroupField(t *testing.T) {
	// Group is display-only and not serialized to YAML.
	const input = `
apiVersion: scribe/v1
kind: Registry
team:
  name: test
catalog:
  - name: my-skill
    source: "github:owner/repo@main"
`
	m, err := manifest.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Set Group programmatically (not from YAML).
	m.Catalog[0].Group = "my-plugin"
	if m.Catalog[0].Group != "my-plugin" {
		t.Errorf("Group: got %q", m.Catalog[0].Group)
	}

	// Verify Group is not serialized.
	out, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(out), "group") {
		t.Errorf("Group should not be serialized, got:\n%s", out)
	}
}

func TestEntryMaintainer(t *testing.T) {
	e := manifest.Entry{
		Name:   "test",
		Author: "krishan",
	}
	if got := e.Maintainer(); got != "krishan" {
		t.Errorf("Maintainer() = %q, want krishan", got)
	}

	empty := manifest.Entry{Name: "no-author"}
	if got := empty.Maintainer(); got != "" {
		t.Errorf("Maintainer() = %q, want empty string", got)
	}
}
