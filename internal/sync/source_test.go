package sync

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
)

func TestSkillSourceFromEntryPersistsBlobSHAs(t *testing.T) {
	blobs := map[string]string{
		"skills/deploy/SKILL.md":       "skillblob",
		"skills/deploy/scripts/foo.sh": "scriptblob",
	}

	source := skillSourceFromEntry("acme/team", &manifest.Entry{
		Name:   "deploy",
		Path:   "skills/deploy",
		Source: "github:acme/skills@main",
	}, "main", "skillblob", blobs)

	if source.BlobSHAs["skills/deploy/SKILL.md"] != "skillblob" {
		t.Fatalf("SKILL.md SHA = %q", source.BlobSHAs["skills/deploy/SKILL.md"])
	}
	if source.BlobSHAs["skills/deploy/scripts/foo.sh"] != "scriptblob" {
		t.Fatalf("script SHA = %q", source.BlobSHAs["skills/deploy/scripts/foo.sh"])
	}

	blobs["skills/deploy/SKILL.md"] = "mutated"
	if source.BlobSHAs["skills/deploy/SKILL.md"] != "skillblob" {
		t.Fatal("source BlobSHAs should not alias caller map")
	}
}
