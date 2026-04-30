package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/add"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
)

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

func TestMissingSourceWarning(t *testing.T) {
	candidate := add.Candidate{
		Name:        "borrowed",
		Description: "Imported from https://github.com/acme/borrowed",
		LocalPath:   "/tmp/borrowed",
	}

	got := missingSourceWarning(candidate)
	if got == "" {
		t.Fatal("expected warning for GitHub URL without source frontmatter")
	}

	candidate.Attribution = discovery.Source{URL: "https://github.com/acme/borrowed"}
	if got := missingSourceWarning(candidate); got != "" {
		t.Fatalf("expected no warning when source is set, got %q", got)
	}

	candidate.Attribution = discovery.Source{}
	candidate.Description = "⛔️ https://github.com/acme/borrowed"
	if got := missingSourceWarning(candidate); got != "" {
		t.Fatalf("expected opt-out marker to suppress warning, got %q", got)
	}
}

func TestMissingSourceWarningUsesRawDiscoveredDescription(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	longPrefix := strings.Repeat("background ", 8)
	descriptionWithURL := longPrefix + "Imported from https://github.com/acme/upstreamed-skill"

	writeSkill := func(name, body string) {
		t.Helper()
		skillDir := filepath.Join(home, ".scribe", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkill("upstreamed", "---\nname: upstreamed\ndescription: "+descriptionWithURL+"\n---\n")
	writeSkill("plain", "---\nname: plain\ndescription: "+longPrefix+"Locally authored helper\n---\n")
	writeSkill("sourced", "---\nname: sourced\ndescription: "+descriptionWithURL+"\nsource:\n  url: https://github.com/acme/upstreamed-skill\n---\n")

	adder := &add.Adder{}
	candidates, err := adder.DiscoverLocal(&state.State{Installed: map[string]state.InstalledSkill{}})
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}

	byName := map[string]add.Candidate{}
	for _, candidate := range candidates {
		byName[candidate.Name] = candidate
	}

	upstreamed := byName["upstreamed"]
	if upstreamed.RawDescription != descriptionWithURL {
		t.Fatalf("RawDescription = %q, want %q", upstreamed.RawDescription, descriptionWithURL)
	}
	if upstreamed.Description == upstreamed.RawDescription {
		t.Fatalf("Description was not truncated: %q", upstreamed.Description)
	}
	if githubURLInDescriptionRE.MatchString(upstreamed.Description) {
		t.Fatalf("truncated Description unexpectedly contains a GitHub URL: %q", upstreamed.Description)
	}
	if got := missingSourceWarning(upstreamed); got == "" {
		t.Fatal("expected warning for raw GitHub URL without source frontmatter")
	}

	if got := missingSourceWarning(byName["plain"]); got != "" {
		t.Fatalf("expected no warning without GitHub URL, got %q", got)
	}
	if got := missingSourceWarning(byName["sourced"]); got != "" {
		t.Fatalf("expected no warning with source frontmatter, got %q", got)
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
