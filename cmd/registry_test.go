package cmd

import "testing"

func TestResolveRegistry(t *testing.T) {
	repos := []string{"ArtistfyHQ/team-skills", "vercel/skills", "acme/skills"}

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{"exact match", "ArtistfyHQ/team-skills", "ArtistfyHQ/team-skills", ""},
		{"exact case-insensitive", "artistfyhq/team-skills", "ArtistfyHQ/team-skills", ""},
		{"partial repo name", "team-skills", "ArtistfyHQ/team-skills", ""},
		{"partial case-insensitive", "Team-Skills", "ArtistfyHQ/team-skills", ""},
		{"exact full disambiguates", "vercel/skills", "vercel/skills", ""},
		{"ambiguous partial", "skills", "", "ambiguous"},
		{"unknown", "nonexistent", "", "not connected"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveRegistry(c.input, repos)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErr)
				}
				if !containsCI(err.Error(), c.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// containsCI checks if s contains substr (case-insensitive).
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(substr) == 0 ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if equalFoldAt(s[i:i+len(substr)], substr) {
						return true
					}
				}
				return false
			}())
}

func equalFoldAt(a, b string) bool {
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
