package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		info    *debug.BuildInfo
		want    string
	}{
		{
			name:    "keeps ldflags version",
			initial: "v1.2.3",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v9.9.9"},
			},
			want: "v1.2.3",
		},
		{
			name:    "uses module version for dev builds",
			initial: "dev",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.12.3"},
			},
			want: "v0.12.3",
		},
		{
			name:    "keeps dev for local devel builds",
			initial: "dev",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
			},
			want: "dev",
		},
		{
			name:    "keeps dev without build info",
			initial: "dev",
			info:    nil,
			want:    "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.initial, tt.info); got != tt.want {
				t.Fatalf("resolveVersion(%q) = %q, want %q", tt.initial, got, tt.want)
			}
		})
	}
}

func TestRoot_JSONFlag_StdoutIsCleanJSON_EvenDuringBuiltinsBackfill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := newRootCmd()
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	root.SetArgs([]string{"list", "--json"})

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()

	execErr := root.Execute()
	w.Close()

	if execErr != nil {
		var stderr bytes.Buffer
		stderr.ReadFrom(r)
		t.Fatalf("execute: %v (stderr=%s stdout=%s)", execErr, errBuf.String(), stderr.String())
	}

	var outBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	out := strings.TrimSpace(outBuf.String())
	var anyJSON interface{}
	if err := json.Unmarshal([]byte(out), &anyJSON); err != nil {
		t.Errorf("stdout not clean JSON: %v\nstdout: %q", err, out)
	}

	stdout := outBuf.String()
	if strings.Contains(stdout, "Welcome to Scribe") || strings.Contains(stdout, "new built-in") {
		t.Errorf("banner leaked into stdout: %q", stdout)
	}

	stderr := errBuf.String()
	if !strings.Contains(stderr, "Welcome to Scribe") {
		t.Errorf("expected first-run banner on stderr, got: %q", stderr)
	}
	// Naoray/scribe is no longer a builtin — scribe-agent is managed by the binary.
	if strings.Contains(stderr, "Naoray/scribe") {
		t.Errorf("Naoray/scribe must not appear in first-run banner, got: %q", stderr)
	}
	if !strings.Contains(stderr, "anthropics/skills") {
		t.Errorf("expected anthropics/skills in first-run banner, got: %q", stderr)
	}
}

func TestRoot_JSONFlag_ExistingUserBuiltinsBackfillIsSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`registries:
  - repo: anthropics/skills
    enabled: true
    builtin: true
    type: community
  - repo: expo/skills
    enabled: true
    builtin: true
    type: community
builtins_version: 1
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	root := newRootCmd()
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	root.SetArgs([]string{"list", "--json"})

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()

	execErr := root.Execute()
	w.Close()

	if execErr != nil {
		var stderr bytes.Buffer
		stderr.ReadFrom(r)
		t.Fatalf("execute: %v (stderr=%s stdout=%s)", execErr, errBuf.String(), stderr.String())
	}

	var outBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	out := strings.TrimSpace(outBuf.String())
	var anyJSON interface{}
	if err := json.Unmarshal([]byte(out), &anyJSON); err != nil {
		t.Fatalf("stdout not clean JSON: %v\nstdout: %q", err, out)
	}

	if got := errBuf.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRoot_JSONFlag_LegacyAnthropicBuiltinIsSilentlyRenamed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".scribe")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`registries:
  - repo: anthropic/skills
    enabled: true
    builtin: true
    type: community
  - repo: expo/skills
    enabled: true
    builtin: true
    type: community
builtins_version: 3
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	root := newRootCmd()
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	root.SetArgs([]string{"list", "--json"})

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()

	execErr := root.Execute()
	w.Close()
	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	var outBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	out := strings.TrimSpace(outBuf.String())
	var anyJSON interface{}
	if err := json.Unmarshal([]byte(out), &anyJSON); err != nil {
		t.Fatalf("stdout not clean JSON: %v\nstdout: %q", err, out)
	}
	if got := errBuf.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	rawConfig, err := os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := string(rawConfig)
	if strings.Contains(configText, "anthropic/skills") {
		t.Fatalf("legacy anthropic/skills entry still present:\n%s", configText)
	}
	if !strings.Contains(configText, "anthropics/skills") {
		t.Fatalf("renamed anthropics/skills entry missing:\n%s", configText)
	}
}
