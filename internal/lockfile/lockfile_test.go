package lockfile

import (
	"strings"
	"testing"
)

const hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestParseEncodeValidate(t *testing.T) {
	raw := []byte(`
version: 1
registry: acme/skills
entries:
  - name: deploy
    source_registry: acme/skills
    registry_commit_sha: abc123
    content_hash: ` + hashA + `
    install_command_hash: ` + hashB + `
`)
	lf, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if lf.Registry != "acme/skills" || len(lf.Entries) != 1 {
		t.Fatalf("unexpected lockfile: %+v", lf)
	}
	encoded, err := lf.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !strings.Contains(string(encoded), "scribe") && !strings.Contains(string(encoded), "deploy") {
		t.Fatalf("encoded lockfile missing entry: %s", encoded)
	}
}

func TestValidateRejectsDuplicateEntries(t *testing.T) {
	lf := &Lockfile{
		Version:  SchemaVersion,
		Registry: "acme/skills",
		Entries: []Entry{
			{Name: "deploy", SourceRegistry: "acme/skills", RegistryCommitSHA: "a", ContentHash: hashA},
			{Name: "deploy", SourceRegistry: "acme/skills", RegistryCommitSHA: "b", ContentHash: hashB},
		},
	}
	if err := lf.Validate(); err == nil {
		t.Fatal("Validate() should reject duplicate entries")
	}
}

func TestDiffReportsChangedPins(t *testing.T) {
	current := &Lockfile{Version: SchemaVersion, Registry: "acme/skills", Entries: []Entry{
		{Name: "deploy", SourceRegistry: "acme/skills", RegistryCommitSHA: "old", ContentHash: hashA},
	}}
	latest := &Lockfile{Version: SchemaVersion, Registry: "acme/skills", Entries: []Entry{
		{Name: "deploy", SourceRegistry: "acme/skills", RegistryCommitSHA: "new", ContentHash: hashB},
		{Name: "review", SourceRegistry: "acme/skills", RegistryCommitSHA: "new", ContentHash: hashA},
	}}
	updates := Diff(current, latest)
	if len(updates) != 2 {
		t.Fatalf("len(updates) = %d, want 2: %+v", len(updates), updates)
	}
	if updates[0].Name != "deploy" || updates[0].CurrentSHA != "old" || updates[0].LatestSHA != "new" {
		t.Fatalf("unexpected first update: %+v", updates[0])
	}
}
