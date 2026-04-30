package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

func TestDetectLegacyGlobalProjectionCompat(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested project: %v", err)
	}

	withGlobal := &state.State{
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Projections: []state.ProjectionEntry{{
					Project: "",
					Tools:   []string{"claude"},
				}},
			},
		},
	}
	withoutGlobal := &state.State{
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Projections: []state.ProjectionEntry{{
					Project: root,
					Tools:   []string{"claude"},
				}},
			},
		},
	}

	got, err := state.DetectLegacyGlobalProjectionCompat(withGlobal, nested)
	if err != nil {
		t.Fatalf("DetectLegacyGlobalProjectionCompat() error = %v", err)
	}
	if !got.Enabled {
		t.Fatalf("Enabled = false, want true for global projection without .scribe.yaml")
	}
	if !got.HasGlobalProjections {
		t.Fatalf("HasGlobalProjections = false, want true")
	}

	projectFile := filepath.Join(root, ".scribe.yaml")
	if err := os.WriteFile(projectFile, []byte("kits: []\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	got, err = state.DetectLegacyGlobalProjectionCompat(withGlobal, nested)
	if err != nil {
		t.Fatalf("DetectLegacyGlobalProjectionCompat() with project file error = %v", err)
	}
	if got.Enabled {
		t.Fatalf("Enabled = true, want false when .scribe.yaml exists")
	}
	if got.ProjectFile != projectFile {
		t.Fatalf("ProjectFile = %q, want %q", got.ProjectFile, projectFile)
	}

	got, err = state.DetectLegacyGlobalProjectionCompat(withoutGlobal, nested)
	if err != nil {
		t.Fatalf("DetectLegacyGlobalProjectionCompat() without global error = %v", err)
	}
	if got.Enabled || got.HasGlobalProjections {
		t.Fatalf("got %+v, want no compat mode without global projections", got)
	}
}

func TestShouldEmitLegacyGlobalProjectionCompatBannerAt(t *testing.T) {
	timestampPath := filepath.Join(t.TempDir(), ".scribe", "legacy-global-projection-banner.date")
	first := time.Date(2026, 4, 30, 9, 0, 0, 0, time.Local)

	emit, err := state.ShouldEmitLegacyGlobalProjectionCompatBannerAt(timestampPath, first)
	if err != nil {
		t.Fatalf("first ShouldEmitLegacyGlobalProjectionCompatBannerAt() error = %v", err)
	}
	if !emit {
		t.Fatalf("first emit = false, want true")
	}

	emit, err = state.ShouldEmitLegacyGlobalProjectionCompatBannerAt(timestampPath, first.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("same-day ShouldEmitLegacyGlobalProjectionCompatBannerAt() error = %v", err)
	}
	if emit {
		t.Fatalf("same-day emit = true, want false")
	}

	emit, err = state.ShouldEmitLegacyGlobalProjectionCompatBannerAt(timestampPath, first.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("next-day ShouldEmitLegacyGlobalProjectionCompatBannerAt() error = %v", err)
	}
	if !emit {
		t.Fatalf("next-day emit = false, want true")
	}
}
