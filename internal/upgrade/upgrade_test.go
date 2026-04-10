package upgrade

import (
	"testing"
)

func TestDetectMethod(t *testing.T) {
	tests := []struct {
		name           string
		executablePath string
		evalSymlinks   string // resolved path (empty = same as executablePath)
		brewListOK     bool
		want           Method
	}{
		{
			name:           "homebrew via /opt/homebrew",
			executablePath: "/opt/homebrew/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via Cellar",
			executablePath: "/usr/local/Cellar/scribe/0.5.0/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via linuxbrew",
			executablePath: "/home/linuxbrew/.linuxbrew/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "homebrew via symlink resolution",
			executablePath: "/usr/local/bin/scribe",
			evalSymlinks:   "/opt/homebrew/Cellar/scribe/0.5.0/bin/scribe",
			want:           MethodHomebrew,
		},
		{
			name:           "go install",
			executablePath: "/home/user/go/bin/scribe",
			want:           MethodGoInstall,
		},
		{
			name:           "ambiguous path, brew list succeeds",
			executablePath: "/usr/local/bin/scribe",
			brewListOK:     true,
			want:           MethodHomebrew,
		},
		{
			name:           "ambiguous path, brew list fails",
			executablePath: "/usr/local/bin/scribe",
			brewListOK:     false,
			want:           MethodCurlBinary,
		},
		{
			name:           "tmp path, fallback to curl binary",
			executablePath: "/tmp/scribe",
			want:           MethodCurlBinary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origExec := executablePath
			origSymlinks := evalSymlinks
			origBrew := brewListCheck
			t.Cleanup(func() {
				executablePath = origExec
				evalSymlinks = origSymlinks
				brewListCheck = origBrew
			})

			executablePath = func() (string, error) {
				return tt.executablePath, nil
			}

			resolved := tt.evalSymlinks
			if resolved == "" {
				resolved = tt.executablePath
			}
			evalSymlinks = func(path string) (string, error) {
				return resolved, nil
			}

			brewListCheck = func(name string) bool {
				return tt.brewListOK
			}

			got := DetectMethod()
			if got != tt.want {
				t.Errorf("DetectMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsUpgrade(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		latestTag   string
		wantSkip    bool // dev build — skip entirely
		wantUpgrade bool
	}{
		{
			name:        "dev build skips upgrade",
			current:     "dev",
			latestTag:   "v0.5.0",
			wantSkip:    true,
			wantUpgrade: false,
		},
		{
			name:        "same version, no upgrade",
			current:     "0.5.0",
			latestTag:   "v0.5.0",
			wantSkip:    false,
			wantUpgrade: false,
		},
		{
			name:        "older version, needs upgrade",
			current:     "0.4.0",
			latestTag:   "v0.5.0",
			wantSkip:    false,
			wantUpgrade: true,
		},
		{
			name:        "tag without v prefix",
			current:     "0.5.0",
			latestTag:   "0.5.0",
			wantSkip:    false,
			wantUpgrade: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, upgrade := NeedsUpgrade(tt.current, tt.latestTag)
			if skip != tt.wantSkip {
				t.Errorf("NeedsUpgrade() skip = %v, want %v", skip, tt.wantSkip)
			}
			if upgrade != tt.wantUpgrade {
				t.Errorf("NeedsUpgrade() upgrade = %v, want %v", upgrade, tt.wantUpgrade)
			}
		})
	}
}
