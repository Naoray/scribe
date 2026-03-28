package cmd

import "testing"

func TestParseOwnerRepo(t *testing.T) {
	cases := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"ArtistfyHQ/team-skills", "ArtistfyHQ", "team-skills", false},
		{"vercel/skills", "vercel", "skills", false},
		{"a/b", "a", "b", false},
		{"  ArtistfyHQ/team-skills  ", "ArtistfyHQ", "team-skills", false}, // trimmed

		// Invalid formats.
		{"notarepo", "", "", true},
		{"", "", "", true},
		{"/repo", "", "", true},
		{"owner/", "", "", true},
		{"a/b/c", "a", "b/c", false}, // SplitN(2) keeps the rest
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			owner, repo, err := parseOwnerRepo(c.input)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", c.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != c.wantOwner {
				t.Errorf("owner: got %q, want %q", owner, c.wantOwner)
			}
			if repo != c.wantRepo {
				t.Errorf("repo: got %q, want %q", repo, c.wantRepo)
			}
		})
	}
}

func TestResolveRepoWithArg(t *testing.T) {
	repo, err := resolveRepo([]string{"ArtistfyHQ/team-skills"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "ArtistfyHQ/team-skills" {
		t.Errorf("got %q", repo)
	}
}

func TestResolveRepoNoArgNonTTY(t *testing.T) {
	// When stdin is not a TTY (like in tests), resolveRepo with no args should error.
	_, err := resolveRepo([]string{})
	if err == nil {
		t.Error("expected error when no arg and non-TTY stdin")
	}
}
