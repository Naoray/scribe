package sync

import "testing"

func TestShouldInclude(t *testing.T) {
	cases := []struct {
		name string
		file string
		want bool
	}{
		{"skill file", "SKILL.md", true},
		{"script", "scripts/deploy.sh", true},
		{"readme", "README.md", true},
		{"nested file", "lib/helper.go", true},

		// Deny list — these are repo-root files that leak into skill directories
		// when the skill path == repo root.
		{"dot git", ".git/config", false},
		{"dot gitignore", ".gitignore", false},
		{"dot gitkeep", ".gitkeep", false},
		{"license", "LICENSE", false},
		{"license md", "LICENSE.md", false},
		{"license txt", "LICENSE.txt", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldInclude(c.file)
			if got != c.want {
				t.Errorf("shouldInclude(%q) = %v, want %v", c.file, got, c.want)
			}
		})
	}
}
