package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
)

func TestResolveRemoveTarget(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		installed []string
		want      string
		wantErr   string
	}{
		{
			name:      "exact namespaced match",
			input:     "Artistfy-hq/recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			want:      "Artistfy-hq/recap",
		},
		{
			name:      "bare name unique match",
			input:     "deploy",
			installed: []string{"Artistfy-hq/deploy", "Artistfy-hq/recap"},
			want:      "Artistfy-hq/deploy",
		},
		{
			name:      "bare name ambiguous",
			input:     "recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			wantErr:   "ambiguous",
		},
		{
			name:      "not found",
			input:     "nonexistent",
			installed: []string{"Artistfy-hq/recap"},
			wantErr:   "not installed",
		},
		{
			name:      "exact match case-sensitive",
			input:     "Artistfy-hq/deploy",
			installed: []string{"Artistfy-hq/deploy"},
			want:      "Artistfy-hq/deploy",
		},
		{
			name:      "bare name single installed",
			input:     "recap",
			installed: []string{"Artistfy-hq/recap"},
			want:      "Artistfy-hq/recap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRemoveTarget(tc.input, tc.installed)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRemoveLeavesConflictResidue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	canonical, err := filepath.Abs(filepath.Join(home, ".scribe", "skills", "recap"))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatalf("MkdirAll canonical: %v", err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# recap\n"), 0o644); err != nil {
		t.Fatalf("WriteFile canonical: %v", err)
	}
	managed := filepath.Join(home, ".codex", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatalf("MkdirAll managed dir: %v", err)
	}
	if err := os.Symlink(canonical, managed); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	residue := filepath.Join(home, ".claude", "skills", "recap")
	if err := os.MkdirAll(filepath.Dir(residue), 0o755); err != nil {
		t.Fatalf("MkdirAll residue dir: %v", err)
	}
	if err := os.WriteFile(residue, []byte("# recap\nlocal drift\n"), 0o644); err != nil {
		t.Fatalf("WriteFile residue: %v", err)
	}

	st := &state.State{
		SchemaVersion: 4,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision:     1,
				Tools:        []string{},
				ManagedPaths: []string{managed},
				Paths:        []string{managed},
				Conflicts:    []state.ProjectionConflict{{Tool: "claude", Path: residue, FoundHash: "hash"}},
			},
		},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cmd := newRemoveCommand()
	cmd.SetArgs([]string{"recap", "--no-interaction", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Lstat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed path still exists: %v", err)
	}
	if _, err := os.Stat(residue); err != nil {
		t.Fatalf("conflict residue missing: %v", err)
	}
}

func TestRunPackageUninstall_RunsScribeYAMLCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pkgsDir, _ := paths.PackagesDir()
	pkgDir := filepath.Join(pkgsDir, "gstack")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	marker := filepath.Join(pkgDir, "UNINSTALLED")
	manifest := []byte("install:\n  command: ./setup\n  uninstall: touch " + marker + "\n")
	if err := os.WriteFile(filepath.Join(pkgDir, "scribe.yaml"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	warnings := runPackageUninstall(cmd, "gstack", state.InstalledSkill{Kind: state.KindPackage})
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("uninstall command did not run: %v", err)
	}
}

func TestRunPackageUninstall_NoManifestNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pkgsDir, _ := paths.PackagesDir()
	pkgDir := filepath.Join(pkgsDir, "bare")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	warnings := runPackageUninstall(cmd, "bare", state.InstalledSkill{Kind: state.KindPackage})
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRemoveRecordsRemovalIntentForAllSources(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		SchemaVersion: 5,
		Installed: map[string]state.InstalledSkill{
			"recap": {
				Revision: 1,
				Sources: []state.SkillSource{
					{Registry: "acme/skills", Ref: "main"},
					{Registry: "other/skills", Ref: "main"},
				},
				Tools: []string{},
			},
		},
		RemovedByUser: []state.RemovedSkill{},
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cmd := newRemoveCommand()
	cmd.SetArgs([]string{"recap", "--no-interaction", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := loaded.Installed["recap"]; ok {
		t.Fatal("recap should be removed from installed state")
	}
	if !loaded.IsRemovedByUser("acme/skills", "recap") {
		t.Fatal("recap should be deny-listed for acme/skills")
	}
	if !loaded.IsRemovedByUser("other/skills", "recap") {
		t.Fatal("recap should be deny-listed for other/skills")
	}
}
