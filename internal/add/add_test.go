package add_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/add"
)

func TestCandidateUploadFlag(t *testing.T) {
	cases := []struct {
		name   string
		c      add.Candidate
		upload bool
	}{
		{
			name:   "local with source",
			c:      add.Candidate{Name: "deploy", Source: "github:owner/repo@v1.0.0", LocalPath: "/home/user/.scribe/skills/deploy"},
			upload: false,
		},
		{
			name:   "local without source",
			c:      add.Candidate{Name: "cleanup", LocalPath: "/home/user/.claude/skills/cleanup"},
			upload: true,
		},
		{
			name:   "remote only",
			c:      add.Candidate{Name: "nextjs", Source: "github:vercel/skills@v2.0.0", Origin: "registry:vercel/skills"},
			upload: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.c.NeedsUpload() != tc.upload {
				t.Errorf("NeedsUpload() = %v, want %v", tc.c.NeedsUpload(), tc.upload)
			}
		})
	}
}

func TestAdderEmitNilSafe(t *testing.T) {
	a := &add.Adder{}
	// Should not panic with nil Emit callback.
	a.Emit = nil
	// No public method yet to test emit, but verifying struct creates without panic.
	if a.Client != nil {
		t.Error("expected nil client on zero-value Adder")
	}
}
