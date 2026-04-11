package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/config"
)

func TestAdoptionMode(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want string
	}{
		{"empty defaults to auto", "", "auto"},
		{"auto stays auto", "auto", "auto"},
		{"prompt stays prompt", "prompt", "prompt"},
		{"off stays off", "off", "off"},
		{"garbage falls back to auto", "garbage", "auto"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Adoption: config.AdoptionConfig{Mode: tc.mode}}
			got := cfg.AdoptionMode()
			if got != tc.want {
				t.Errorf("AdoptionMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAdoptionPaths(t *testing.T) {
	t.Run("zero value returns builtins", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := &config.Config{}
		paths, err := cfg.AdoptionPaths()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".codex", "skills"),
		}
		if len(paths) != len(want) {
			t.Fatalf("got %d paths, want %d: %v", len(paths), len(want), paths)
		}
		for i, w := range want {
			if paths[i] != w {
				t.Errorf("paths[%d] = %q, want %q", i, paths[i], w)
			}
		}
	})

	t.Run("tilde prefix expands and appends", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := &config.Config{Adoption: config.AdoptionConfig{Paths: []string{"~/src/my-skills"}}}
		paths, err := cfg.AdoptionPaths()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// builtins + 1 extra
		if len(paths) != 3 {
			t.Fatalf("got %d paths, want 3: %v", len(paths), paths)
		}
		want := filepath.Join(home, "src", "my-skills")
		if paths[2] != want {
			t.Errorf("expanded path = %q, want %q", paths[2], want)
		}
	})

	t.Run("relative path resolves to home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := &config.Config{Adoption: config.AdoptionConfig{Paths: []string{"relative/dir"}}}
		paths, err := cfg.AdoptionPaths()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 3 {
			t.Fatalf("got %d paths, want 3: %v", len(paths), paths)
		}
		want := filepath.Join(home, "relative", "dir")
		if paths[2] != want {
			t.Errorf("resolved path = %q, want %q", paths[2], want)
		}
	})

	t.Run("absolute path outside home returns error", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := &config.Config{Adoption: config.AdoptionConfig{Paths: []string{"/tmp/outside"}}}
		_, err := cfg.AdoptionPaths()
		if err == nil {
			t.Fatal("expected error for path outside home")
		}
		if !strings.Contains(err.Error(), "outside home") {
			t.Errorf("error %q should contain %q", err.Error(), "outside home")
		}
	})

	t.Run("builtins always come first", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := &config.Config{Adoption: config.AdoptionConfig{Paths: []string{"~/extra"}}}
		paths, err := cfg.AdoptionPaths()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) < 2 {
			t.Fatal("expected at least 2 paths")
		}
		if !strings.Contains(paths[0], ".claude") {
			t.Errorf("first path should be .claude/skills, got %q", paths[0])
		}
		if !strings.Contains(paths[1], ".codex") {
			t.Errorf("second path should be .codex/skills, got %q", paths[1])
		}
	})
}

func TestConfigAdoptionRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := &config.Config{
		Adoption: config.AdoptionConfig{
			Mode:  "off",
			Paths: []string{"~/src/foo"},
		},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Adoption.Mode != "off" {
		t.Errorf("Mode = %q, want %q", loaded.Adoption.Mode, "off")
	}
	if len(loaded.Adoption.Paths) != 1 || loaded.Adoption.Paths[0] != "~/src/foo" {
		t.Errorf("Paths = %v, want [~/src/foo]", loaded.Adoption.Paths)
	}
}

func TestLoadRejectsOutsideHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeYAMLConfig(t, home, "adoption:\n  paths:\n    - /etc/passwd\n")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for outside-home path in config")
	}
	if !strings.Contains(err.Error(), "outside home") {
		t.Errorf("error %q should contain %q", err.Error(), "outside home")
	}
}
