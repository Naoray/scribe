package sync

import (
	"testing"

	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
)

// resolveSkillBlobSHA looks up the blob SHA of SKILL.md for a catalog entry
// in a tree listing. This is the identity signal we compare against state —
// commit SHAs change for unrelated repo activity, blob SHAs only change when
// the file content changes.
func TestResolveSkillBlobSHA(t *testing.T) {
	tree := []provider.TreeEntry{
		{Path: "README.md", Type: "blob", SHA: "readmesha"},
		{Path: "SKILL.md", Type: "blob", SHA: "rootsha"},
		{Path: "skills/xray/SKILL.md", Type: "blob", SHA: "xrayblob"},
		{Path: "skills/xray/helpers.md", Type: "blob", SHA: "helperblob"},
		{Path: "skills/deploy/SKILL.md", Type: "blob", SHA: "deployblob"},
		{Path: "skills", Type: "tree", SHA: "treesha"},
	}

	cases := []struct {
		name  string
		entry manifest.Entry
		want  string
		found bool
	}{
		{
			name:  "resolves via explicit path",
			entry: manifest.Entry{Name: "xray", Path: "skills/xray"},
			want:  "xrayblob",
			found: true,
		},
		{
			name:  "falls back to name when path omitted",
			entry: manifest.Entry{Name: "skills/deploy"},
			want:  "deployblob",
			found: true,
		},
		{
			name:  "returns empty for missing skill",
			entry: manifest.Entry{Name: "ghost", Path: "skills/ghost"},
			want:  "",
			found: false,
		},
		{
			name:  "handles root-level skill paths",
			entry: manifest.Entry{Name: "repo-skill", Path: "."},
			want:  "rootsha",
			found: true,
		},
		{
			name:  "ignores tree entries (only blobs)",
			entry: manifest.Entry{Name: "skills", Path: "skills"},
			want:  "",
			found: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := resolveSkillBlobSHA(tree, c.entry)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if found != c.found {
				t.Errorf("found = %v, want %v", found, c.found)
			}
		})
	}
}
