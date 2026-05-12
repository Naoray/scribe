package workflow_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
)

func TestStepInstallKitsWritesAndStampsState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bag := &workflow.Bag{
		RepoArg:   "Naoray/skills",
		State:     newKitInstallState(),
		Formatter: workflow.NewFormatterWithWriters(false, false, &bytes.Buffer{}, &bytes.Buffer{}),
		Kits:      []provider.KitFile{kitFile("daily-workflow", "kits/daily-workflow.yaml", []string{"plan-my-day"}, "abc123")},
	}
	if err := workflow.StepInstallKits(context.Background(), bag); err != nil {
		t.Fatalf("StepInstallKits: %v", err)
	}

	written := loadKitFile(t, home, "daily-workflow")
	if written.Source == nil || written.Source.Registry != "Naoray/skills" || written.Source.Rev != "abc123" {
		t.Fatalf("written source = %+v", written.Source)
	}
	if !bag.StateDirty {
		t.Fatal("StateDirty = false, want true")
	}
	stamped := bag.State.Kits["daily-workflow"]
	if stamped.Source != "Naoray/skills" || stamped.Version != "abc123" || len(stamped.Skills) != 1 {
		t.Fatalf("state stamp = %+v", stamped)
	}
	if len(bag.KitsInstalled) != 1 || bag.KitsInstalled[0] != "daily-workflow" {
		t.Fatalf("KitsInstalled = %v", bag.KitsInstalled)
	}
}

func TestStepInstallKitsConflictMatrix(t *testing.T) {
	cases := []struct {
		name           string
		existing       *kit.Source
		force          bool
		wantWritten    bool
		wantPartial    bool
		wantWarnSubstr string
	}{
		{name: "same registry", existing: &kit.Source{Registry: "Naoray/skills"}, wantWritten: true},
		{name: "hand authored nil source", existing: nil, wantPartial: true, wantWarnSubstr: "source=hand-authored"},
		{name: "hand authored empty registry", existing: &kit.Source{Registry: ""}, wantPartial: true, wantWarnSubstr: "source=hand-authored"},
		{name: "other registry", existing: &kit.Source{Registry: "Other/skills"}, wantPartial: true, wantWarnSubstr: "source=Other/skills"},
		{name: "force overwrites hand authored", existing: nil, force: true, wantWritten: true},
		{name: "force overwrites other registry", existing: &kit.Source{Registry: "Other/skills"}, force: true, wantWritten: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeExistingKit(t, home, "daily-workflow", tc.existing)
			var out, errOut bytes.Buffer
			bag := &workflow.Bag{
				RepoArg:   "Naoray/skills",
				State:     newKitInstallState(),
				ForceKits: tc.force,
				Formatter: workflow.NewFormatterWithWriters(false, false, &out, &errOut),
				Kits:      []provider.KitFile{kitFile("daily-workflow", "kits/daily-workflow.yaml", []string{"plan-my-day"}, "abc123")},
			}

			if err := workflow.StepInstallKits(context.Background(), bag); err != nil {
				t.Fatalf("StepInstallKits: %v", err)
			}
			got := loadKitFile(t, home, "daily-workflow")
			wroteIncoming := got.Source != nil && got.Source.Registry == "Naoray/skills"
			if wroteIncoming != tc.wantWritten {
				t.Fatalf("wrote incoming = %v, want %v; source=%+v", wroteIncoming, tc.wantWritten, got.Source)
			}
			if bag.Partial != tc.wantPartial {
				t.Fatalf("Partial = %v, want %v", bag.Partial, tc.wantPartial)
			}
			if tc.wantWarnSubstr != "" && !strings.Contains(errOut.String(), tc.wantWarnSubstr) {
				t.Fatalf("warning %q missing %q", errOut.String(), tc.wantWarnSubstr)
			}
		})
	}
}

func TestStepInstallKitsBodyNameMismatchIsFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bag := &workflow.Bag{
		RepoArg:   "Naoray/skills",
		State:     newKitInstallState(),
		Formatter: workflow.NewFormatterWithWriters(false, false, &bytes.Buffer{}, &bytes.Buffer{}),
		Kits: []provider.KitFile{{
			Name: "daily-workflow",
			Path: "kits/daily-workflow.yaml",
			Body: []byte("apiVersion: scribe/v1\nkind: Kit\nname: other\nskills: []\n"),
			Ref:  "abc123",
		}},
	}
	err := workflow.StepInstallKits(context.Background(), bag)
	if err == nil || !strings.Contains(err.Error(), "doesn't match manifest ref") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestStepInstallKitsPreflightAtomic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bag := &workflow.Bag{
		RepoArg:   "Naoray/skills",
		State:     newKitInstallState(),
		Formatter: workflow.NewFormatterWithWriters(false, false, &bytes.Buffer{}, &bytes.Buffer{}),
		Kits: []provider.KitFile{
			kitFile("daily-workflow", "kits/daily-workflow.yaml", []string{"plan-my-day"}, "abc123"),
			{
				Name: "bad-kit",
				Path: "kits/bad-kit.yaml",
				Body: []byte("apiVersion: scribe/v1\nkind: Kit\nname: ../bad\nskills: []\n"),
				Ref:  "abc123",
			},
		},
	}
	err := workflow.StepInstallKits(context.Background(), bag)
	if err == nil {
		t.Fatal("expected preflight parse error")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".scribe", "kits", "daily-workflow.yaml")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("preflight failure wrote first kit; stat err = %v", statErr)
	}
	if len(bag.State.Kits) != 0 {
		t.Fatalf("preflight failure stamped state: %+v", bag.State.Kits)
	}
}

func kitFile(name, path string, skills []string, ref string) provider.KitFile {
	return provider.KitFile{
		Name: name,
		Path: path,
		Body: []byte("apiVersion: scribe/v1\nkind: Kit\nname: " + name + "\nskills: [" + strings.Join(skills, ", ") + "]\n"),
		Ref:  ref,
	}
}

func newKitInstallState() *state.State {
	return &state.State{
		SchemaVersion: state.CurrentSchemaVersion,
		Installed:     map[string]state.InstalledSkill{},
		Kits:          map[string]state.InstalledKit{},
	}
}

func writeExistingKit(t *testing.T, home, name string, source *kit.Source) {
	t.Helper()
	k := &kit.Kit{Name: name, Skills: []string{"existing"}, Source: source}
	if err := kit.Save(filepath.Join(home, ".scribe", "kits", name+".yaml"), k); err != nil {
		t.Fatalf("save existing kit: %v", err)
	}
}

func loadKitFile(t *testing.T, home, name string) *kit.Kit {
	t.Helper()
	k, err := kit.Load(filepath.Join(home, ".scribe", "kits", name+".yaml"))
	if err != nil {
		t.Fatalf("load kit: %v", err)
	}
	return k
}
